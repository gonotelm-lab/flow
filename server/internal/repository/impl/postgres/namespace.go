package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"

	pkgerr "github.com/pkg/errors"
	"gorm.io/gorm"
)

type NamespaceStoreImpl struct {
	db *gorm.DB
}

func NewNamespaceStoreImpl(db *gorm.DB) store.Namespace {
	return &NamespaceStoreImpl{db: db}
}

func (s *NamespaceStoreImpl) Create(
	ctx context.Context, namespace *schema.Namespace,
) (*schema.Namespace, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(namespace).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db create failed")
	}
	return namespace, nil
}

func (s *NamespaceStoreImpl) Get(
	ctx context.Context, name string,
) (*schema.Namespace, error) {
	db := util.GetDB(ctx, s.db)
	var namespace schema.Namespace
	if err := db.Where("name = ?", name).First(&namespace).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db first failed")
	}

	return &namespace, nil
}
