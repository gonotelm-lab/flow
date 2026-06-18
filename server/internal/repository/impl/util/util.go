package util

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"gorm.io/gorm"
)

func GetDB(ctx context.Context, raw *gorm.DB) *gorm.DB {
	tx := repository.GetTTx(ctx)
	if tx != nil {
		return tx
	}

	return raw
}
