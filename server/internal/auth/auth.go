// Package auth 负责客户端面 gRPC 的 namespace api_key 鉴权：拦截器把
// `authorization: Bearer <key>` 解析成 namespace 放进 context，服务层用 Allows 判断归属。
//
// 鉴权关闭时 context 里没有 namespace，Allows 一律返回 true，服务层不用写额外分支。
package auth

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/metadata"
)

// MetadataKey 是客户端携带凭据的 gRPC metadata 键。
const MetadataKey = "authorization"

const bearerScheme = "bearer"

// MismatchDetail 是「请求里的 namespace 和凭据对不上」时对外的固定文案。
// 不回显任何 namespace 名，免得有人拿它探测。
const MismatchDetail = "api key does not belong to the requested namespace"

var (
	// ErrMissingKey metadata 里没有 authorization。
	ErrMissingKey = errors.New("auth: authorization metadata is missing")
	// ErrMalformedKey authorization 不是 "Bearer <key>" 形式。
	ErrMalformedKey = errors.New("auth: authorization metadata is not a bearer token")
	// ErrEmptyKey Bearer 后面的 key 是空的。
	ErrEmptyKey = errors.New("auth: api key is empty")
)

// BearerToken 从 metadata 里取出 Bearer 的凭据部分。只认 Bearer 一种写法。
func BearerToken(md metadata.MD) (string, error) {
	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return "", ErrMissingKey
	}

	// 只裁左边的空白：右边也裁掉的话，"Bearer " 会变成没有分隔符的 "Bearer"。
	scheme, token, ok := strings.Cut(strings.TrimLeft(values[0], " \t"), " ")
	if !ok || !strings.EqualFold(scheme, bearerScheme) {
		return "", ErrMalformedKey
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrEmptyKey
	}

	return token, nil
}

// ctxKey 是 namespace 在 context 里的私有键，外部无法伪造。
type ctxKey struct{}

// WithNamespace 把已认证的 namespace 放进 context，只应由鉴权拦截器调用。
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, ctxKey{}, namespace)
}

// NamespaceFromContext 取出已认证的 namespace；鉴权关闭时 ok 为 false。
func NamespaceFromContext(ctx context.Context) (string, bool) {
	namespace, ok := ctx.Value(ctxKey{}).(string)
	if !ok || namespace == "" {
		return "", false
	}

	return namespace, true
}

// Allows 判断调用方能不能操作某个 namespace 下的资源。context 里没有 namespace
// （鉴权关闭，或者请求来自运维面）时一律返回 true。
//
// 「认证成功但 namespace 为空」必须拒绝，不能让空值落到「没鉴权就放行」那条分支。
func Allows(ctx context.Context, namespace string) bool {
	caller, ok := ctx.Value(ctxKey{}).(string)
	if !ok {
		return true
	}

	return caller != "" && caller == namespace
}
