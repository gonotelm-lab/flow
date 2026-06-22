package interceptor

import (
	"context"

	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
)

func errorUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		resp, err = handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		return resp, toStatusError(err)
	}
}

func errorStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if err := handler(srv, ss); err != nil {
			return toStatusError(err)
		}

		return nil
	}
}

func toStatusError(err error) error {
	if err == nil {
		return nil
	}

	if _, ok := status.FromError(err); ok {
		return err
	}

	domainErr, ok := pkgerr.As(err)
	if ok {
		bizStatus := status.New(domainErr.Code(), string(domainErr.Key()))
		if detailsMsgs := domainErr.Details(); len(detailsMsgs) > 0 {
			details := make([]protoadapt.MessageV1, 0, len(detailsMsgs))
			for _, detailMsg := range detailsMsgs {
				details = append(details, &errdetails.LocalizedMessage{
					Message: detailMsg,
				})
			}

			bizStatus, _ = bizStatus.WithDetails(details...)
		}

		return bizStatus.Err()
	}

	errStatus := status.New(codes.Internal, string(srverr.KeyInternalError))
	errStatus, _ = errStatus.WithDetails(
		&errdetails.LocalizedMessage{
			Message: err.Error(),
		},
	)

	return errStatus.Err()
}
