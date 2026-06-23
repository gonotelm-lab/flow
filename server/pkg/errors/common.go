package errors

import (
	"google.golang.org/grpc/codes"
)

const (
	KeyInvalidArgument    DomainErrorKey = "INVALID_ARGUMENT"
	KeyNoRecord           DomainErrorKey = "NO_RECORD"
	KeyDuplicatedResource DomainErrorKey = "DUPLICATED_RESOURCE"
	KeyInternalError      DomainErrorKey = "INTERNAL_ERROR"
)

var (
	InvalidArgument    = New(codes.InvalidArgument, KeyInvalidArgument)
	NoRecord           = New(codes.NotFound, KeyNoRecord)
	DuplicatedResource = New(codes.AlreadyExists, KeyDuplicatedResource)
	Internal           = New(codes.Internal, KeyInternalError)
)
