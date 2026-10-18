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

// GetByApiKey 按 api_key 反查 namespace，客户端面鉴权用它。
//
// api_key 列上有唯一约束 uk_api_key，这是一次索引点查。不做缓存：和调度器取签名
// 密钥的理由一样，缓存会出现「key 换了、旧值还在用」的窗口。
func (s *NamespaceStoreImpl) GetByApiKey(
	ctx context.Context, apiKey string,
) (*schema.Namespace, error) {
	db := util.GetDB(ctx, s.db)
	var namespace schema.Namespace
	if err := db.Where("api_key = ?", apiKey).First(&namespace).Error; err != nil {
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

func (s *NamespaceStoreImpl) Update(
	ctx context.Context,
	ns *schema.Namespace,
) error {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.Namespace{}).
		Where("name = ?", ns.Name).
		Updates(map[string]any{
			"description": ns.Description,
			"creator":     ns.Creator,
		})
	if err := res.Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}
