package schema

import "github.com/google/uuid"

// 触发类型。
const (
	ScheduleTypeOneShot = "ONE_SHOT"
	ScheduleTypeCron    = "CRON"
)

// schedule 生命周期状态。ACTIVE 之外都是终态：只读、可删，不会再触发。
const (
	ScheduleStateActive   = "ACTIVE"
	ScheduleStateStopped  = "STOPPED"
	ScheduleStateFinished = "FINISHED"
	ScheduleStateFailed   = "FAILED"
)

// fire 槽位状态。空（nil）表示槽位空闲。
const (
	FireStatePending    = "PENDING"
	FireStateDelivering = "DELIVERING"
	FireStateDeadLetter = "DEAD_LETTER"
)

// 死信状态。RETRYING 阻塞 schedule 的新触发；EXHAUSTED 不阻塞，等人工重投；
// REPLAYED 是人工重投过；STOPPED 是所属 schedule 被停掉时写入的终态。
const (
	DeadLetterStateRetrying  = "RETRYING"
	DeadLetterStateExhausted = "EXHAUSTED"
	DeadLetterStateReplayed  = "REPLAYED"
	DeadLetterStateStopped   = "STOPPED"
)

// 投递尝试阶段。
const (
	AttemptPhaseFast       = "FAST"
	AttemptPhaseDeadLetter = "DEAD_LETTER"
)

// Schedule 既是定时任务定义，也是它唯一那个 fire 槽位的状态载体。
//
// 「每条 schedule 同时最多一个未完成 fire」是调度语义的基石，所以 fire 状态直接压在
// 这一行上而不是单独建表：fire_state 为空即空闲，非空即被阻塞。
//
// FireId / FireState 用指针类型是必须的：部分索引依赖 `fire_state IS NULL` 判定空闲，
// 若用空字符串代替 NULL，索引谓词会失效，schedule 会永久卡在「看起来还有 fire 在飞」。
type Schedule struct {
	Id         uuid.UUID `gorm:"column:id;primaryKey"`
	Namespace  string    `gorm:"column:namespace"`
	Name       string    `gorm:"column:name"`
	CreateTime int64     `gorm:"column:create_time"`
	UpdateTime int64     `gorm:"column:update_time"`

	// 触发方式：ScheduleTypeOneShot 或 ScheduleTypeCron
	ScheduleType string `gorm:"column:schedule_type"`
	// cron 专用，标准 5 字段
	CronExpr string `gorm:"column:cron_expr"`
	// cron 专用，IANA 时区名
	Timezone string `gorm:"column:timezone"`
	// 一次性专用，unix 毫秒
	FireAt int64 `gorm:"column:fire_at"`

	State       string `gorm:"column:state"`
	CallbackUrl string `gorm:"column:callback_url"`
	ContentType string `gorm:"column:content_type"`
	Payload     []byte `gorm:"column:payload"`

	// 可选幂等键。必须是 **nil 而不是空字符串**：部分唯一索引是
	// `(namespace, idempotency_key) WHERE idempotency_key IS NOT NULL`，若用空字符串
	// 表示「没传键」，所有不传键的 schedule 会被当成同一个键，第二条就撞唯一约束。
	IdempotencyKey *string `gorm:"column:idempotency_key"`

	// 静态分片号 0..1023，注册时算一次、永不重算
	Shard int16 `gorm:"column:shard"`
	// 下次计划触发时刻，unix 毫秒
	NextFireAt int64 `gorm:"column:next_fire_at"`

	// ---- fire 槽位 ----
	FireId            *uuid.UUID `gorm:"column:fire_id"`
	FireScheduledTime int64      `gorm:"column:fire_scheduled_time"`
	FireState         *string    `gorm:"column:fire_state"`
	AttemptNo         int        `gorm:"column:attempt_no"`
	NextAttemptAt     int64      `gorm:"column:next_attempt_at"`
	LeaseOwner        string     `gorm:"column:lease_owner"`
	LeaseUntil        int64      `gorm:"column:lease_until"`

	// ---- 运行统计 ----
	ConsecutiveFailures int    `gorm:"column:consecutive_failures"`
	TotalFailures       int64  `gorm:"column:total_failures"`
	SkippedCount        int64  `gorm:"column:skipped_count"`
	LastError           string `gorm:"column:last_error"`
	LastSuccessAt       int64  `gorm:"column:last_success_at"`
}

func (Schedule) TableName() string {
	return "schedules"
}

// Busy 表示该 schedule 当前有未完成的 fire，期间到点的 occurrence 会被跳过。
func (s *Schedule) Busy() bool {
	return s.FireState != nil
}
