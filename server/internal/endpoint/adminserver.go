package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/endpoint/interceptor"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/service/admin"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AdminServer struct {
	rootCtx context.Context
	cfg     *config.ApiServer

	adminGrpcListener net.Listener
	adminGrpcServer   *grpc.Server

	httpServer *http.Server
	proxyConn  *grpc.ClientConn
}

func NewAdminServer(
	ctx context.Context,
	cfg *config.ApiServer,
	repoStore *repository.Store,
) (*AdminServer, error) {
	s := &AdminServer{
		rootCtx: ctx,
		cfg:     cfg,
	}
	if err := s.init(repoStore); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AdminServer) RegisterWithGroup(eg *errgroup.Group) {
	eg.Go(func() error {
		slog.Info(fmt.Sprintf("admin grpc server listening on: %s", s.adminGrpcListener.Addr()))
		return s.adminGrpcServer.Serve(s.adminGrpcListener)
	})

	eg.Go(func() error {
		slog.Info(fmt.Sprintf("http server listening on: %s", s.httpServer.Addr))
		return s.httpServer.ListenAndServe()
	})
}

func (s *AdminServer) Stop() {
	slog.Info("closing grpc-gateway proxy conn")
	if err := s.proxyConn.Close(); err != nil {
		slog.Error("failed to close proxy connection", slog.Any("err", err))
	}

	slog.Info("closing http server")
	if err := s.httpServer.Close(); err != nil {
		slog.Error("failed to close http server", slog.Any("err", err))
	}

	slog.Info("stopping admin grpc server")
	s.adminGrpcServer.GracefulStop()
	slog.Info("admin server stopped")
}

func (s *AdminServer) init(repoStore *repository.Store) error {
	const unixSocketPath = "/tmp/flow-admin.sock"
	os.Remove(unixSocketPath)
	var err error
	s.adminGrpcListener, err = net.Listen("unix", unixSocketPath)
	if err != nil {
		return err
	}

	s.adminGrpcServer = grpc.NewServer(interceptor.UnaryServerInterceptor())

	adminService := admin.NewService(repoStore)
	adminv1.RegisterAdminServiceServer(s.adminGrpcServer, adminService)

	conn, err := grpc.NewClient(
		fmt.Sprintf("unix:///%s", unixSocketPath),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}

	s.proxyConn = conn
	mux := runtime.NewServeMux()
	err = adminv1.RegisterAdminServiceHandler(s.rootCtx, mux, s.proxyConn)
	if err != nil {
		return err
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Http.Port),
		Handler: mux,
	}

	return nil
}
