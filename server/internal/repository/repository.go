package repository

import (
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"gorm.io/gorm"
)

type Impl struct {
	db        *gorm.DB
	txManager *TxManager
	store     *Store
}

func newRepository(driver sql.Driver, db *gorm.DB) (*Impl, error) {
	store, err := newStore(driver, db)
	if err != nil {
		return nil, err
	}

	return &Impl{
		db:        db,
		txManager: &TxManager{db: db},
		store:     store,
	}, nil
}

func (s *Impl) DB() *gorm.DB {
	return s.db
}

func (s *Impl) TxManager() *TxManager {
	return s.txManager
}

func (s *Impl) Store() *Store {
	return s.store
}

func (s *Impl) Close() error {
	sqlDb, err := s.db.DB()
	if err != nil {
		return err
	}

	return sqlDb.Close()
}
