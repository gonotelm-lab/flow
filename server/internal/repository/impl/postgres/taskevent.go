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

type TaskEventStoreImpl struct {
	db *gorm.DB
}

func NewTaskEventStoreImpl(db *gorm.DB) store.TaskEvent {
	return &TaskEventStoreImpl{db: db}
}

func (s *TaskEventStoreImpl) Append(
	ctx context.Context,
	event *schema.TaskEvent,
) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(event).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}

func (s *TaskEventStoreImpl) ListByTaskID(
	ctx context.Context,
	taskID uuid.UUID,
	offset, limit int,
) ([]*schema.TaskEvent, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Where("task_id = ?", taskID)

	var total int64
	if err := q.Model(&schema.TaskEvent{}).Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var events []*schema.TaskEvent
	if err := q.Offset(offset).Limit(limit).
		Order("create_time ASC").
		Find(&events).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}
	return events, total, nil
}
