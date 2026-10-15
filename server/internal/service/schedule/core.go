// Package schedule 承载定时回调的业务逻辑。
//
// 客户端面（api.schedule.v1.ScheduleService）与运维面（AdminService 上的 schedule
// 系列 RPC）共用这里的 Core；两者唯一的区别是运维面会额外写审计日志，因此审计不放在
// Core 里。
package schedule

import (
	"context"
	"net/url"
	"strings"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/internal/scheduler"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type ListParams struct {
	Namespace    string
	ScheduleType string
	State        string
	Id           string
	Page         int
	PageSize     int
}

type DeadLetterListParams struct {
	Namespace  string
	ScheduleId string
	State      string
	Page       int
	PageSize   int
}

type Core struct {
	store *repository.Store
}

func NewCore(store *repository.Store) *Core {
	return &Core{store: store}
}

func (c *Core) now(ctx context.Context) (int64, error) {
	now, err := c.store.Schedule.Now(ctx)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to get database time")
	}
	return now, nil
}

func (c *Core) mustGet(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	sched, err := c.store.Schedule.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.ScheduleNotFound
		}
		return nil, errors.WithMessage(err, "failed to get schedule")
	}
	return sched, nil
}

// ---------------------------------------------------------------------------
// 注册与查询
// ---------------------------------------------------------------------------

// Create 注册一条 schedule。
//
// 幂等：带 idempotency_key 时，命中已有记录直接返回（不报错、不新建）。并发场景下
// 两个请求可能同时查不到而都去插入，此时靠部分唯一索引 uk_schedules_idempotency
// 拦下后一个，再回头取已有记录返回——所以重复注册对调用方表现为幂等。
func (c *Core) Create(ctx context.Context, in *schema.Schedule) (*schema.Schedule, error) {
	if err := c.checkNamespace(ctx, in.Namespace); err != nil {
		return nil, err
	}

	key := IdempotencyKeyOf(in)
	if existing, err := c.lookupIdempotent(ctx, in.Namespace, key); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	now, err := c.now(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate schedule id")
	}

	in.Id = id
	// 静态分片号：注册时算一次、写死，之后永不重算，因此扩缩容不重写数据行。
	in.Shard = scheduler.ShardOf(id)
	in.State = schema.ScheduleStateActive
	in.CreateTime = now
	in.UpdateTime = now

	if err := c.applyTrigger(in, now); err != nil {
		return nil, err
	}

	created, err := c.store.Schedule.Create(ctx, in)
	if err != nil {
		if errors.Is(err, pkgerr.DuplicatedResource) && key != "" {
			existing, lookupErr := c.store.Schedule.GetByIdempotencyKey(ctx, in.Namespace, key)
			if lookupErr == nil {
				return existing, nil
			}
		}
		return nil, errors.WithMessage(err, "failed to create schedule")
	}

	return created, nil
}

func (c *Core) Get(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	return c.mustGet(ctx, id)
}

func (c *Core) List(ctx context.Context, p ListParams) ([]*schema.Schedule, int64, error) {
	offset, limit := paging(p.Page, p.PageSize)

	list, total, err := c.store.Schedule.List(ctx, &store.ScheduleListParams{
		Namespace:    p.Namespace,
		ScheduleType: p.ScheduleType,
		State:        p.State,
		Id:           p.Id,
		Offset:       offset,
		Limit:        limit,
	})
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed to list schedules")
	}
	return list, total, nil
}

// Update 修改定义字段。运行态（next_fire_at / fire 槽位 / 统计）由调度器维护，
// 这里不接受，也不受影响。
func (c *Core) Update(ctx context.Context, id uuid.UUID, patch *schema.Schedule) (*schema.Schedule, error) {
	cur, err := c.mustGet(ctx, id)
	if err != nil {
		return nil, err
	}

	now, err := c.now(ctx)
	if err != nil {
		return nil, err
	}

	cur.Name = patch.Name
	cur.CallbackUrl = patch.CallbackUrl
	cur.ContentType = patch.ContentType
	cur.Payload = patch.Payload
	cur.ScheduleType = patch.ScheduleType
	cur.CronExpr = patch.CronExpr
	cur.Timezone = patch.Timezone
	cur.FireAt = patch.FireAt
	cur.UpdateTime = now

	// 用当前时间重算 next_fire_at：改 cron 表达式或时区后，旧的计划时刻已经没有意义。
	if err := c.applyTrigger(cur, now); err != nil {
		return nil, err
	}

	ok, err := c.store.Schedule.UpdateDefinition(ctx, cur)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to update schedule")
	}
	if !ok {
		return nil, srverr.ScheduleNotFound
	}

	return cur, nil
}

// Pause 只停未来的 occurrence。幂等：已经是 PAUSED 直接返回。
func (c *Core) Pause(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	cur, err := c.mustGet(ctx, id)
	if err != nil {
		return nil, err
	}

	switch cur.State {
	case schema.ScheduleStatePaused:
		return cur, nil
	case schema.ScheduleStateActive:
		now, err := c.now(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := c.store.Schedule.SetState(
			ctx, id, schema.ScheduleStateActive, schema.ScheduleStatePaused, 0, now,
		); err != nil {
			return nil, errors.WithMessage(err, "failed to pause schedule")
		}
		return c.mustGet(ctx, id)
	default:
		return nil, srverr.ScheduleEnded.WithDetail("schedule state: " + cur.State)
	}
}

// Resume 恢复触发。恢复后按当前时间重算 next_fire_at，不补发暂停期间错过的 occurrence。
func (c *Core) Resume(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	cur, err := c.mustGet(ctx, id)
	if err != nil {
		return nil, err
	}

	switch cur.State {
	case schema.ScheduleStateActive:
		return cur, nil
	case schema.ScheduleStatePaused:
		now, err := c.now(ctx)
		if err != nil {
			return nil, err
		}

		next, err := c.nextFireAt(cur, now)
		if err != nil {
			return nil, err
		}

		if _, err := c.store.Schedule.SetState(
			ctx, id, schema.ScheduleStatePaused, schema.ScheduleStateActive, next, now,
		); err != nil {
			return nil, errors.WithMessage(err, "failed to resume schedule")
		}
		return c.mustGet(ctx, id)
	default:
		return nil, srverr.ScheduleEnded.WithDetail("schedule state: " + cur.State)
	}
}

// Delete 硬删 schedule，并级联删除它的失败记录与死信。
//
// 永远成功、不做前置检查：已写出的 HTTP 无法撤回，剩余的快速重试与死信重试随行一起
// 消失。调用方（运维面）负责在删除前取好快照写进审计——否则事后无从知道删了什么。
func (c *Core) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := c.mustGet(ctx, id); err != nil {
		return err
	}
	if err := c.store.Schedule.DeleteCascade(ctx, id); err != nil {
		return errors.WithMessage(err, "failed to delete schedule")
	}
	return nil
}

// ---------------------------------------------------------------------------
// 手动触发与死信
// ---------------------------------------------------------------------------

// TriggerNow 立即投递一次。
//
// - 生成独立的 fire id，不影响 cron 的自然节奏；
// - fire_scheduled_time 置 0 作为「手动触发」标记，投递结束后调度器不会推进 next_fire_at；
// - PAUSED 状态下依然可用（Temporal 语义：pause 只作用于未来的 action）。
func (c *Core) TriggerNow(ctx context.Context, id uuid.UUID) (*schema.Schedule, error) {
	cur, err := c.mustGet(ctx, id)
	if err != nil {
		return nil, err
	}

	if cur.State == schema.ScheduleStateFinished || cur.State == schema.ScheduleStateFailed {
		return nil, srverr.ScheduleEnded.WithDetail("schedule state: " + cur.State)
	}
	if cur.Busy() {
		return nil, srverr.ScheduleBusy.WithDetail("a fire is already in flight")
	}

	now, err := c.now(ctx)
	if err != nil {
		return nil, err
	}

	fireId, err := uuid.NewV7()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate fire id")
	}

	ok, err := c.store.Schedule.ClaimByID(ctx, id, &store.ClaimByIDParams{
		FireId:            fireId,
		FireState:         schema.FireStatePending,
		FireScheduledTime: 0,
		Now:               now,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to trigger schedule")
	}
	if !ok {
		return nil, srverr.ScheduleBusy.WithDetail("a fire is already in flight")
	}

	return c.mustGet(ctx, id)
}

func (c *Core) ListDeadLetters(
	ctx context.Context,
	p DeadLetterListParams,
) ([]*schema.ScheduleDeadLetter, int64, error) {
	offset, limit := paging(p.Page, p.PageSize)

	list, total, err := c.store.ScheduleDeadLetter.List(ctx, &store.DeadLetterListParams{
		Namespace:  p.Namespace,
		ScheduleId: p.ScheduleId,
		State:      p.State,
		Offset:     offset,
		Limit:      limit,
	})
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed to list dead letters")
	}
	return list, total, nil
}

func (c *Core) GetDeadLetter(ctx context.Context, id uuid.UUID) (*schema.ScheduleDeadLetter, error) {
	dl, err := c.store.ScheduleDeadLetter.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.ScheduleDeadLetterNotFound
		}
		return nil, errors.WithMessage(err, "failed to get dead letter")
	}
	return dl, nil
}

// ReplayDeadLetter 人工重投一次。
//
// 关键：**沿用原 fire_id**。接收方靠 Fire-Id 做幂等去重，如果重投生成新的 fire_id，
// 去重契约就断了，对方会把补投当成一次全新事件。
func (c *Core) ReplayDeadLetter(ctx context.Context, dlId uuid.UUID) error {
	dl, err := c.GetDeadLetter(ctx, dlId)
	if err != nil {
		return err
	}
	if dl.State != schema.DeadLetterStateExhausted {
		return srverr.ScheduleDeadLetterNotReplayable.WithDetail("dead letter state: " + dl.State)
	}

	cur, err := c.mustGet(ctx, dl.ScheduleId)
	if err != nil {
		return err
	}
	if cur.Busy() {
		return srverr.ScheduleBusy.WithDetail("a fire is already in flight")
	}

	now, err := c.now(ctx)
	if err != nil {
		return err
	}

	ok, err := c.store.Schedule.ClaimByID(ctx, dl.ScheduleId, &store.ClaimByIDParams{
		FireId:            dl.FireId,
		FireState:         schema.FireStatePending,
		FireScheduledTime: dl.ScheduledTime,
		Now:               now,
	})
	if err != nil {
		return errors.WithMessage(err, "failed to claim schedule for replay")
	}
	if !ok {
		return srverr.ScheduleBusy.WithDetail("a fire is already in flight")
	}

	if _, err := c.store.ScheduleDeadLetter.SetState(
		ctx, dlId, schema.DeadLetterStateExhausted, schema.DeadLetterStateReplayed, now,
	); err != nil {
		return errors.WithMessage(err, "failed to mark dead letter replayed")
	}

	return nil
}

func (c *Core) DiscardDeadLetter(ctx context.Context, dlId uuid.UUID) error {
	dl, err := c.GetDeadLetter(ctx, dlId)
	if err != nil {
		return err
	}
	if dl.State != schema.DeadLetterStateExhausted {
		return srverr.ScheduleDeadLetterNotReplayable.WithDetail("dead letter state: " + dl.State)
	}

	now, err := c.now(ctx)
	if err != nil {
		return err
	}

	if _, err := c.store.ScheduleDeadLetter.SetState(
		ctx, dlId, schema.DeadLetterStateExhausted, schema.DeadLetterStateDiscarded, now,
	); err != nil {
		return errors.WithMessage(err, "failed to discard dead letter")
	}

	return nil
}

// ---------------------------------------------------------------------------
// 校验与归一化
// ---------------------------------------------------------------------------

func (c *Core) checkNamespace(ctx context.Context, namespace string) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return pkgerr.InvalidArgument.WithDetail("namespace is empty")
	}

	if _, err := c.store.Namespace.Get(ctx, namespace); err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return srverr.NamespaceNotFound
		}
		return errors.WithMessage(err, "failed to get namespace")
	}
	return nil
}

func (c *Core) lookupIdempotent(
	ctx context.Context,
	namespace, key string,
) (*schema.Schedule, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}

	existing, err := c.store.Schedule.GetByIdempotencyKey(ctx, strings.TrimSpace(namespace), key)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "failed to look up idempotency key")
	}
	return existing, nil
}

// applyTrigger 校验并归一化定义字段，同时算出 next_fire_at。
//
// 一次性与 cron 互斥：一次性要求 fire_at 严格晚于当前时间（用数据库时间判定，避免
// 各 pod 时钟偏移）；cron 要求 5 字段表达式能解析。
func (c *Core) applyTrigger(in *schema.Schedule, now int64) error {
	in.Namespace = strings.TrimSpace(in.Namespace)
	if in.Namespace == "" {
		return pkgerr.InvalidArgument.WithDetail("namespace is empty")
	}

	in.Name = strings.TrimSpace(in.Name)
	in.CallbackUrl = strings.TrimSpace(in.CallbackUrl)
	if err := ValidateCallbackUrl(in.CallbackUrl); err != nil {
		return err
	}

	if strings.TrimSpace(in.ContentType) == "" {
		in.ContentType = scheduler.DefaultContentType
	}
	// nil 而不是空字符串：部分唯一索引靠 IS NOT NULL 区分「传了键」与「没传键」。
	in.IdempotencyKey = normalizeIdempotencyKey(in.IdempotencyKey)

	next, err := c.nextFireAt(in, now)
	if err != nil {
		return err
	}
	in.NextFireAt = next

	return nil
}

// nextFireAt 按触发类型算出下次触发时刻；同时完成该类型的字段归一化与校验。
func (c *Core) nextFireAt(in *schema.Schedule, now int64) (int64, error) {
	switch in.ScheduleType {
	case schema.ScheduleTypeOneShot:
		// cron 专用字段必须清空，否则库里会留下「两者都有」的歧义数据。
		in.CronExpr = ""
		in.Timezone = scheduler.DefaultTimezone

		if in.FireAt <= now {
			return 0, pkgerr.InvalidArgument.WithDetail(
				"fire_at must be later than current time")
		}
		return in.FireAt, nil

	case schema.ScheduleTypeCron:
		in.FireAt = 0

		tz := strings.TrimSpace(in.Timezone)
		if tz == "" {
			tz = scheduler.DefaultTimezone
		}
		in.Timezone = tz

		if strings.TrimSpace(in.CronExpr) == "" {
			return 0, pkgerr.InvalidArgument.WithDetail("cron expression is empty")
		}

		next, err := scheduler.NextMillis(in.CronExpr, tz, now)
		if err != nil {
			return 0, pkgerr.InvalidArgument.WithDetail(err.Error())
		}
		return next, nil

	default:
		return 0, pkgerr.InvalidArgument.WithDetail(
			"schedule_type must be ONE_SHOT or CRON")
	}
}

// ValidateCallbackUrl 只做格式校验。
//
// 明确不做的事（见设计文档「已接受的残余风险」）：不探测可达性、不拦截私网地址与云
// metadata 地址。这是有意为之的选择，代价是「谁能注册谁就能让 flow 打内网任意地址」，
// 因此部署上必须保证 7090 / 7091 不暴露到不可信网络。
func ValidateCallbackUrl(raw string) error {
	if raw == "" {
		return pkgerr.InvalidArgument.WithDetail("callback_url is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return pkgerr.InvalidArgument.WithDetail("callback_url is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return pkgerr.InvalidArgument.WithDetail("callback_url scheme must be http or https")
	}
	if u.Host == "" {
		return pkgerr.InvalidArgument.WithDetail("callback_url host is empty")
	}
	if u.User != nil {
		return pkgerr.InvalidArgument.WithDetail("callback_url must not contain credentials")
	}

	return nil
}

func paging(page, pageSize int) (offset, limit int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return (page - 1) * pageSize, pageSize
}

// IdempotencyKeyOf 安全地取幂等键（nil 表示未提供）。
func IdempotencyKeyOf(s *schema.Schedule) string {
	if s == nil || s.IdempotencyKey == nil {
		return ""
	}
	return *s.IdempotencyKey
}

// normalizeIdempotencyKey 把「未提供」统一成 nil。
//
// 这一点很关键：如果空键以空字符串入库，部分唯一索引
// `(namespace, idempotency_key) WHERE idempotency_key IS NOT NULL` 会把所有
// 不带键的 schedule 视作同一个键，第二条注册就会撞唯一约束。
func normalizeIdempotencyKey(p *string) *string {
	if p == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*p)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
