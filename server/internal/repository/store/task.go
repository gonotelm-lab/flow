package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
)

type TaskClaimUpdateParams struct {
	WorkerId   int64
	NewState   string
	UpdateTime int64
}

type TaskUpdateOutcomeParams struct {
	Payload    []byte
	UpdateTime int64
}

type TaskBatchUpdateParams struct {
	State       string
	AttemptNo   int
	NextRunTime int64
	WorkerId    int64
	UpdateTime  int64
	Error       []byte
}

type TaskListParams struct {
	Namespace string
	TaskType  string
	State     string
	Id        string
	Offset    int
	Limit     int
}

type Task interface {
	Create(ctx context.Context, task *schema.Task) (*schema.Task, error)
	Get(ctx context.Context, id uuid.UUID) (*schema.Task, error)
	Delete(ctx context.Context, id uuid.UUID) error

	List(ctx context.Context, params *TaskListParams) ([]*schema.Task, int64, error)

	// 获取下一个可执行的任务
	Claim(ctx context.Context, namespace, taskType string, states []string) (*schema.Task, error)

	// 和Claim配合使用 用来抢占任务
	ClaimUpdate(ctx context.Context, id uuid.UUID, oldState string, params *TaskClaimUpdateParams) (bool, error)

	Update(ctx context.Context, task *schema.Task) (bool, error)

	UpdateOutcome(ctx context.Context,
		id uuid.UUID,
		success bool,
		workerId int64,
		oldState, newState string,
		params *TaskUpdateOutcomeParams,
	) (bool, error)

	// UpdateHeartbeat 批量更新任务心跳时间（仅限 state=RUNNING 且 worker_id 匹配）
	UpdateHeartbeat(ctx context.Context, ids []uuid.UUID, workerId int64, heartTime int64) error

	// GetCancelledTasks 查询指定 worker 的已取消任务
	GetCancelledTasks(ctx context.Context, workerId int64) ([]uuid.UUID, error)

	// GetRetriableTasks 查询可重试的失败任务
	GetRetriableTasks(ctx context.Context, batchSize int) ([]*schema.Task, error)

	// GetStaleTasks 查询失联任务（last_heartbeat_time < now - timeout）
	GetStaleTasks(ctx context.Context, timeout int64, batchSize int) ([]*schema.Task, error)

	// BatchUpdate 批量保存任务变更
	BatchUpdate(ctx context.Context, tasks []*schema.Task) error
}
