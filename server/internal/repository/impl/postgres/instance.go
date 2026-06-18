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

type InstanceStoreImpl struct {
	db *gorm.DB
}

func NewInstanceStoreImpl(db *gorm.DB) store.Instance {
	return &InstanceStoreImpl{db: db}
}

func (s *InstanceStoreImpl) Create(
	ctx context.Context, instance *schema.Instance,
) (*schema.Instance, error) {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(instance).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db create failed")
	}
	return instance, nil
}

func (s *InstanceStoreImpl) Delete(ctx context.Context, id int64) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Delete(&schema.Instance{}, id).Error; err != nil {
		return pkgerr.Wrap(err, "db delete failed")
	}
	return nil
}

func (s *InstanceStoreImpl) Get(
	ctx context.Context, id int64,
) (*schema.Instance, error) {
	db := util.GetDB(ctx, s.db)
	var instance schema.Instance
	if err := db.Where("id = ?", id).First(&instance).Error; err != nil {
		if stderr.Is(err, gorm.ErrRecordNotFound) {
			return nil, sql.ErrNoRecord
		}

		return nil, pkgerr.Wrap(err, "db first failed")
	}

	return &instance, nil
}

func (s *InstanceStoreImpl) UpdateExpireTime(
	ctx context.Context, id int64,
	expireTimeMs int64,
	expectToken int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Model(&schema.Instance{}).
		Where("id = ?", id).
		Where("fencing_token = ?", expectToken).
		Updates(map[string]any{
			"expire_time": expireTimeMs,
		})
	err := res.Error
	if err != nil {
		return false, pkgerr.Wrap(err, "db update failed")
	}

	return res.RowsAffected > 0, nil
}

func (s *InstanceStoreImpl) ListExpired(
	ctx context.Context,
	expireBeforeMs int64,
	limit int,
) ([]*schema.Instance, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Where("expire_time <= ?", expireBeforeMs).
		Order("expire_time ASC").
		Order("id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}

	var instances []*schema.Instance
	if err := q.Find(&instances).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db find failed")
	}

	return instances, nil
}

func (s *InstanceStoreImpl) DeleteExpired(
	ctx context.Context,
	id int64,
	expireBeforeMs int64,
) (bool, error) {
	db := util.GetDB(ctx, s.db)
	res := db.Where("id = ?", id).
		Where("expire_time <= ?", expireBeforeMs).
		Delete(&schema.Instance{})
	if err := res.Error; err != nil {
		return false, pkgerr.Wrap(err, "db delete failed")
	}

	return res.RowsAffected > 0, nil
}
