package task

import (
	"context"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client 是 v1 任务客户端。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 Client。
type Client struct {
	conn     grpc.ClientConnInterface
	client   taskv1.TaskServiceClient
	ownsConn bool
}

// New 建立连接并返回 v1 客户端。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 New（Config 绑定 namespace 与 api_key）。
func New(addr string, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	baseOpts = append(baseOpts, opts...)

	conn, err := grpc.NewClient(addr, baseOpts...)
	if err != nil {
		return nil, err
	}

	c := NewWithConn(conn)
	c.ownsConn = true
	return c, nil
}

// NewWithConn 用调用方自己的连接。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 NewWithConn。
func NewWithConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		conn:   conn,
		client: taskv1.NewTaskServiceClient(conn),
	}
}

// Submit 提交任务；namespace 每次调用都要传。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 Client.Submit。
func (c *Client) Submit(
	ctx context.Context,
	namespace, taskType string,
	payload []byte,
	opts ...SubmitOption,
) (*schemav1.Task, error) {
	o := &submitOptions{}
	for _, opt := range opts {
		opt(o)
	}

	resp, err := c.client.Submit(ctx, &taskv1.SubmitRequest{
		Namespace: namespace,
		TaskType:  taskType,
		Payload:   payload,
		MaxRetry:  int64(o.maxRetry),
	})
	if err != nil {
		return nil, err
	}
	return resp.GetTask(), nil
}

// Get 按 id 读取任务，返回 proto 类型。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 Client.Get（返回 SDK 自有类型）。
func (c *Client) Get(ctx context.Context, taskID string) (*schemav1.Task, error) {
	resp, err := c.client.Get(ctx, &taskv1.GetRequest{Id: taskID})
	if err != nil {
		return nil, err
	}
	return resp.GetTask(), nil
}

// Cancel 取消任务。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 Client.Cancel。
func (c *Client) Cancel(ctx context.Context, taskID string) error {
	_, err := c.client.Cancel(ctx, &taskv1.CancelRequest{Id: taskID})
	return err
}

// Close 关闭由 New 建立的连接。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 Client.Close。
func (c *Client) Close() error {
	if c.ownsConn {
		if cc, ok := c.conn.(*grpc.ClientConn); ok {
			return cc.Close()
		}
	}
	return nil
}
