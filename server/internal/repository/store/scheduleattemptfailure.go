package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
)

// ScheduleAttemptFailure 只记录失败的投递尝试；成功不落记录。
type ScheduleAttemptFailure interface {
	Create(ctx context.Context, f *schema.ScheduleAttemptFailure) error
	ListBySchedule(ctx context.Context, scheduleId uuid.UUID, limit int) ([]*schema.ScheduleAttemptFailure, error)
}
