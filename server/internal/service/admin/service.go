package admin

import (
	"context"
	"strings"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"

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
		return nil, srverr.InvalidArgument.WithDetail("request namespace is empty")
	}

	if name := strings.TrimSpace(req.GetNamespace().GetName()); name == "" {
		return nil, srverr.InvalidArgument.WithDetail("namespace name is empty")
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
		return nil, srverr.InvalidArgument
	}

	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, srverr.InvalidArgument
	}

	ns, err := s.getNamespace(ctx, name)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	return ns, nil
}
