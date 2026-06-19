package repository

import (
	"github.com/gonotelm-lab/flow/server/internal/repository/impl"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"gorm.io/gorm"
)

type Store struct {
	Instance       store.Instance
	InstanceEvent  store.InstanceEvent
	GlobalRevision store.GlobalRevision
	Namespace      store.Namespace
}

func newStore(driver sql.Driver, db *gorm.DB) (*Store, error) {
	instance, err := impl.NewInstanceStore(driver, db)
	if err != nil {
		return nil, err
	}
	instanceEvent, err := impl.NewInstanceEventStore(driver, db)
	if err != nil {
		return nil, err
	}
	namespace, err := impl.NewNamespaceStore(driver, db)
	if err != nil {
		return nil, err
	}

	globalRevision, err := impl.NewGlobalRevisionStore(driver, db)
	if err != nil {
		return nil, err
	}

	return &Store{
		Instance:       instance,
		InstanceEvent:  instanceEvent,
		GlobalRevision: globalRevision,
		Namespace:      namespace,
	}, nil
}
