package error

import (
	"github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyInvalidArgument errors.DomainErrorKey = "INVALID_ARGUMENT"
	KeyInternalError   errors.DomainErrorKey = "INTERNAL_ERROR"
)

var (
	InvalidArgument = errors.New(codes.InvalidArgument, KeyInvalidArgument)
	Internal        = errors.New(codes.Internal, KeyInternalError)
)
