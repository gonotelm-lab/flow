package admin

import (
	"context"
	"strings"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"

	"github.com/pkg/errors"
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
