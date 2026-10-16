package util

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/tx"
	"gorm.io/gorm"
)

// GetDB 取数据库连接，上下文里有事务就用事务连接，没有就用原始连接。
func GetDB(ctx context.Context, raw *gorm.DB) *gorm.DB {
	tx := tx.GetTTx(ctx)
	if tx != nil {
		return tx
	}

	return raw.WithContext(ctx)
}
