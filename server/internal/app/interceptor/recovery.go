package interceptor

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"
)

func recoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return recovery.UnaryServerInterceptor(
		recovery.WithRecoveryHandlerContext(
			func(ctx context.Context, p interface{}) (err error) {
				slog.ErrorContext(ctx,
					"grpc server panic", slog.Any("err", p),
					slog.String("stack", string(debug.Stack())),
				)

				return status.Error(codes.Internal, string(srverr.KeyInternalError))
			},
		),
	)
}

func recoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return recovery.StreamServerInterceptor(
		recovery.WithRecoveryHandlerContext(
			func(ctx context.Context, p interface{}) (err error) {
				slog.ErrorContext(ctx,
					"grpc server panic", slog.Any("err", p),
					slog.String("stack", string(debug.Stack())),
				)

				return status.Error(codes.Internal, string(srverr.KeyInternalError))
			},
		),
	)
}
