package impl

import (
	"fmt"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/postgres"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"gorm.io/gorm"
)

var errDriverNotSupported = fmt.Errorf("sql driver not supported")

func NewInstanceStore(driver sql.Driver, db *gorm.DB) (store.Instance, error) {
	switch driver {
	case sql.DriverPgsql:
		return postgres.NewInstanceStoreImpl(db), nil
	}

	return nil, errDriverNotSupported
}

func NewNamespaceStore(driver sql.Driver, db *gorm.DB) (store.Namespace, error) {
	switch driver {
	case sql.DriverPgsql:
		return postgres.NewNamespaceStoreImpl(db), nil
	}

	return nil, errDriverNotSupported
}

func NewGlobalRevisionStore(driver sql.Driver, db *gorm.DB) (store.GlobalRevision, error) {
	switch driver {
	case sql.DriverPgsql:
		return postgres.NewGlobalRevisionStoreImpl(db), nil
	}

	return nil, errDriverNotSupported
}

func NewInstanceEventStore(driver sql.Driver, db *gorm.DB) (store.InstanceEvent, error) {
	switch driver {
	case sql.DriverPgsql:
		return postgres.NewInstanceEventStoreImpl(db), nil
	}

	return nil, errDriverNotSupported
}
