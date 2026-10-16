package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type GlobalRevision interface {
	// 取一条记录，不存在就插入 zero。
	// 并发安全要靠外层事务包住这个方法。
	GetOrInitForUpdate(ctx context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error)

	// 递增 revision
	IncrRevision(ctx context.Context, name string, updateTime int64) error

	Get(ctx context.Context, name string) (*schema.GlobalRevision, error)
}
