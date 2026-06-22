package worker

import (
	"context"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"github.com/pkg/errors"
)

func (s *Service) register(ctx context.Context, worker *schemav1.Worker) error {
	namespace := worker.GetNamespace()
	_, err := s.repo.Namespace.Get(ctx, namespace)
	if err != nil {
		if errors.Is(err, sql.ErrNoRecord) {
			return srverr.NamespaceNotFound
		}

		return errors.WithMessage(err, "failed to get namespace")
	}

	nowMilli := time.Now().UnixMilli()

	// insert into worker
	res, err := s.repo.TaskWorker.Create(ctx, &reposchema.TaskWorker{
		Name:          worker.GetName(),
		Namespace:     namespace,
		TaskType:      worker.GetTaskType(),
		CreateTime:    nowMilli,
		HeartbeatTime: nowMilli,
	})
	if err != nil {
		return errors.WithMessage(err, "failed to create task worker")
	}

	worker.Id = res.Id

	return nil
}

func (s *Service) unregister(ctx context.Context, workerId int64) error {
	err := s.repo.TaskWorker.Delete(ctx, workerId)
	if err != nil {
		return errors.WithMessage(err, "failed to delete task worker")
	}

	return nil
}
