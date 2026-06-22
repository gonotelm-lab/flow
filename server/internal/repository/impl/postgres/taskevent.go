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
	limit int,
) ([]*schema.TaskEvent, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Where("task_id = ?", taskID).
		Order("id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}

	var events []*schema.TaskEvent
	if err := q.Find(&events).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return events, nil
}
