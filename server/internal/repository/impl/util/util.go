package util

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/tx"
	"gorm.io/gorm"
)

// GetDB 获取数据库连接，如果当前上下文有事务，则返回事务连接，否则返回原始连接。
func GetDB(ctx context.Context, raw *gorm.DB) *gorm.DB {
	tx := tx.GetTTx(ctx)
	if tx != nil {
		return tx
	}

	return raw
}
