package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/auth"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const submitMethod = "/api.task.v1.TaskService/Submit"

// fakeNamespaceStore 只实现鉴权用到的那一个方法，其余方法被调用即说明走错了路。
type fakeNamespaceStore struct {
	byKey map[string]*schema.Namespace
	err   error

	calls int
}

func (f *fakeNamespaceStore) Create(context.Context, *schema.Namespace) (*schema.Namespace, error) {
	panic("Create should not be called by the auth interceptor")
}

func (f *fakeNamespaceStore) Get(context.Context, string) (*schema.Namespace, error) {
	panic("Get should not be called by the auth interceptor")
}

func (f *fakeNamespaceStore) GetByApiKey(_ context.Context, apiKey string) (*schema.Namespace, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}

	ns, ok := f.byKey[apiKey]
	if !ok {
		return nil, pkgerr.NoRecord
	}

	return ns, nil
}

func (f *fakeNamespaceStore) List(context.Context, int, int) ([]*schema.Namespace, int64, error) {
	panic("List should not be called by the auth interceptor")
}

func (f *fakeNamespaceStore) Update(context.Context, *schema.Namespace) error {
	panic("Update should not be called by the auth interceptor")
}

func incomingCtx(pairs ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(pairs...))
}

// invoke 直接调用 unary 鉴权拦截器，返回 handler 拿到的 ctx 与错误。
func invoke(t *testing.T, cfg *AuthConfig, ctx context.Context) (context.Context, error) {
	t.Helper()

	var gotCtx context.Context
	_, err := authUnaryInterceptor(cfg)(
		ctx,
		&struct{}{},
		&grpc.UnaryServerInfo{FullMethod: submitMethod},
		func(ctx context.Context, _ any) (any, error) {
			gotCtx = ctx
			return &struct{}{}, nil
		},
	)

	return gotCtx, err
}

func requireAuthKey(t *testing.T, err error, key pkgerr.DomainErrorKey) {
	t.Helper()

	domainErr, ok := pkgerr.As(err)
	require.True(t, ok, "expected a domain error, got %v", err)
	assert.Equal(t, key, domainErr.Key())
}

// 关闭时不读 metadata、不查库、也不改 ctx：这就是「默认关闭等于行为不变」。
func TestAuth_DisabledPassesThrough(t *testing.T) {
	t.Parallel()

	// 故意带上一个坏 key：关闭时它必须被完全忽略。
	badKeyCtx := incomingCtx("authorization", "not-a-bearer-token")

	t.Run("nil cfg", func(t *testing.T) {
		t.Parallel()

		ctx, err := invoke(t, nil, badKeyCtx)
		require.NoError(t, err)
		require.NotNil(t, ctx)

		_, ok := auth.NamespaceFromContext(ctx)
		assert.False(t, ok, "disabled auth must not put a namespace into ctx")
	})

	t.Run("Enabled is false", func(t *testing.T) {
		t.Parallel()

		store := &fakeNamespaceStore{}
		ctx, err := invoke(t, &AuthConfig{Enabled: false, Namespaces: store}, badKeyCtx)
		require.NoError(t, err)
		require.NotNil(t, ctx)

		_, ok := auth.NamespaceFromContext(ctx)
		assert.False(t, ok, "disabled auth must not put a namespace into ctx")
		assert.Zero(t, store.calls, "disabled auth must not hit the store")
	})
}

func TestAuth_Unauthenticated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "missing authorization",
			ctx:  incomingCtx("x-trace-id", "abc"),
		},
		{
			name: "bare key without bearer scheme",
			ctx:  incomingCtx("authorization", "sk-unknown"),
		},
		{
			name: "empty token after bearer",
			ctx:  incomingCtx("authorization", "Bearer "),
		},
		{
			name: "well-formed but unknown key",
			ctx:  incomingCtx("authorization", "Bearer sk-unknown"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &AuthConfig{
				Enabled:    true,
				Namespaces: &fakeNamespaceStore{byKey: map[string]*schema.Namespace{}},
			}

			ctx, err := invoke(t, cfg, tt.ctx)
			require.Error(t, err)
			assert.Nil(t, ctx)
			requireAuthKey(t, err, pkgerr.KeyUnauthenticated)

			// 走一遍统一映射：客户端看到的 status 是 UNAUTHENTICATED 而不是 INTERNAL。
			st, ok := status.FromError(toStatusError(err))
			require.True(t, ok)
			assert.Equal(t, codes.Unauthenticated, st.Code())
			assert.Equal(t, string(pkgerr.KeyUnauthenticated), st.Message())
		})
	}
}

// store 报的不是 NoRecord 时不能当成「未认证」：那是服务端故障，要按 Internal 上报。
func TestAuth_StoreFailureIsInternal(t *testing.T) {
	t.Parallel()

	cfg := &AuthConfig{
		Enabled:    true,
		Namespaces: &fakeNamespaceStore{err: errors.New("connection refused")},
	}

	_, err := invoke(t, cfg, incomingCtx("authorization", "Bearer sk-demo"))
	require.Error(t, err)

	// 这类错误不是业务错误，因此走 toStatusError 的兜底分支：Internal + INTERNAL_ERROR。
	_, isDomain := pkgerr.As(err)
	assert.False(t, isDomain, "store failure must not be reported as a domain error")

	st, _ := status.FromError(toStatusError(err))
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, string(pkgerr.KeyInternalError), st.Message())
}

// key 反查到空 namespace 说明数据坏了，必须 fail closed，不能当成「没鉴权就放行」。
func TestAuth_EmptyNamespaceFailsClosed(t *testing.T) {
	t.Parallel()

	cfg := &AuthConfig{
		Enabled: true,
		Namespaces: &fakeNamespaceStore{
			byKey: map[string]*schema.Namespace{"sk-demo": {Name: ""}},
		},
	}

	ctx, err := invoke(t, cfg, incomingCtx("authorization", "Bearer sk-demo"))
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.NotEqual(t, pkgerr.KeyUnauthenticated, mustKey(t, err))

	st, _ := status.FromError(toStatusError(err))
	assert.Equal(t, codes.Internal, st.Code())
}

func TestAuth_ResolvesNamespaceIntoContext(t *testing.T) {
	t.Parallel()

	store := &fakeNamespaceStore{
		byKey: map[string]*schema.Namespace{"sk-demo": {Name: "demo"}},
	}
	cfg := &AuthConfig{Enabled: true, Namespaces: store}

	ctx, err := invoke(t, cfg, incomingCtx("authorization", "Bearer sk-demo"))
	require.NoError(t, err)

	namespace, ok := auth.NamespaceFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "demo", namespace)
	assert.Equal(t, 1, store.calls)

	// 服务层靠这两个判断区分「自己的资源」与「别人的资源」。
	assert.True(t, auth.Allows(ctx, "demo"))
	assert.False(t, auth.Allows(ctx, "other"))
}

func mustKey(t *testing.T, err error) pkgerr.DomainErrorKey {
	t.Helper()

	domainErr, ok := pkgerr.As(err)
	require.True(t, ok, "expected a domain error, got %v", err)

	return domainErr.Key()
}
