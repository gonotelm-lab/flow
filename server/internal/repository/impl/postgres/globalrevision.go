package postgres

import (
	"context"
	stderr "errors"

	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"

	pkgerr "github.com/pkg/errors"
	"gorm.io/gorm"
)

type GlobalRevisionStoreImpl struct {
	db *gorm.DB
}

func NewGlobalRevisionStoreImpl(db *gorm.DB) store.GlobalRevision {
	return &GlobalRevisionStoreImpl{db: db}
}

func (s *GlobalRevisionStoreImpl) GetOrInitForUpdate(
	ctx context.Context,
	zero *schema.GlobalRevision,
) (*schema.GlobalRevision, error) {
	db := util.GetDB(ctx, s.db)
	var rev schema.GlobalRevision

	// INSERT + no-op ON CONFLICT UPDATE 获取排他行锁，RETURNING 一次返回真实值
	err := db.Raw(
		`INSERT INTO global_revisions (name, current_revision, update_time)
		VALUES (?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING *`, zero.Name, zero.CurrentRevision, zero.UpdateTime,
	).Scan(&rev).Error
	if err != nil {
		return nil, pkgerr.Wrap(err, "db raw exec failed")
	}

	return &rev, nil
}

func (s *GlobalRevisionStoreImpl) IncrRevision(
	ctx context.Context,
	name string,
	updateTime int64,
) error {
	db := util.GetDB(ctx, s.db)
	err := db.Model(&schema.GlobalRevision{}).
		Where("name = ?", name).
		Updates(map[string]any{
			"current_revision": gorm.Expr("current_revision + 1"),
			"update_time":      updateTime,
		}).Error
	if err != nil {
		return pkgerr.Wrap(err, "db update failed")
	}

	return nil
}

func (s *GlobalRevisionStoreImpl) Get(
	ctx context.Context,
	name string,
) (*schema.GlobalRevision, error) {
	db := util.GetDB(ctx, s.db)
	var rev schema.GlobalRevision
	if err := db.Where("name = ?", name).First(&rev).Error; err != nil {
		if stderr.Is(err, gorm.ErrRecordNotFound) {
			return nil, sql.ErrNoRecord
		}

		return nil, pkgerr.Wrap(err, "db first failed")
	}

	return &rev, nil
}
