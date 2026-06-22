package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
)

type TaskEvent interface {
	Append(ctx context.Context, event *schema.TaskEvent) error
	ListByTaskID(ctx context.Context, taskID uuid.UUID, limit int) ([]*schema.TaskEvent, error)
}
