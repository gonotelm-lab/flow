package adminservice

import (
	"context"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	return nil, status.Error(codes.Unimplemented, "method CreateNamespace not implemented")
}

func (s *Service) GetNamespace(
	ctx context.Context,
	req *adminv1.GetNamespaceRequest,
) (*schemav1.Namespace, error) {
	return nil, status.Error(codes.Unimplemented, "method GetNamespace not implemented")
}
