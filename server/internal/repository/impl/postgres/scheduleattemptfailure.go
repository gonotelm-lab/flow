package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ScheduleAttemptFailureStoreImpl struct {
	db *gorm.DB
}

func NewScheduleAttemptFailureStoreImpl(db *gorm.DB) store.ScheduleAttemptFailure {
	return &ScheduleAttemptFailureStoreImpl{db: db}
}

func (s *ScheduleAttemptFailureStoreImpl) Create(
	ctx context.Context,
	f *schema.ScheduleAttemptFailure,
) error {
	db := util.GetDB(ctx, s.db)
	f.Error = truncate(f.Error, 1024)
	if err := db.Create(f).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}

func (s *ScheduleAttemptFailureStoreImpl) ListBySchedule(
	ctx context.Context,
	scheduleId uuid.UUID,
	limit int,
) ([]*schema.ScheduleAttemptFailure, error) {
	db := util.GetDB(ctx, s.db)
	var list []*schema.ScheduleAttemptFailure
	if err := db.Where("schedule_id = ?", scheduleId).
		Order("create_time DESC").
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return list, nil
}

func (s *ScheduleAttemptFailureStoreImpl) DeleteBySchedule(
	ctx context.Context,
	scheduleId uuid.UUID,
) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Delete(&schema.ScheduleAttemptFailure{}, "schedule_id = ?", scheduleId).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}
