package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"gorm.io/gorm"

	pkgerr "github.com/pkg/errors"
)

type InstanceEventStoreImpl struct {
	db *gorm.DB
}

func NewInstanceEventStoreImpl(db *gorm.DB) store.InstanceEvent {
	return &InstanceEventStoreImpl{db: db}
}

func (s *InstanceEventStoreImpl) Append(
	ctx context.Context,
	event *schema.InstanceEvent,
) error {
	db := util.GetDB(ctx, s.db)
	if err := db.Create(event).Error; err != nil {
		return pkgerr.Wrap(err, "db create failed")
	}

	return nil
}

func (s *InstanceEventStoreImpl) Last(
	ctx context.Context,
	group string,
) (*schema.InstanceEvent, error) {
	db := util.GetDB(ctx, s.db)
	var event schema.InstanceEvent
	if err := db.Where(`"group" = ?`, group).
		Order("revision DESC").
		First(&event).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db first failed")
	}
	return &event, nil
}

func (s *InstanceEventStoreImpl) List(
	ctx context.Context,
	group string,
	lastRevision int64,
	limit int,
) ([]*schema.InstanceEvent, error) {
	db := util.GetDB(ctx, s.db)
	var events []*schema.InstanceEvent
	if err := db.Where(`"group" = ? AND revision > ?`, group, lastRevision).
		Order("revision ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, pkgerr.Wrap(err, "db find failed")
	}
	return events, nil
}
