package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
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
		return nil, sql.WrapError(err)
	}

	return namespace, nil
}

func (s *NamespaceStoreImpl) Get(
	ctx context.Context, name string,
) (*schema.Namespace, error) {
	db := util.GetDB(ctx, s.db)
	var namespace schema.Namespace
	if err := db.Where("name = ?", name).First(&namespace).Error; err != nil {
		return nil, sql.WrapError(err)
	}

	return &namespace, nil
}

func (s *NamespaceStoreImpl) List(
	ctx context.Context,
	offset, limit int,
) ([]*schema.Namespace, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Model(&schema.Namespace{})

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var namespaces []*schema.Namespace
	if err := q.Offset(offset).Limit(limit).
		Order("create_time DESC").
		Find(&namespaces).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	return namespaces, total, nil
}
