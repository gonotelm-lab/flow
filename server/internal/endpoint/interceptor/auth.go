package interceptor

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/auth"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthConfig 是客户端面鉴权要用的依赖。
//
// Enabled 为 false（或者 AuthConfig 是 nil）时直接放行：不读 metadata、不查库，
// 和加鉴权之前一模一样。运维面显式传 nil。
type AuthConfig struct {
	// Enabled 对应 [apiServer.auth].enabled。
	Enabled bool
	// Namespaces 用于按 api_key 反查 namespace。
	Namespaces store.Namespace
}

// unauthorizedDetail 是鉴权失败时统一对外的文案：不区分「没带 key」「key 不认识」
// 「格式不对」，失败也不写日志。区分原因、留下记录，只会方便扫描的人。
const unauthorizedDetail = "api key is missing or invalid"

func authUnaryInterceptor(cfg *AuthConfig) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx, err := authenticate(ctx, cfg)
		if err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

func authStreamInterceptor(cfg *AuthConfig) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, err := authenticate(ss.Context(), cfg)
		if err != nil {
			return err
		}

		return handler(srv, &authServerStream{ServerStream: ss, ctx: ctx})
	}
}

// authenticate 校验凭据并把解析出的 namespace 放进 context。
func authenticate(ctx context.Context, cfg *AuthConfig) (context.Context, error) {
	if cfg == nil || !cfg.Enabled {
		return ctx, nil
	}

	md, _ := metadata.FromIncomingContext(ctx)
	key, err := auth.BearerToken(md)
	if err != nil {
		return nil, pkgerr.Unauthenticated.WithDetail(unauthorizedDetail)
	}

	namespace, err := cfg.Namespaces.GetByApiKey(ctx, key)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, pkgerr.Unauthenticated.WithDetail(unauthorizedDetail)
		}

		return nil, errors.WithMessage(err, "failed to resolve api key")
	}

	// 反查回来的 namespace 是空的，说明数据坏了，这里必须 fail closed：
	// 再往下走就成了「认证通过、namespace 为空」，一次数据事故会变成全量越权。
	if namespace.Name == "" {
		return nil, pkgerr.Internal.WithDetail("api key resolved to an empty namespace")
	}

	return auth.WithNamespace(ctx, namespace.Name), nil
}

// authServerStream 让下游 handler 拿到带 namespace 的 context。
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authServerStream) Context() context.Context {
	return s.ctx
}
