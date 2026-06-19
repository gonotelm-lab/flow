package repository

import (
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"gorm.io/gorm"
)

type repository struct {
	db        *gorm.DB
	txManager *TxManager
	store     *Store
}

func newRepository(driver sql.Driver, db *gorm.DB) (*repository, error) {
	store, err := newStore(driver, db)
	if err != nil {
		return nil, err
	}

	return &repository{
		db:        db,
		txManager: &TxManager{db: db},
		store:     store,
	}, nil
}

func (s *repository) DB() *gorm.DB {
	return s.db
}

func (s *repository) TxManager() *TxManager {
	return s.txManager
}

func (s *repository) Store() *Store {
	return s.store
}

func (s *repository) Close() error {
	sqlDb, err := s.db.DB()
	if err != nil {
		return err
	}

	return sqlDb.Close()
}
