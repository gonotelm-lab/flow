package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
)

type ScheduleListParams struct {
	Namespace    string
	ScheduleType string
	State        string
	Id           string
	Offset       int
	Limit        int
}

// ScheduleClaimParams 是批量认领的过滤条件与租约信息。
//
// ShardStart/ShardEnd 是本实例当前负责的分片闭区间。分片只用于切分扫描范围以提效；
// 正确性由 SELECT ... FOR UPDATE SKIP LOCKED 加 lease 保证，与分片归属无关——扩缩容
// 期间两个实例同时认为自己拥有同一分片也不会重复投递。
type ScheduleClaimParams struct {
	ShardStart int
	ShardEnd   int
	Now        int64
	BatchSize  int
	LeaseOwner string
	LeaseUntil int64
}

type ClaimByIDParams struct {
	FireId            uuid.UUID
	FireState         string
	FireScheduledTime int64
	LeaseOwner        string
	LeaseUntil        int64
	Now               int64
}

// AttemptOutcome 描述一次投递尝试结束后对 schedules 行的全部变更。
//
// NextFireState 为 nil 表示释放 fire 槽位（写 NULL，该 schedule 恢复可触发）；
// 非 nil 表示继续占用。
type AttemptOutcome struct {
	NextFireState *string
	NextAttemptAt int64
	AttemptNo     int
	// 0 表示不修改 next_fire_at
	NextFireAt int64
	// "" 表示不修改 schedule 生命周期状态
	NewState  string
	LastError string
	// true 表示这次尝试成功：重置 consecutive_failures
	Success bool
	// AddSkipped > 0 时把 skipped_count 累加这么多。
	// 用于「本次投递耗时过长导致落后」，此时被跳过的 occurrence 数量在此一次性结算。
	AddSkipped int64
	Now        int64
}

type Schedule interface {
	Create(ctx context.Context, s *schema.Schedule) (*schema.Schedule, error)
	Get(ctx context.Context, id uuid.UUID) (*schema.Schedule, error)
	// GetByIdempotencyKey 用于幂等注册：命中则返回已存在的 schedule。
	GetByIdempotencyKey(ctx context.Context, namespace, key string) (*schema.Schedule, error)
	List(ctx context.Context, params *ScheduleListParams) ([]*schema.Schedule, int64, error)

	// Now 返回数据库当前时间（unix 毫秒）。
	//
	// 到期判定与「fire_at 必须晚于当前时间」的校验都必须用它，而不是各 pod 自己的
	// time.Now()：多个 pod 时钟快慢不一，快几百毫秒的那个会提前触发。一次性任务是
	// 秒级精度，所以这里必须单一时钟源。
	Now(ctx context.Context) (int64, error)

	// ClaimDue 认领本分片范围内到期的 occurrence，把 fire 槽位置为 PENDING。
	// 不修改 next_fire_at——下一次触发时刻由投递结束时的 ApplyOutcome 落地。
	ClaimDue(ctx context.Context, p *ScheduleClaimParams) ([]*schema.Schedule, error)

	// ClaimRetry 认领需要继续投递的 fire：快重试、死信慢重试，以及租约过期后的接管。
	ClaimRetry(ctx context.Context, p *ScheduleClaimParams) ([]*schema.Schedule, error)

	// ClaimByID 为 TriggerNow 与死信 Replay 占用槽位（要求当前空闲）。
	ClaimByID(ctx context.Context, id uuid.UUID, p *ClaimByIDParams) (bool, error)

	// SkipDue 丢弃一个迟到的 occurrence：释放槽位、推进 next_fire_at、累加 skipped_count。
	SkipDue(ctx context.Context, id, fireId uuid.UUID, nextFireAt, skipped, now int64) (bool, error)

	// ApplyOutcome 落地一次投递尝试的结果（成功 / 继续重试 / 进入死信 / 死信耗尽）。
	ApplyOutcome(ctx context.Context, id, fireId uuid.UUID, p *AttemptOutcome) (bool, error)

	// UpdateDefinition 修改定义字段（name / callback_url / content_type / payload /
	// trigger / next_fire_at）。运行态字段由调度器维护，不在此处。
	UpdateDefinition(ctx context.Context, s *schema.Schedule) (bool, error)

	// SetState 做 ACTIVE <-> PAUSED 的条件切换；nextFireAt > 0 时同时推进
	// next_fire_at（Resume 需要按当前时间重算，不能沿用暂停前的时刻）。
	SetState(ctx context.Context, id uuid.UUID, from, to string, nextFireAt, now int64) (bool, error)

	// DeleteCascade 硬删 schedule，并在同一个事务里级联删除它的失败尝试记录与死信。
	//
	// 级联放在仓储层用事务完成，是因为服务层拿到的只有 Store（没有 TxManager），
	// 且这三张表的清理必须原子——否则会留下指向已删除 schedule 的孤儿死信。
	DeleteCascade(ctx context.Context, id uuid.UUID) error
}
