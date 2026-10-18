// Package apikey 是 task / schedule / worker 三个客户端包共用的凭据注入实现。
//
// 做法是包一层连接，而不是给每个方法加参数：worker 的 RPC 散在四个类型里，包一层
// 就全盖住了。也没用 PerRPCCredentials，因为它只在拨号时生效，会让 New 和
// NewWithConn 的行为对不上。
package apikey

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Header 是携带凭据的 metadata 键，到 HTTP/2 上就是 authorization 头。
const Header = "authorization"

const scheme = "Bearer "

// WithKey 把 api_key 附到 ctx 上；key 为空时原样返回。
func WithKey(ctx context.Context, key string) context.Context {
	if strings.TrimSpace(key) == "" {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, Header, scheme+key)
}

// WrapConn 返回每次调用都自动带上凭据的连接；key 为空时原样返回。
func WrapConn(conn grpc.ClientConnInterface, key string) grpc.ClientConnInterface {
	if conn == nil || strings.TrimSpace(key) == "" {
		return conn
	}

	return &credentialConn{ClientConnInterface: conn, key: key}
}

type credentialConn struct {
	grpc.ClientConnInterface
	key string
}

func (c *credentialConn) Invoke(
	ctx context.Context,
	method string,
	args, reply any,
	opts ...grpc.CallOption,
) error {
	return c.ClientConnInterface.Invoke(WithKey(ctx, c.key), method, args, reply, opts...)
}

func (c *credentialConn) NewStream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	method string,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return c.ClientConnInterface.NewStream(WithKey(ctx, c.key), desc, method, opts...)
}
