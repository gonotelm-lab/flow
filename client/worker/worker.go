// flow/client/worker/worker.go
package worker

import (
	"context"
	"sync"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	cfg     Config
	conn    grpc.ClientConnInterface
	ownsConn bool

	mu      sync.Mutex
	handler HandleFunc
	rt      *runtime.Runtime
}

func New(addr string, cfg Config, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	baseOpts = append(baseOpts, opts...)

	conn, err := grpc.NewClient(addr, baseOpts...)
	if err != nil {
		return nil, err
	}

	c := NewWithConn(conn, cfg)
	c.ownsConn = true
	return c, nil
}

func NewWithConn(conn grpc.ClientConnInterface, cfg Config) *Client {
	return &Client{
		cfg:  ConfigWithDefaults(cfg),
		conn: conn,
	}
}

func (c *Client) Handle(fn HandleFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = fn
}

func (c *Client) Start() error {
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
		Conn:              c.conn,
		Namespace:         c.cfg.Namespace,
		TaskType:          c.cfg.TaskType,
		Name:              c.cfg.Name,
		MaxConcurrency:    c.cfg.MaxConcurrency,
		HeartbeatInterval: c.cfg.HeartbeatInterval,
		Handler:           c.adaptHandler(handler),
		Logger:            c.cfg.Logger,
		OwnsConn:          false,
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
	return func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte) {
		result, err := fn(ctx, task.GetPayload())
		action, payload := ResolveReport(result, err)
		return action, payload
	}
}
