package tx

import (
	"context"

	"gorm.io/gorm"
)

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