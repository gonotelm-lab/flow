package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	"github.com/gonotelm-lab/flow/server/internal/app/interceptor"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/service/adminservice"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ApiServer struct {
	rootCtx context.Context
	cfg     *config.ApiServer

	grpcListener net.Listener
	grpcServer   *grpc.Server

	httpServer *http.Server
	proxyConn  *grpc.ClientConn
}

func NewApiServer(
	ctx context.Context,
	cfg *config.ApiServer,
	repoStore *repository.Store,
) (*ApiServer, error) {
	// grpc server
	listenAddr := fmt.Sprintf("%s:%d", cfg.Grpc.Listen, cfg.Grpc.Port)
	grpcListener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(
		interceptor.UnaryServerInterceptor(),
		interceptor.StreamServerInterceptor(),
	)
	apiServer := &ApiServer{
		rootCtx:      ctx,
		cfg:          cfg,
		grpcListener: grpcListener,
		grpcServer:   grpcServer,
	}
	apiServer.registerGrpcServices(repoStore)
	err = apiServer.registerHttpGrpcGateway()
	if err != nil {
		return nil, err
	}

	return apiServer, nil
}

// block until server is stopped
func (s *ApiServer) Spin() error {
	var eg errgroup.Group
	eg.Go(func() error {
		slog.Info(fmt.Sprintf("grpc server listening on: %s", s.grpcListener.Addr().String()))
		return s.grpcServer.Serve(s.grpcListener)
	})

	eg.Go(func() error {
		slog.Info(fmt.Sprintf("http server listening on: %s", s.httpServer.Addr))
		return s.httpServer.ListenAndServe()
	})

	return eg.Wait()
}

func (s *ApiServer) Stop() {
	slog.Info("closing grpc-gateway proxy conn")
	if err := s.proxyConn.Close(); err != nil {
		slog.Error("failed to close proxy connection", slog.Any("err", err))
	}
	slog.Info("closing http server")
	if err := s.httpServer.Close(); err != nil {
		slog.Error("failed to close http server", slog.Any("err", err))
	}

	slog.Info("stopping grpc server")
	s.grpcServer.GracefulStop()

	slog.Info("api server stopped")
}

func (s *ApiServer) registerGrpcServices(
	repoStore *repository.Store,
) {
	adminService := adminservice.NewService(repoStore)

	adminv1.RegisterAdminServiceServer(s.grpcServer, adminService)
}

func (s *ApiServer) registerHttpGrpcGateway() error {
	addr := fmt.Sprintf("%s:%d", "localhost", s.cfg.Grpc.Port)
	conn, err := grpc.NewClient(addr,
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
