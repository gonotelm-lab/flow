package repository

import (
	"context"

	"gorm.io/gorm"
)

type TxManager struct {
	db *gorm.DB
}

type ttxKey struct{}

func WithTTx(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, ttxKey{}, db)
}

func GetTTx(ctx context.Context) *gorm.DB {
	db, ok := ctx.Value(ttxKey{}).(*gorm.DB)
	if !ok {
		return nil
	}

	return db
}

func (m *TxManager) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	tx := GetTTx(ctx)
	if tx != nil {
		return fn(ctx)
	}

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTTx(ctx, tx))
	})
}
