package admin

import (
	"context"
	"strings"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Service struct {
	adminv1.UnimplementedAdminServiceServer

	store *repository.Store
}

func NewService(
	store *repository.Store,
) adminv1.AdminServiceServer {
	return &Service{
		store: store,
	}
}

func (s *Service) CreateNamespace(
	ctx context.Context,
	req *adminv1.CreateNamespaceRequest,
) (*schemav1.Namespace, error) {
	if req == nil || req.GetNamespace() == nil {
		return nil, pkgerr.InvalidArgument.WithDetail("request namespace is empty")
	}

	if name := strings.TrimSpace(req.GetNamespace().GetName()); name == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("namespace name is empty")
	}

	ns, err := s.createNamespace(ctx, req.GetNamespace())
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create namespace")
	}

	return ns, nil
}

func (s *Service) GetNamespace(
	ctx context.Context,
	req *adminv1.GetNamespaceRequest,
) (*schemav1.Namespace, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, pkgerr.InvalidArgument
	}

	ns, err := s.getNamespace(ctx, name)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	return ns, nil
}

func (s *Service) ListNamespaces(
	ctx context.Context,
	req *adminv1.ListNamespacesRequest,
) (*adminv1.ListNamespacesResponse, error) {
	page, pageSize := normalizePage(req.GetPage())

	nsList, total, err := s.listNamespaces(ctx, page, pageSize)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list namespaces")
	}

	return &adminv1.ListNamespacesResponse{
		Page: &adminv1.PageResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalCount: total,
		},
		Namespaces: nsList,
	}, nil
}

func (s *Service) UpdateNamespace(
	ctx context.Context,
	req *adminv1.UpdateNamespaceRequest,
) (*schemav1.Namespace, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, pkgerr.InvalidArgument.WithDetail("namespace name is required")
	}

	ns, err := s.updateNamespace(ctx, name, req.GetDescription(), req.GetCreator())
	if err != nil {
		return nil, errors.WithMessage(err, "failed to update namespace")
	}

	return ns, nil
}

func (s *Service) ListTasks(
	ctx context.Context,
	req *adminv1.ListTasksRequest,
) (*adminv1.ListTasksResponse, error) {
	page, pageSize := normalizePage(req.GetPage())

	var state string
	if req.GetState() != schemav1.TaskState_TASK_STATE_UNSPECIFIED {
		state = req.GetState().String()
	}

	tasks, total, err := s.listTasks(ctx, page, pageSize, req.GetNamespace(), req.GetTaskType(), state)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list tasks")
	}

	return &adminv1.ListTasksResponse{
		Page: &adminv1.PageResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalCount: total,
		},
		Tasks: tasks,
	}, nil
}

func (s *Service) GetTask(
	ctx context.Context,
	req *adminv1.GetTaskRequest,
) (*schemav1.Task, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	task, err := s.getTask(ctx, req.GetId())
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get task")
	}

	return task, nil
}

func (s *Service) CancelTask(
	ctx context.Context,
	req *adminv1.CancelTaskRequest,
) (*emptypb.Empty, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	if err := s.cancelTask(ctx, req.GetId()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) DeleteTask(
	ctx context.Context,
	req *adminv1.DeleteTaskRequest,
) (*emptypb.Empty, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	if err := s.deleteTask(ctx, req.GetId()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) ListWorkers(
	ctx context.Context,
	req *adminv1.ListWorkersRequest,
) (*adminv1.ListWorkersResponse, error) {
	page, pageSize := normalizePage(req.GetPage())

	workers, total, err := s.listWorkers(ctx, page, pageSize, req.GetNamespace(), req.GetTaskType())
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list workers")
	}

	return &adminv1.ListWorkersResponse{
		Page: &adminv1.PageResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalCount: total,
		},
		Workers: workers,
	}, nil
}

func (s *Service) GetWorker(
	ctx context.Context,
	req *adminv1.GetWorkerRequest,
) (*schemav1.Worker, error) {
	if req == nil {
		return nil, pkgerr.InvalidArgument
	}

	worker, err := s.getWorker(ctx, req.GetId())
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get worker")
	}

	return worker, nil
}

func (s *Service) ListTaskEvents(
	ctx context.Context,
	req *adminv1.ListTaskEventsRequest,
) (*adminv1.ListTaskEventsResponse, error) {
	page, pageSize := normalizePage(req.GetPage())

	events, total, err := s.listTaskEvents(ctx, req.GetTaskId(), page, pageSize)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to list task events")
	}

	return &adminv1.ListTaskEventsResponse{
		Page: &adminv1.PageResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalCount: total,
		},
		Events: events,
	}, nil
}
