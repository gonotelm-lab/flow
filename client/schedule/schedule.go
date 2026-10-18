// Package schedule 是 Flow 定时回调（Schedule）的客户端 SDK。
//
// 这个包同时服务两端：注册方用它管理定时任务，接收方用它校验回调签名。对外类型都是
// 本包自己的（Schedule / Trigger / State），调用方不需要引入 proto。
//
// 回调契约（接收方必须知道的三件事）：
//
//  1. body 是注册时的 payload 原样字节，不包 envelope；Content-Type 取注册时的值。
//  2. 元数据在 header：X-Flow-Schedule-Id / X-Flow-Fire-Id / X-Flow-Scheduled-Time /
//     X-Flow-Timestamp / X-Flow-Namespace / X-Flow-Signature。
//  3. 投递是 at-least-once：同一次 fire 失败会重试，实例崩溃后也会被别的实例接管重投。
//     Fire-Id 在该次 fire 的所有重试与接管之间不变，接收方靠它做幂等去重。
//
// 签名是 FLOW-HMAC-SHA256，密钥由 namespace 的 api_key 按日期与 namespace 派生，
// 覆盖方法、路径、查询串、固定六个头与 body 哈希；规范见 api/signature 的包注释。
//
// 接收方一个对象就能验签：
//
//	client, _ := schedule.New("", schedule.Config{APIKey: apiKey})
//	if err := client.VerifyRequest(r); err != nil { /* 401 */ }
//
// body 验完仍可读；Fire-Id 去重仍要自己做。可运行示例见 client/example/schedule-receiver。
package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/client/internal/apikey"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config 是客户端自己的配置，和 gRPC 拨号参数分开。
// 一个 Client 对应一个 namespace，和 client/worker、client/task/v2 一样。
type Config struct {
	// Namespace 会走 RPC 的客户端必填（调用时校验）；纯验签的接收方可以留空。
	Namespace string
	// APIKey 是该 namespace 的 api_key，两个方向都用：出站随 RPC 发出
	// （authorization: Bearer <key>，留空就不发），入站给 VerifyRequest 验签。
	APIKey string
}

// validateNamespace 校验 namespace 是否配置。
func (cfg Config) validateNamespace() error {
	if strings.TrimSpace(cfg.Namespace) == "" {
		return ErrNoNamespace
	}

	return nil
}

type Client struct {
	conn     grpc.ClientConnInterface
	client   schedulev1.ScheduleServiceClient
	cfg      Config
	ownsConn bool
}

// New 返回客户端，opts 原样透传给 grpc.NewClient。
//
// 只做验签的接收方 addr 留空就行（gRPC 是懒连接）。这里不校验 Config：
// namespace 是否为空，由每个方法在调用前判断。
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

// NewWithConn 用调用方自己的连接（多客户端共享连接时用）。
func NewWithConn(conn grpc.ClientConnInterface, cfg Config) *Client {
	return &Client{
		conn:   conn,
		client: schedulev1.NewScheduleServiceClient(apikey.WrapConn(conn, cfg.APIKey)),
		cfg:    cfg,
	}
}

// Register 注册一条定时任务，只提交定义（Spec）。
//
// namespace 取 Config.Namespace，定义里不带它。
// 通常用 OneShot / After / Cron 构造 Spec，不必自己拼字段。
//
// 幂等键没显式设置时自动生成一个随机键，所以 RPC 层的重试（同一次调用被投递两次）
// 不会建出两条 schedule。要跨调用去重（网络超时后自己重发、进程重启后重放），
// 就显式给一个稳定的键——那种情况下重复注册返回已存在的那条，不会新建。
func (c *Client) Register(
	ctx context.Context,
	spec Spec,
	opts ...Option,
) (*Schedule, error) {
	if err := c.checkNamespace(); err != nil {
		return nil, err
	}

	newOptions(opts).apply(&spec)

	if err := fillIdempotencyKey(&spec); err != nil {
		return nil, err
	}

	resp, err := c.client.Register(ctx, &schedulev1.RegisterRequest{
		Schedule: specToProto(spec, c.cfg.Namespace),
	})
	if err != nil {
		return nil, err
	}
	return scheduleFromProto(resp), nil
}

// fillIdempotencyKey 给没设幂等键的定义补一个随机键。
func fillIdempotencyKey(spec *Spec) error {
	if spec.IdempotencyKey != "" {
		return nil
	}

	key, err := newIdempotencyKey()
	if err != nil {
		return err
	}
	spec.IdempotencyKey = key
	return nil
}

// newIdempotencyKey 生成一个随机幂等键（16 字节 hex）。
func newIdempotencyKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("schedule: generate idempotency key: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (c *Client) Get(ctx context.Context, id string) (*Schedule, error) {
	if err := c.checkNamespace(); err != nil {
		return nil, err
	}

	resp, err := c.client.Get(ctx, &schedulev1.GetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return scheduleFromProto(resp), nil
}

// List 列出本 namespace 的 schedule，按条件分页。
func (c *Client) List(ctx context.Context, opts ...ListOption) ([]*Schedule, int64, error) {
	if err := c.checkNamespace(); err != nil {
		return nil, 0, err
	}

	o := newListOptions(opts)

	resp, err := c.client.List(ctx, &schedulev1.ListRequest{
		Page: &schemav1.PageRequest{
			Page:     int32(o.page),
			PageSize: int32(o.pageSize),
		},
		Namespace:    c.cfg.Namespace,
		ScheduleType: typeToProto(o.scheduleType),
		State:        stateToProto(o.state),
		Id:           o.id,
	})
	if err != nil {
		return nil, 0, err
	}

	list := make([]*Schedule, 0, len(resp.GetSchedules()))
	for _, item := range resp.GetSchedules() {
		list = append(list, scheduleFromProto(item))
	}
	return list, resp.GetPage().GetTotalCount(), nil
}

// checkNamespace 拦住「没配 namespace 就调接口」：注册会被服务端拒，列表则会变成
// 跨 namespace 查全量。
//
// NewWithConn 没有 error 返回值，这条校验就是那条构造路径的兜底。
func (c *Client) checkNamespace() error {
	if c == nil {
		return ErrNoNamespace
	}

	return c.cfg.validateNamespace()
}

// ErrNoNamespace 客户端没有配置 Config.Namespace，注册与查询都无法进行。
var ErrNoNamespace = errors.New("schedule: client namespace is empty")

// Stop 单向停止一条 schedule：清空触发计划与未完成的投递链路，此后永不触发。
//
// 不可恢复，要再次触发只能重新注册一条。已发出的 HTTP 无法撤回，但它的重试与死信
// 链路到此为止。幂等：已经是 STOPPED 时直接返回当前对象。
func (c *Client) Stop(ctx context.Context, id string) (*Schedule, error) {
	if err := c.checkNamespace(); err != nil {
		return nil, err
	}

	resp, err := c.client.Stop(ctx, &schedulev1.StopRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return scheduleFromProto(resp), nil
}

// Delete 删除一条 schedule。
//
// 硬删：定义、失败记录、死信一并消失。已发出的 HTTP 无法撤回。
func (c *Client) Delete(ctx context.Context, id string) error {
	if err := c.checkNamespace(); err != nil {
		return err
	}

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
