package worker

import (
	"context"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/pkg/errors"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultPollWait          = 10 * time.Second
	defaultPollCheckInterval = 500 * time.Millisecond
)

type ServiceConfig struct {
	PollWait          time.Duration
	PollCheckInterval time.Duration
}

type Service struct {
	workerv1.UnimplementedWorkerServiceServer

	repo              *repository.Store
	pollWait          time.Duration
	pollCheckInterval time.Duration
}

func NewService(repo *repository.Store, cfg ServiceConfig) workerv1.WorkerServiceServer {
	pollWait := defaultPollWait
	if cfg.PollWait > 0 {
		pollWait = cfg.PollWait
	}
	pollCheckInterval := defaultPollCheckInterval
	if cfg.PollCheckInterval > 0 {
		pollCheckInterval = cfg.PollCheckInterval
	}

	return &Service{
		repo:              repo,
		pollWait:          pollWait,
		pollCheckInterval: pollCheckInterval,
	}
}

func (s *Service) Register(ctx context.Context, req *schemav1.Worker) (*schemav1.Worker, error) {
	if req.GetNamespace() == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("namespace is required")
	}
	if req.GetTaskType() == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("task_type is required")
	}

	err := s.register(ctx, req)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to register worker")
	}

	return req, nil
}

func (s *Service) Unregister(
	ctx context.Context,
	req *workerv1.UnregisterRequest,
) (*workerv1.UnregisterResponse, error) {
	if req.GetId() == 0 {
		return nil, pkgerr.InvalidArgument.WithDetail("id is required")
	}

	err := s.unregister(ctx, req.GetId())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to unregister worker %d", req.GetId())
	}

	return &workerv1.UnregisterResponse{}, nil
}

func (s *Service) Heartbeat(
	ctx context.Context,
	req *workerv1.HeartbeatRequest,
) (*workerv1.HeartbeatResponse, error) {
	if req.GetId() == 0 {
		return nil, pkgerr.InvalidArgument.WithDetail("id is required")
	}

	heartbeatMs, err := s.heartbeat(ctx, req.GetId())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to heartbeat worker %d", req.GetId())
	}

	return &workerv1.HeartbeatResponse{
		HeartbeatTime: timestamppb.New(time.UnixMilli(heartbeatMs)),
	}, nil
}

// 长轮询
func (s *Service) Poll(
	ctx context.Context,
	req *workerv1.PollRequest,
) (*workerv1.PollResponse, error) {
	if req.GetId() == 0 {
		return nil, pkgerr.InvalidArgument.WithDetail("id is required")
	}
	if req.GetNamespace() == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("namespace is required")
	}
	if req.GetTaskType() == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("task_type is required")
	}

	pollCtx, cancel := context.WithTimeout(ctx, s.pollWait)
	defer cancel()

	// 先立即查询一次，避免空等一个轮询间隔
	resp, err := s.tryPoll(pollCtx, ctx, req)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp, nil
	}

	ticker := time.NewTicker(s.pollCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return nil, status.FromContextError(ctx.Err()).Err()
			}
			return &workerv1.PollResponse{}, nil
		case <-ticker.C:
			resp, err = s.tryPoll(pollCtx, ctx, req)
			if err != nil {
				return nil, err
			}
			if resp != nil {
				return resp, nil
			}
		}
	}
}

func (s *Service) tryPoll(
	pollCtx context.Context,
	requestCtx context.Context,
	req *workerv1.PollRequest,
) (*workerv1.PollResponse, error) {
	task, err := s.poll(pollCtx, req.GetId(), req.GetNamespace(), req.GetTaskType())
	if err != nil {
		// 本地长轮询超时，按空任务正常返回
		if pollCtx.Err() != nil && requestCtx.Err() == nil {
			return &workerv1.PollResponse{}, nil
		}

		// 调用方主动取消/超时，透传 context 错误
		if requestCtx.Err() != nil {
			return nil, status.FromContextError(requestCtx.Err()).Err()
		}

		return nil, errors.WithMessagef(err, "failed to poll task for worker %d", req.GetId())
	}
	if task == nil {
		return nil, nil
	}

	return &workerv1.PollResponse{Task: toProtoTask(task)}, nil
}

func (s *Service) Report(
	ctx context.Context,
	req *workerv1.ReportRequest,
) (*workerv1.ReportResponse, error) {
	if req.GetWorkerId() == 0 {
		return nil, pkgerr.InvalidArgument.WithDetail("worker_id is required")
	}

	err := s.report(ctx, req)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to report task %s", req.GetTaskId())
	}

	return &workerv1.ReportResponse{}, nil
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
		Id:          task.Id.String(),
		Namespace:   task.Namespace,
		TaskType:    task.TaskType,
		Payload:     task.Payload,
		Result:      task.Result,
		Error:       task.Error,
		State:       state,
		CreateTime:  timestamppb.New(time.UnixMilli(task.CreateTime)),
		UpdateTime:  timestamppb.New(time.UnixMilli(task.UpdateTime)),
		NextRunTime: task.NextRunTime,
		MaxRetry:    int64(task.MaxRetry),
		AttemptNo:   int32(task.AttemptNo),
		WorkerId:    task.WorkerId,
	}
}
