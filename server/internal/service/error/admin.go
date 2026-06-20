package error

import (
	"github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyNamespaceNotFound errors.DomainErrorKey = "NAMESPACE_NOT_FOUND"
	KeyNamespaceExists   errors.DomainErrorKey = "NAMESPACE_ALREADY_EXISTS"
)

var (
	NamespaceNotFound = errors.New(codes.NotFound, KeyNamespaceNotFound)
	NamespaceExists   = errors.New(codes.AlreadyExists, KeyNamespaceExists)
)
