package worker

import (
	"context"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"

	"github.com/pkg/errors"
)

func (s *Service) register(ctx context.Context, worker *schemav1.Worker) error {
	namespace := worker.GetNamespace()
	_, err := s.repo.Namespace.Get(ctx, namespace)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
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
		return errors.WithMessagef(err, "failed to create task worker %s/%s", namespace, worker.GetTaskType())
	}

	worker.Id = res.Id

	return nil
}

func (s *Service) unregister(ctx context.Context, workerId int64) error {
	err := s.repo.TaskWorker.Delete(ctx, workerId)
	if err != nil {
		return errors.WithMessagef(err, "failed to delete task worker %d", workerId)
	}

	return nil
}

func (s *Service) heartbeat(ctx context.Context, workerId int64) (int64, error) {
	heartbeatTime := time.Now().UnixMilli()
	_, err := s.repo.TaskWorker.UpdateHeartbeat(ctx, workerId, heartbeatTime)
	if err != nil {
		return 0, errors.WithMessagef(err, "failed to update task worker heartbeat %d", workerId)
	}

	return heartbeatTime, nil
}

func (s *Service) poll(
	ctx context.Context,
	workerId int64,
	namespace string,
	taskType string,
) (*reposchema.Task, error) {
	oldState := schemav1.TaskState_INITED.String()
	newState := schemav1.TaskState_RUNNING.String()

	task, err := s.repo.Task.Claim(
		ctx,
		namespace,
		taskType,
		[]string{oldState},
	)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, nil
		}
		return nil, errors.WithMessagef(err, "failed to claim task in %s/%s", namespace, taskType)
	}

	nowMilli := time.Now().UnixMilli()
	ok, err := s.repo.Task.ClaimUpdate(
		ctx,
		task.Id,
		oldState,
		&store.TaskClaimUpdateParams{
			WorkerId:   workerId,
			NewState:   newState,
			UpdateTime: nowMilli,
		},
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to update task state %s", task.Id)
	}
	if !ok {
		return nil, nil
	}

	task.WorkerId = workerId
	task.State = newState
	task.UpdateTime = nowMilli

	return task, nil
}

func (s *Service) report(ctx context.Context, req *workerv1.ReportRequest) error {
	taskId, err := uuid.Parse(req.GetTaskId())
	if err != nil {
		return pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	var (
		success  bool
		newState string
		oldState = schemav1.TaskState_RUNNING.String()
		nowMilli = time.Now().UnixMilli()
	)

	switch req.GetAction() {
	case workerv1.ReportAction_SUCCESS:
		success = true
		newState = schemav1.TaskState_DONE.String()
	case workerv1.ReportAction_FAIL:
		success = false
		newState = schemav1.TaskState_FAILED.String()
	default:
		return pkgerr.InvalidArgument.WithDetail("action is invalid")
	}

	_, err = s.repo.Task.UpdateOutcome(
		ctx,
		taskId,
		success,
		req.GetWorkerId(),
		oldState,
		newState,
		&store.TaskUpdateOutcomeParams{
			Payload:    req.GetPayload(),
			UpdateTime: nowMilli,
		},
	)
	if err != nil {
		return errors.WithMessagef(err, "failed to update task outcome %s", taskId)
	}

	return nil
}
