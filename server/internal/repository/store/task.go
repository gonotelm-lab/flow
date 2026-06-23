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

type Task interface {
	Create(ctx context.Context, task *schema.Task) (*schema.Task, error)
	Get(ctx context.Context, id uuid.UUID) (*schema.Task, error)

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
}
