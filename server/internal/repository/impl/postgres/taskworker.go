package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"gorm.io/gorm"
)

type TaskWorkerStoreImpl struct {
	db *gorm.DB
}

func NewTaskWorkerStoreImpl(db *gorm.DB) store.TaskWorker {
	return &TaskWorkerStoreImpl{db: db}
}

func (s *TaskWorkerStoreImpl) Create(
	ctx context.Context,
	worker *schema.TaskWorker,
) (*schema.TaskWorker, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(worker).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return worker, nil
}

func (s *TaskWorkerStoreImpl) Get(
	ctx context.Context,
	id int64,
) (*schema.TaskWorker, error) {
	db := util.GetDB(ctx, s.db)
	var worker schema.TaskWorker
	if err := db.Where("id = ?", id).First(&worker).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &worker, nil
}

func (s *TaskWorkerStoreImpl) UpdateHeartbeat(
	ctx context.Context,
	id int64,
	heartbeatTime int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.TaskWorker{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"heartbeat_time": heartbeatTime,
		})
	if err := res.Error; err != nil {
		return false, sql.WrapError(err)
	}
	return res.RowsAffected > 0, nil
}

func (s *TaskWorkerStoreImpl) Delete(
	ctx context.Context,
	id int64,
) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Delete(&schema.TaskWorker{}, id).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}
