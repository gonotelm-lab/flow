package errors

import (
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyNamespaceNotFound pkgerr.DomainErrorKey = "NAMESPACE_NOT_FOUND"
	KeyNamespaceExists   pkgerr.DomainErrorKey = "NAMESPACE_ALREADY_EXISTS"
	KeyWorkerNotFound    pkgerr.DomainErrorKey = "WORKER_NOT_FOUND"
)

var (
	NamespaceNotFound = pkgerr.New(codes.NotFound, KeyNamespaceNotFound)
	NamespaceExists   = pkgerr.New(codes.AlreadyExists, KeyNamespaceExists)
	WorkerNotFound    = pkgerr.New(codes.NotFound, KeyWorkerNotFound)
)
