package interceptor

import (
	"google.golang.org/grpc"
)

func UnaryServerInterceptor() grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(
		recoveryUnaryInterceptor(),
		errorUnaryInterceptor(),
	)
}

func StreamServerInterceptor() grpc.ServerOption {
	return grpc.ChainStreamInterceptor(
		recoveryStreamInterceptor(),
		errorStreamInterceptor(),
	)
}
