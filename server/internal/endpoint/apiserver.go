package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/endpoint/interceptor"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/service/task"
	"github.com/gonotelm-lab/flow/server/internal/service/worker"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type ApiServer struct {
	rootCtx context.Context
	cfg     *config.ApiServer

	grpcListener net.Listener
	grpcServer   *grpc.Server

	adminServer *AdminServer
}

func NewApiServer(
	ctx context.Context,
	cfg *config.ApiServer,
	repoStore *repository.Store,
) (*ApiServer, error) {
	listenAddr := fmt.Sprintf("%s:%d", cfg.Grpc.Listen, cfg.Grpc.Port)
	grpcListener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(
		interceptor.UnaryServerInterceptor(),
		interceptor.StreamServerInterceptor(),
	)

	adminServer, err := NewAdminServer(ctx, cfg, repoStore)
	if err != nil {
		return nil, err
	}

	apiServer := &ApiServer{
		rootCtx:      ctx,
		cfg:          cfg,
		grpcListener: grpcListener,
		grpcServer:   grpcServer,
		adminServer:  adminServer,
	}
	apiServer.registerGrpcServices(repoStore)

	return apiServer, nil
}

func (s *ApiServer) Spin() error {
	var eg errgroup.Group
	eg.Go(func() error {
		slog.Info(fmt.Sprintf("grpc server listening on: %s", s.grpcListener.Addr().String()))
		return s.grpcServer.Serve(s.grpcListener)
	})

	s.adminServer.RegisterWithGroup(&eg)

	return eg.Wait()
}

func (s *ApiServer) Stop() {
	slog.Info("stopping grpc server")
	s.grpcServer.GracefulStop()
	s.adminServer.Stop()
	slog.Info("api server stopped")
}

func (s *ApiServer) registerGrpcServices(repoStore *repository.Store) {
	var workerCfg worker.ServiceConfig
	if config.Conf != nil && config.Conf.Worker != nil {
		workerCfg = worker.ServiceConfig{
			PollWait:          config.Conf.Worker.PollWait,
			PollCheckInterval: config.Conf.Worker.PollCheckInterval,
		}
	}
	workerService := worker.NewService(repoStore, workerCfg)
	workerv1.RegisterWorkerServiceServer(s.grpcServer, workerService)

	taskService := task.NewService(repoStore)
	taskv1.RegisterTaskServiceServer(s.grpcServer, taskService)
}
