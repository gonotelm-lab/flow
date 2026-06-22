package worker

import (
	"context"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	workerv1.UnimplementedWorkerServiceServer

	repo *repository.Store
}

func NewService(repo *repository.Store) workerv1.WorkerServiceServer {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Register(ctx context.Context, req *schemav1.Worker) (*schemav1.Worker, error) {
	if req.GetNamespace() == "" {
		return nil, srverr.InvalidArgument.WithDetail("namespace is required")
	}
	if req.GetTaskType() == "" {
		return nil, srverr.InvalidArgument.WithDetail("task_type is required")
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
		return nil, srverr.InvalidArgument.WithDetail("id is required")
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
	return nil, status.Errorf(codes.Unimplemented, "method Heartbeat not implemented")
}

func (s *Service) Poll(
	ctx context.Context,
	req *workerv1.PollRequest,
) (*workerv1.PollResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Poll not implemented")
}

func (s *Service) Report(
	ctx context.Context,
	req *workerv1.ReportRequest,
) (*workerv1.ReportResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Report not implemented")
}
