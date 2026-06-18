package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type InstanceEvent interface {
	Append(ctx context.Context, event *schema.InstanceEvent) error
	Last(ctx context.Context, group string) (*schema.InstanceEvent, error)
	List(ctx context.Context, group string, lastRevision int64, limit int) ([]*schema.InstanceEvent, error)
}
