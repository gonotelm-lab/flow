package repository

import (
	"github.com/gonotelm-lab/flow/server/internal/repository/store"

	"gorm.io/gorm"
)

type Singleton struct {
	db        *gorm.DB
	txManager *TxManager
	store     *Store
}

func NewSingleton(db *gorm.DB) *Singleton {
	return &Singleton{
		db:        db,
		txManager: &TxManager{db: db},
		store:     &Store{},
	}
}

func (s *Singleton) DB() *gorm.DB {
	return s.db
}

func (s *Singleton) TxManager() *TxManager {
	return s.txManager
}

func (s *Singleton) Store() *Store {
	return s.store
}

func (s *Singleton) Close() error {
	sqlDb, err := s.db.DB()
	if err != nil {
		return err
	}

	return sqlDb.Close()
}

type Store struct {
	Instance       store.Instance
	InstanceEvent  store.InstanceEvent
	GlobalRevision store.GlobalRevision
	Namespace      store.Namespace
}
