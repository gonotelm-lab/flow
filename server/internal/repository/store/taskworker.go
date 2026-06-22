package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type TaskWorker interface {
	Create(ctx context.Context, worker *schema.TaskWorker) (*schema.TaskWorker, error)
	Get(ctx context.Context, id int64) (*schema.TaskWorker, error)
	UpdateHeartbeat(ctx context.Context, id int64, heartbeatTime int64) (bool, error)
	Delete(ctx context.Context, id int64) error
}
