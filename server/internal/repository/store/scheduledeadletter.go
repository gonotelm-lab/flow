package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
)

type DeadLetterListParams struct {
	Namespace  string
	ScheduleId string
	State      string
	Offset     int
	Limit      int
}

// DeadLetterRetryParams 是一次死信慢重试的落地内容。
type DeadLetterRetryParams struct {
	AttemptCount   int
	NextAttemptAt  int64
	LastHttpStatus int
	LastError      string
	Attempts       []byte
	UpdateTime     int64
}

type ScheduleDeadLetter interface {
	Create(ctx context.Context, dl *schema.ScheduleDeadLetter) (*schema.ScheduleDeadLetter, error)
	Get(ctx context.Context, id uuid.UUID) (*schema.ScheduleDeadLetter, error)
	List(ctx context.Context, params *DeadLetterListParams) ([]*schema.ScheduleDeadLetter, int64, error)

	// GetRetryingBySchedule 取该 schedule 当前处于 RETRYING 的死信（最多一条，
	// 由部分唯一索引 uk_schedule_dead_letters_retrying 保证）。
	GetRetryingBySchedule(ctx context.Context, scheduleId uuid.UUID) (*schema.ScheduleDeadLetter, error)

	UpdateRetry(ctx context.Context, id uuid.UUID, p *DeadLetterRetryParams) (bool, error)

	// SetState 做条件状态切换（from -> to），返回是否生效。
	SetState(ctx context.Context, id uuid.UUID, from, to string, now int64) (bool, error)

	// DeleteBySchedule 供 schedule 硬删时级联清理。
	DeleteBySchedule(ctx context.Context, scheduleId uuid.UUID) error
}
