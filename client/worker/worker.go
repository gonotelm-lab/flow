package worker

import (
	"context"
	"sync"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/internal/apikey"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
)

// frequentMethodFilter 过滤高频 RPC（poll/heartbeat）的 otelgrpc span：
// 成功不产生 span，失败由 runtime 手动记录错误 span（见 recordFailureSpan）。
func frequentMethodFilter() otelgrpc.Filter {
	skip := filters.Any(
		filters.FullMethodName(workerv1.WorkerService_Poll_FullMethodName),
		filters.FullMethodName(workerv1.WorkerService_Heartbeat_FullMethodName),
	)
	return func(tag *stats.RPCTagInfo) bool {
		return !skip(tag)
	}
}

type Client struct {
	cfg Config
	// conn 是调用方给的原始连接（只用于 Close）；callConn 是注入凭据后的连接，RPC 都走它。
	conn     grpc.ClientConnInterface
	callConn grpc.ClientConnInterface
	ownsConn bool

	mu      sync.Mutex
	handler HandleFunc
	rt      *runtime.Runtime
}

// New 建立连接并返回 worker 客户端，opts 原样透传给 grpc.NewClient。
//
// 这里不校验 Config：namespace 为空时由 Start 返回 ErrEmptyNamespace。
func New(addr string, cfg Config, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithFilter(frequentMethodFilter()))),
	}
	baseOpts = append(baseOpts, opts...)

	conn, err := grpc.NewClient(addr, baseOpts...)
	if err != nil {
		return nil, err
	}

	c := NewWithConn(conn, cfg)
	c.ownsConn = true
	return c, nil
}

// NewWithConn 用调用方自己的连接（多 worker 共享连接时用）。
// Config.APIKey 非空时，所有 RPC 都会自动带上 `authorization: Bearer <key>`。
func NewWithConn(conn grpc.ClientConnInterface, cfg Config) *Client {
	callConn := apikey.WrapConn(conn, cfg.APIKey)

	return &Client{
		cfg:      ConfigWithDefaults(cfg),
		conn:     conn,
		callConn: callConn,
	}
}

func (c *Client) Handle(fn HandleFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = fn
}

func (c *Client) Start() error {
	// 配置校验放在这里而不是 New：Start 才是第一个会发 RPC 的入口。
	if err := c.cfg.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rt != nil {
		return nil
	}
	if c.handler == nil {
		panic("worker: Handle must be called before Start")
	}

	handler := c.handler
	c.rt = runtime.New(runtime.RuntimeConfig{
		Conn:              c.callConn,
		Namespace:         c.cfg.Namespace,
		TaskType:          c.cfg.TaskType,
		Name:              c.cfg.Name,
		MaxConcurrency:    c.cfg.MaxConcurrency,
		HeartbeatInterval: c.cfg.HeartbeatInterval,
		Handler:           c.adaptHandler(handler),
		Logger:            c.cfg.Logger,
		OwnsConn:          false,
		TraceMode:         c.cfg.TraceMode,
	})
	return c.rt.Start(context.Background())
}

func (c *Client) Run(ctx context.Context) error {
	if err := c.Start(); err != nil {
		return err
	}

	<-ctx.Done()
	return c.Close()
}

func (c *Client) Close() error {
	c.mu.Lock()
	rt := c.rt
	conn := c.conn
	ownsConn := c.ownsConn
	c.rt = nil
	c.mu.Unlock()

	var err error
	if rt != nil {
		err = rt.Stop(context.Background())
	}
	if ownsConn {
		if cc, ok := conn.(*grpc.ClientConn); ok {
			_ = cc.Close()
		}
	}
	return err
}

func (c *Client) adaptHandler(fn HandleFunc) runtime.TaskHandler {
	return func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
		result, err := fn(ctx, task.GetPayload())
		action, payload := ResolveReport(result, err)
		skipRetry := false
		if er, ok := result.(ErrorResult); ok && er.SkipRetry {
			skipRetry = true
		}
		return action, payload, skipRetry
	}
}
