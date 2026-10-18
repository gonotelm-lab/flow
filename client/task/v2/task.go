// Package task 是 Flow 任务队列的客户端 SDK（v2）。
//
// 和 v1 的区别：namespace 与 api_key 由 Config 在构造时绑定，Submit 不用再传 namespace；
// 返回值是 SDK 自己的 Task / State，不把 proto 类型暴露给调用方。
package task

import (
	"context"

	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/gonotelm-lab/flow/client/internal/apikey"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client 是任务客户端，一个 Client 对应一个 namespace。
type Client struct {
	conn     grpc.ClientConnInterface
	client   taskv1.TaskServiceClient
	cfg      Config
	ownsConn bool
}

// New 建立连接并返回客户端，opts 原样透传给 grpc.NewClient。
//
// 这里不校验 Config：namespace 为空时，由每个方法在调用前返回 ErrEmptyNamespace。
func New(addr string, cfg Config, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
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

// NewWithConn 用调用方自己的连接（多客户端共享一个连接时用）。
//
// 和 schedule / worker 一样不返回 error，配置校验放到调用时做。
func NewWithConn(conn grpc.ClientConnInterface, cfg Config) *Client {
	return &Client{
		conn:   conn,
		client: taskv1.NewTaskServiceClient(apikey.WrapConn(conn, cfg.APIKey)),
		cfg:    cfg,
	}
}

// Submit 提交一个任务，namespace 取 Config.Namespace。
func (c *Client) Submit(
	ctx context.Context,
	taskType string,
	payload []byte,
	opts ...SubmitOption,
) (*Task, error) {
	if err := c.cfg.validate(); err != nil {
		return nil, err
	}

	o := &submitOptions{}
	for _, opt := range opts {
		opt(o)
	}

	resp, err := c.client.Submit(ctx, &taskv1.SubmitRequest{
		Namespace: c.cfg.Namespace,
		TaskType:  taskType,
		Payload:   payload,
		MaxRetry:  int64(o.maxRetry),
	})
	if err != nil {
		return nil, err
	}

	return taskFromProto(resp.GetTask()), nil
}

// Get 按 id 读取任务；别人的 id 表现为不存在。
func (c *Client) Get(ctx context.Context, taskID string) (*Task, error) {
	if err := c.cfg.validate(); err != nil {
		return nil, err
	}

	resp, err := c.client.Get(ctx, &taskv1.GetRequest{Id: taskID})
	if err != nil {
		return nil, err
	}

	return taskFromProto(resp.GetTask()), nil
}

// Cancel 取消一个任务。
func (c *Client) Cancel(ctx context.Context, taskID string) error {
	if err := c.cfg.validate(); err != nil {
		return err
	}

	_, err := c.client.Cancel(ctx, &taskv1.CancelRequest{Id: taskID})
	return err
}

// Close 关闭由 New 建立的连接；NewWithConn 建的客户端不动调用方的连接。
func (c *Client) Close() error {
	if !c.ownsConn {
		return nil
	}

	if cc, ok := c.conn.(*grpc.ClientConn); ok {
		return cc.Close()
	}

	return nil
}
