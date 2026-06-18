package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type Instance interface {
	Create(ctx context.Context, instance *schema.Instance) (*schema.Instance, error)
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*schema.Instance, error)
	ListActive(ctx context.Context, aliveAfterMs int64) ([]*schema.Instance, error)
	UpdateExpireTime(ctx context.Context, id int64, expireTimeMs, expectToken int64) (bool, error)
	ListExpired(ctx context.Context, expireBeforeMs int64, limit int) ([]*schema.Instance, error)
	DeleteExpired(ctx context.Context, id int64, expireBeforeMs int64) (bool, error)
}
