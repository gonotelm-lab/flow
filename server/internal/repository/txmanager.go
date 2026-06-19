package repository

import (
	"context"

	repotx "github.com/gonotelm-lab/flow/server/internal/repository/tx"

	"gorm.io/gorm"
)

type TxManager struct {
	db *gorm.DB
}

func (m *TxManager) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	tx := repotx.GetTTx(ctx)
	if tx != nil {
		return fn(ctx)
	}

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(repotx.WithTTx(ctx, tx))
	})
}
