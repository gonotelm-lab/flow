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

type ScheduleDeadLetterStoreImpl struct {
	db *gorm.DB
}

func NewScheduleDeadLetterStoreImpl(db *gorm.DB) store.ScheduleDeadLetter {
	return &ScheduleDeadLetterStoreImpl{db: db}
}

func (s *ScheduleDeadLetterStoreImpl) Create(
	ctx context.Context,
	dl *schema.ScheduleDeadLetter,
) (*schema.ScheduleDeadLetter, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(dl).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return dl, nil
}

func (s *ScheduleDeadLetterStoreImpl) Get(
	ctx context.Context,
	id uuid.UUID,
) (*schema.ScheduleDeadLetter, error) {
	db := util.GetDB(ctx, s.db)
	var dl schema.ScheduleDeadLetter
	if err := db.Where("id = ?", id).First(&dl).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &dl, nil
}

func (s *ScheduleDeadLetterStoreImpl) GetRetryingBySchedule(
	ctx context.Context,
	scheduleId uuid.UUID,
) (*schema.ScheduleDeadLetter, error) {
	db := util.GetDB(ctx, s.db)
	var dl schema.ScheduleDeadLetter
	if err := db.Where("schedule_id = ? AND state = ?", scheduleId, schema.DeadLetterStateRetrying).
		First(&dl).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &dl, nil
}

func (s *ScheduleDeadLetterStoreImpl) List(
	ctx context.Context,
	params *store.DeadLetterListParams,
) ([]*schema.ScheduleDeadLetter, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Model(&schema.ScheduleDeadLetter{})

	if params.Namespace != "" {
		q = q.Where("namespace = ?", params.Namespace)
	}
	if params.ScheduleId != "" {
		q = q.Where("schedule_id::text = ?", params.ScheduleId)
	}
	if params.State != "" {
		q = q.Where("state = ?", params.State)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var list []*schema.ScheduleDeadLetter
	if err := q.Offset(params.Offset).Limit(params.Limit).
		Order("create_time DESC").
		Find(&list).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	return list, total, nil
}

func (s *ScheduleDeadLetterStoreImpl) UpdateRetry(
	ctx context.Context,
	id uuid.UUID,
	p *store.DeadLetterRetryParams,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.ScheduleDeadLetter{}).
		Where("id = ? AND state = ?", id, schema.DeadLetterStateRetrying).
		Updates(map[string]any{
			"attempt_count":    p.AttemptCount,
			"next_attempt_at":  p.NextAttemptAt,
			"last_http_status": p.LastHttpStatus,
			"last_error":       truncate(p.LastError, 1024),
			"attempts":         p.Attempts,
			"update_time":      p.UpdateTime,
		})
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (s *ScheduleDeadLetterStoreImpl) SetState(
	ctx context.Context,
	id uuid.UUID,
	from, to string,
	now int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.ScheduleDeadLetter{}).
		Where("id = ? AND state = ?", id, from).
		Updates(map[string]any{
			"state":       to,
			"update_time": now,
		})
	if res.Error != nil {
		return false, sql.WrapError(res.Error)
	}
	return res.RowsAffected > 0, nil
}
