package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskStoreImpl struct {
	db *gorm.DB
}

func NewTaskStoreImpl(db *gorm.DB) store.Task {
	return &TaskStoreImpl{db: db}
}

func (s *TaskStoreImpl) Create(ctx context.Context, task *schema.Task) (*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(task).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return task, nil
}

func (s *TaskStoreImpl) Get(ctx context.Context, id uuid.UUID) (*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	var task schema.Task
	if err := db.Where("id = ?", id).First(&task).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return &task, nil
}

func (s *TaskStoreImpl) Claim(
	ctx context.Context,
	namespace, taskType string,
	states []string,
) (*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	// select * from tasks where namespace = ? and task_type = ? and state in ?
	// order by next_run_time asc, id asc
	// for update skip locked limit 1
	var task schema.Task
	err := db.Clauses(clause.Locking{
		Strength: clause.LockingStrengthUpdate,
		Options:  clause.LockingOptionsSkipLocked,
	}).Where("namespace = ? and task_type = ? and state in ?", namespace, taskType, states).
		Order("next_run_time asc, id asc").
		Limit(1).
		Take(&task).Error
	if err != nil {
		return nil, sql.WrapError(err)
	}

	return &task, nil
}

func (s *TaskStoreImpl) ClaimUpdate(
	ctx context.Context,
	id uuid.UUID,
	oldState string,
	params *store.TaskClaimUpdateParams,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	// update task set state = newState, update_time = updateTime, worker_id = workerId
	// where id = id and state = oldState
	result := db.Model(&schema.Task{}).
		Where("id = ? and state = ?", id, oldState).
		Updates(map[string]any{
			"state":       params.NewState,
			"update_time": params.UpdateTime,
			"worker_id":   params.WorkerId,
		})
	if result.Error != nil {
		return false, sql.WrapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

func (s *TaskStoreImpl) Update(ctx context.Context, task *schema.Task) (bool, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Model(&schema.Task{}).
		Where("id = ?", task.Id).
		Updates(task).Error; err != nil {
		return false, sql.WrapError(err)
	}

	return db.RowsAffected > 0, nil
}

func (s *TaskStoreImpl) UpdateOutcome(ctx context.Context,
	id uuid.UUID,
	success bool,
	workerId int64,
	oldState, newState string,
	params *store.TaskUpdateOutcomeParams,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	updates := make(map[string]any)
	if success {
		updates["result"] = params.Payload
	} else {
		updates["error"] = params.Payload
	}
	updates["state"] = newState
	updates["update_time"] = params.UpdateTime

	result := db.Model(&schema.Task{}).
		Updates(updates).
		Where("id = ? and state = ? and worker_id = ?", id, oldState, workerId)
	if result.Error != nil {
		return false, sql.WrapError(result.Error)
	}

	return result.RowsAffected > 0, nil
}
