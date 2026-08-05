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
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// workerFrequentFilter 过滤 worker 高频 RPC（poll/heartbeat）的 server span，避免 trace 量过大。
func workerFrequentFilter() otelgrpc.Filter {
	skip := filters.Any(
		filters.FullMethodName(workerv1.WorkerService_Poll_FullMethodName),
		filters.FullMethodName(workerv1.WorkerService_Heartbeat_FullMethodName),
	)
	return func(tag *stats.RPCTagInfo) bool {
		return !skip(tag)
	}
}

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
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithFilter(workerFrequentFilter()))),
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
