package postgres

import (
	"context"
	"time"

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
	// update task set state = newState, update_time = updateTime, worker_id = workerId,
	// last_heartbeat_time = updateTime
	// where id = id and state = oldState
	result := db.Model(&schema.Task{}).
		Where("id = ? and state = ?", id, oldState).
		Updates(map[string]any{
			"state":               params.NewState,
			"update_time":         params.UpdateTime,
			"worker_id":           params.WorkerId,
			"last_heartbeat_time": params.UpdateTime,
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
	result := db.Model(&schema.Task{}).
		Where("id = ?", task.Id).
		Updates(task)
	if result.Error != nil {
		return false, sql.WrapError(result.Error)
	}

	return result.RowsAffected > 0, nil
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
		Where("id = ? and state = ? and worker_id = ?", id, oldState, workerId).
		Updates(updates)
	if result.Error != nil {
		return false, sql.WrapError(result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (s *TaskStoreImpl) UpdateHeartbeat(
	ctx context.Context,
	ids []uuid.UUID,
	workerId int64,
	heartTime int64,
) error {
	db := util.GetDB(ctx, s.db)
	result := db.Model(&schema.Task{}).
		Where("id IN ? AND worker_id = ? AND state = ?", ids, workerId, "RUNNING").
		Update("last_heartbeat_time", heartTime)
	if result.Error != nil {
		return sql.WrapError(result.Error)
	}
	return nil
}

func (s *TaskStoreImpl) GetCancelledTasks(
	ctx context.Context,
	workerId int64,
) ([]uuid.UUID, error) {
	db := util.GetDB(ctx, s.db)
	var ids []uuid.UUID
	if err := db.Model(&schema.Task{}).
		Where("worker_id = ? AND state = ?", workerId, "CANCELLED").
		Pluck("id", &ids).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return ids, nil
}

func (s *TaskStoreImpl) GetRetriableTasks(
	ctx context.Context,
	batchSize int,
) ([]*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	var tasks []*schema.Task
	err := db.Clauses(clause.Locking{
		Strength: clause.LockingStrengthUpdate,
		Options:  clause.LockingOptionsSkipLocked,
	}).Where("state = ? AND attempt_no < max_retry", "FAILED").
		Order("id ASC").
		Limit(batchSize).
		Find(&tasks).Error
	if err != nil {
		return nil, sql.WrapError(err)
	}
	return tasks, nil
}

func (s *TaskStoreImpl) GetStaleTasks(
	ctx context.Context,
	timeout int64,
	batchSize int,
) ([]*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	cutoff := time.Now().UnixMilli() - timeout
	var tasks []*schema.Task
	err := db.Clauses(clause.Locking{
		Strength: clause.LockingStrengthUpdate,
		Options:  clause.LockingOptionsSkipLocked,
	}).Where("state = ? AND last_heartbeat_time < ?", "RUNNING", cutoff).
		Order("last_heartbeat_time ASC").
		Limit(batchSize).
		Find(&tasks).Error
	if err != nil {
		return nil, sql.WrapError(err)
	}
	return tasks, nil
}

func (s *TaskStoreImpl) BatchUpdate(
	ctx context.Context,
	tasks []*schema.Task,
) error {
	if len(tasks) == 0 {
		return nil
	}
	db := util.GetDB(ctx, s.db)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, task := range tasks {
			if err := tx.Model(&schema.Task{}).
				Where("id = ?", task.Id).
				Updates(map[string]any{
					"state":               task.State,
					"attempt_no":          task.AttemptNo,
					"next_run_time":       task.NextRunTime,
					"worker_id":           task.WorkerId,
					"update_time":         task.UpdateTime,
					"error":               task.Error,
					"last_heartbeat_time": task.LastHeartbeatTime,
				}).Error; err != nil {
				return sql.WrapError(err)
			}
		}
		return nil
	})
}

func (s *TaskStoreImpl) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Delete(&schema.Task{}, "id = ?", id).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}

func (s *TaskStoreImpl) List(
	ctx context.Context,
	params *store.TaskListParams,
) ([]*schema.Task, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Model(&schema.Task{})

	if params.Namespace != "" {
		q = q.Where("namespace = ?", params.Namespace)
	}
	if params.TaskType != "" {
		q = q.Where("task_type = ?", params.TaskType)
	}
	if params.State != "" {
		q = q.Where("state = ?", params.State)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var tasks []*schema.Task
	if err := q.Offset(params.Offset).Limit(params.Limit).
		Order("create_time DESC").
		Find(&tasks).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	return tasks, total, nil
}
