package errors

import (
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyTaskNotFound     pkgerr.DomainErrorKey = "TASK_NOT_FOUND"
	KeyTaskAlreadyEnded pkgerr.DomainErrorKey = "TASK_ALREADY_ENDED"
)

var (
	TaskNotFound     = pkgerr.New(codes.NotFound, KeyTaskNotFound)
	TaskAlreadyEnded = pkgerr.New(codes.FailedPrecondition, KeyTaskAlreadyEnded)
)
