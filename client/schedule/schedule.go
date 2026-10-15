// Package schedule 是 Flow 定时回调（Schedule）的客户端 SDK。
//
// 客户端可以注册两类定时任务：一次性（绝对时刻）与 cron（标准 5 字段重复触发）。
// 到期后由 flowserver 主动 POST 到注册时给定的 callback_url。
//
// 回调契约（接收方必须了解的三件事）：
//
//  1. body 就是注册时传入的 payload 原样字节，不包 envelope；
//     Content-Type 取注册时指定的值。
//  2. 元数据在 header 里：
//     X-Flow-Schedule-Id / X-Flow-Fire-Id / X-Flow-Scheduled-Time /
//     X-Flow-Timestamp / X-Flow-Signature。
//  3. 投递是 **at-least-once**：同一次 fire 失败会重试，实例崩溃后也会被其他实例接管重投。
//     X-Flow-Fire-Id 在同一次 fire 的所有重试与接管之间**保持不变**，接收方必须用它做
//     幂等去重——这是唯一的去重依据。
//
// 签名：HMAC-SHA256，密钥是 namespace 的 api_key，签名串为 `timestamp + "." + rawBody`，
// 结果放在 X-Flow-Signature（形如 sha256=<hex>）。接收方可以按同样规则复算校验。
package schedule

import (
	"context"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn     grpc.ClientConnInterface
	client   schedulev1.ScheduleServiceClient
	ownsConn bool
}

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

func NewWithConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		conn:   conn,
		client: schedulev1.NewScheduleServiceClient(conn),
	}
}

// Register 注册一条定时任务。
//
// 通常用 OneShot / After / Cron 构造参数，而不是手写 proto。
// 幂等：带 WithIdempotencyKey 时，重复注册返回已存在的那条，不会新建。
func (c *Client) Register(
	ctx context.Context,
	schedule *schemav1.Schedule,
	opts ...Option,
) (*schemav1.Schedule, error) {
	newOptions(opts).apply(schedule)

	resp, err := c.client.Register(ctx, &schedulev1.RegisterRequest{Schedule: schedule})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) Get(ctx context.Context, id string) (*schemav1.Schedule, error) {
	return c.client.Get(ctx, &schedulev1.GetRequest{Id: id})
}

// List 按条件分页查询，返回当页数据与总数。
func (c *Client) List(
	ctx context.Context,
	opts ...ListOption,
) ([]*schemav1.Schedule, int64, error) {
	o := newListOptions(opts)

	resp, err := c.client.List(ctx, &schedulev1.ListRequest{
		Page: &schemav1.PageRequest{
			Page:     int32(o.page),
			PageSize: int32(o.pageSize),
		},
		Namespace:    o.namespace,
		ScheduleType: o.scheduleType,
		State:        o.state,
		Id:           o.id,
	})
	if err != nil {
		return nil, 0, err
	}

	return resp.GetSchedules(), resp.GetPage().GetTotalCount(), nil
}

// Update 修改一条 schedule 的定义。
//
// 只接受 name / callback_url / content_type / payload / trigger；运行态字段由服务端维护。
// trigger 必须完整给出（改 cron 就跑一次 Cron(...)，改一次性就跑一次 OneShot(...)）。
func (c *Client) Update(
	ctx context.Context,
	id string,
	schedule *schemav1.Schedule,
	opts ...Option,
) (*schemav1.Schedule, error) {
	newOptions(opts).apply(schedule)

	return c.client.Update(ctx, &schedulev1.UpdateRequest{
		Id:       id,
		Schedule: schedule,
	})
}

// Pause 停止产生新的触发。
//
// 已经发出的 HTTP 与它的快速重试会跑完；死信慢重试在暂停期间仍会继续（Temporal 语义：
// pause 只作用于 schedule 未来的 action）。
func (c *Client) Pause(ctx context.Context, id string) (*schemav1.Schedule, error) {
	return c.client.Pause(ctx, &schedulev1.PauseRequest{Id: id})
}

// Resume 恢复触发，并按当前时间重算下次触发时刻（不补发暂停期间错过的触发）。
func (c *Client) Resume(ctx context.Context, id string) (*schemav1.Schedule, error) {
	return c.client.Resume(ctx, &schedulev1.ResumeRequest{Id: id})
}

// Delete 删除一条 schedule。
//
// 硬删：定义、失败记录、死信一并消失。已发出的 HTTP 无法撤回。
func (c *Client) Delete(ctx context.Context, id string) error {
	_, err := c.client.Delete(ctx, &schedulev1.DeleteRequest{Id: id})
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
