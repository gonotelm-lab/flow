package task

import (
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

type Service struct {
	taskv1.UnimplementedTaskServiceServer

	repo *repository.Store
}

func NewService(repo *repository.Store) taskv1.TaskServiceServer {
	return &Service{repo: repo}
}
