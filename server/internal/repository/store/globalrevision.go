package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type GlobalRevision interface {
	// 获取一条记录 如果记录不存在 会插入一条zero
	// 需要在外部使用事务包裹此方法以保证并发安全性
	GetOrInitForUpdate(ctx context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error)

	// 更新revision
	IncrRevision(ctx context.Context, name string, updateTime int64) error

	Get(ctx context.Context, name string) (*schema.GlobalRevision, error)
}
