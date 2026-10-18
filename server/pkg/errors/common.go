package errors

import (
	"google.golang.org/grpc/codes"
)

const (
	KeyInvalidArgument    DomainErrorKey = "INVALID_ARGUMENT"
	KeyNoRecord           DomainErrorKey = "NO_RECORD"
	KeyDuplicatedResource DomainErrorKey = "DUPLICATED_RESOURCE"
	KeyInternalError      DomainErrorKey = "INTERNAL_ERROR"
	KeyUnauthenticated    DomainErrorKey = "UNAUTHENTICATED"
	KeyPermissionDenied   DomainErrorKey = "PERMISSION_DENIED"
)

var (
	InvalidArgument    = New(codes.InvalidArgument, KeyInvalidArgument)
	NoRecord           = New(codes.NotFound, KeyNoRecord)
	DuplicatedResource = New(codes.AlreadyExists, KeyDuplicatedResource)
	Internal           = New(codes.Internal, KeyInternalError)

	// Unauthenticated 没带凭据、格式不对，或者 key 不认识。
	Unauthenticated = New(codes.Unauthenticated, KeyUnauthenticated)
	// PermissionDenied 凭据有效，但请求里的 namespace 不是它所属的那个。
	PermissionDenied = New(codes.PermissionDenied, KeyPermissionDenied)
)
