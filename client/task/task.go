package task

import (
	"context"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn     grpc.ClientConnInterface
	client   taskv1.TaskServiceClient
	ownsConn bool
}

func New(addr string, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	baseOpts = append(baseOpts, opts...)

	conn, err := grpc.NewClient(addr, baseOpts...)
	if err != nil {
		return nil, err
	}

	c := NewWithConn(conn)
	c.ownsConn = true
	return c, nil
}

func NewWithConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		conn:   conn,
		client: taskv1.NewTaskServiceClient(conn),
	}
}

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

func (c *Client) Get(ctx context.Context, taskID string) (*schemav1.Task, error) {
	resp, err := c.client.Get(ctx, &taskv1.GetRequest{Id: taskID})
	if err != nil {
		return nil, err
	}
	return resp.GetTask(), nil
}

func (c *Client) Cancel(ctx context.Context, taskID string) error {
	_, err := c.client.Cancel(ctx, &taskv1.CancelRequest{Id: taskID})
	return err
}

func (c *Client) Close() error {
	if c.ownsConn {
		if cc, ok := c.conn.(*grpc.ClientConn); ok {
			return cc.Close()
		}
	}
	return nil
}
