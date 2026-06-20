package interceptor

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"google.golang.org/grpc"
)

func recoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return grpc_recovery.UnaryServerInterceptor(
		grpc_recovery.WithRecoveryHandlerContext(
			func(ctx context.Context, p interface{}) (err error) {
				slog.ErrorContext(ctx,
					"grpc server panic", slog.Any("err", p),
					slog.String("stack", string(debug.Stack())),
				)

				return nil
			},
		),
	)
}

func recoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return grpc_recovery.StreamServerInterceptor(
		grpc_recovery.WithRecoveryHandlerContext(
			func(ctx context.Context, p interface{}) (err error) {
				slog.ErrorContext(ctx,
					"grpc server panic", slog.Any("err", p),
					slog.String("stack", string(debug.Stack())),
				)

				return nil
			},
		),
	)
}

func UnaryServerInterceptor() grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(
		recoveryUnaryInterceptor(),
	)
}

func StreamServerInterceptor() grpc.ServerOption {
	return grpc.ChainStreamInterceptor(
		recoveryStreamInterceptor(),
	)
}
