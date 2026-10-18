package interceptor

import (
	"google.golang.org/grpc"
)

// Options 是拦截器链的依赖。
type Options struct {
	// Auth 为 nil（或者 Enabled 为 false）时不鉴权；运维面显式不传。
	Auth *AuthConfig
}

// 顺序：recovery → error → auth → validate。
//
// auth 要放在 error 里面，它返回的错误才会被映射成带 key 的 status；放在 validate
// 前面，是为了没通过认证的调用方看不到参数校验的细节。
func UnaryServerInterceptor(opts Options) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(
		recoveryUnaryInterceptor(),
		errorUnaryInterceptor(),
		authUnaryInterceptor(opts.Auth),
		validateUnaryInterceptor(),
	)
}

func StreamServerInterceptor(opts Options) grpc.ServerOption {
	return grpc.ChainStreamInterceptor(
		recoveryStreamInterceptor(),
		errorStreamInterceptor(),
		authStreamInterceptor(opts.Auth),
		validateStreamInterceptor(),
	)
}
