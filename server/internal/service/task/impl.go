package task

import (
	"context"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) Submit(
	ctx context.Context,
	req *taskv1.SubmitRequest,
) (*taskv1.SubmitResponse, error) {
	namespace := req.GetNamespace()
	_, err := s.repo.Namespace.Get(ctx, namespace)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.NamespaceNotFound
		}
		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate task id")
	}

	nowMilli := time.Now().UnixMilli()
	task := &reposchema.Task{
		Id:          id,
		Namespace:   namespace,
		TaskType:    req.GetTaskType(),
		Payload:     req.GetPayload(),
		State:       schemav1.TaskState_INITED.String(),
		CreateTime:  nowMilli,
		NextRunTime: nowMilli,
		UpdateTime:  nowMilli,
		MaxRetry:    int(req.GetMaxRetry()),
		AttemptNo:   0,
		WorkerId:    0,
	}

	created, err := s.repo.Task.Create(ctx, task)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create task")
	}

	_ = s.repo.TaskEvent.Append(ctx, &reposchema.TaskEvent{
		TaskId:     created.Id,
		EventType:  schemav1.TaskState_INITED.String(),
		CreateTime: nowMilli,
	})

	return &taskv1.SubmitResponse{Task: toProtoTask(created)}, nil
}

func (s *Service) Get(
	ctx context.Context,
	req *taskv1.GetRequest,
) (*taskv1.GetResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.repo.Task.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.TaskNotFound
		}
		return nil, errors.WithMessage(err, "failed to get task")
	}

	return &taskv1.GetResponse{Task: toProtoTask(task)}, nil
}

func (s *Service) Cancel(
	ctx context.Context,
	req *taskv1.CancelRequest,
) (*taskv1.CancelResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.repo.Task.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.TaskNotFound
		}
		return nil, errors.WithMessage(err, "failed to get task")
	}

	if task.State != schemav1.TaskState_INITED.String() &&
		task.State != schemav1.TaskState_RUNNING.String() {
		return nil, srverr.TaskAlreadyEnded.WithDetail(
			"task state: " + task.State,
		)
	}

	nowMilli := time.Now().UnixMilli()
	task.State = schemav1.TaskState_CANCELLED.String()
	task.UpdateTime = nowMilli

	ok, err := s.repo.Task.Update(ctx, task)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to cancel task")
	}
	if !ok {
		return nil, srverr.TaskAlreadyEnded
	}

	_ = s.repo.TaskEvent.Append(ctx, &reposchema.TaskEvent{
		TaskId:     task.Id,
		EventType:  schemav1.TaskState_CANCELLED.String(),
		CreateTime: nowMilli,
	})

	return &taskv1.CancelResponse{}, nil
}

func toProtoTask(task *reposchema.Task) *schemav1.Task {
	if task == nil {
		return nil
	}

	state := schemav1.TaskState_TASK_STATE_UNSPECIFIED
	if rawState, ok := schemav1.TaskState_value[task.State]; ok {
		state = schemav1.TaskState(rawState)
	}

	return &schemav1.Task{
		Id:                task.Id.String(),
		Namespace:         task.Namespace,
		TaskType:          task.TaskType,
		Payload:           task.Payload,
		Result:            task.Result,
		Error:             task.Error,
		State:             state,
		CreateTime:        timestamppb.New(time.UnixMilli(task.CreateTime)),
		UpdateTime:        timestamppb.New(time.UnixMilli(task.UpdateTime)),
		NextRunTime:       task.NextRunTime,
		MaxRetry:          int64(task.MaxRetry),
		AttemptNo:         int32(task.AttemptNo),
		WorkerId:          task.WorkerId,
		LastHeartbeatTime: timestamppb.New(time.UnixMilli(task.LastHeartbeatTime)),
	}
}
