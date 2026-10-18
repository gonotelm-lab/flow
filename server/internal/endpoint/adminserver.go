package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	"github.com/gonotelm-lab/flow/server/internal/audit"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/endpoint/interceptor"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/service/admin"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	workerCfg *config.WorkerConfig,
	repoStore *repository.Store,
	auditRec *audit.Recorder,
) (*AdminServer, error) {
	s := &AdminServer{
		rootCtx: ctx,
		cfg:     cfg,
	}
	if err := s.init(workerCfg, repoStore, auditRec); err != nil {
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

func (s *AdminServer) init(
	workerCfg *config.WorkerConfig,
	repoStore *repository.Store,
	auditRec *audit.Recorder,
) error {
	const unixSocketPath = "/tmp/flow-admin.sock"
	os.Remove(unixSocketPath)
	var err error
	s.adminGrpcListener, err = net.Listen("unix", unixSocketPath)
	if err != nil {
		return err
	}

	// 运维面不开客户端面鉴权：Auth 留 nil，这条链和加鉴权之前一样。
	s.adminGrpcServer = grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		interceptor.UnaryServerInterceptor(interceptor.Options{}),
	)

	var workerTTL time.Duration
	if workerCfg != nil {
		workerTTL = workerCfg.WorkerTTL
	}
	adminService := admin.NewService(repoStore, auditRec, admin.ServiceConfig{WorkerTTL: workerTTL})
	adminv1.RegisterAdminServiceServer(s.adminGrpcServer, adminService)

	conn, err := grpc.NewClient(
		fmt.Sprintf("unix:///%s", unixSocketPath),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return err
	}

	s.proxyConn = conn
	// 透传调用方 IP / UA，供审计日志记录。
	//
	// Operator 字段按设计留空（当前没有用户体系），所以 source_ip 与 user_agent 是审计
	// 记录里唯一非自填的痕迹。grpc-gateway 默认只映射一小撮固定头，X-Forwarded-For
	// 不在其中，必须显式放行。
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			switch strings.ToLower(key) {
			case "x-forwarded-for", "x-real-ip", "user-agent":
				return key, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
	)
	err = adminv1.RegisterAdminServiceHandler(s.rootCtx, mux, s.proxyConn)
	if err != nil {
		return err
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Http.Port),
		Handler: otelhttp.NewHandler(mux, "flow.admin.http"),
	}

	return nil
}
