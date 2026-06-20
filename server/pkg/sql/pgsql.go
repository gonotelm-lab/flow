package sql

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenPgSql(config *Config) (*gorm.DB, error) {
	return OpenPgSqlWithLogger(config, nil)
}

func OpenPgSqlWithLogger(config *Config, lg logger.Interface) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Host, config.Port, config.User, config.Password, config.DbName,
	)
	if lg == nil {
		lg = NewGormSlogger(nil)
	}
	gormConfig := &gorm.Config{
		Logger:         lg,
		QueryFields:    true,
		PrepareStmt:    true,
		TranslateError: true,
	}
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}
	return db, nil
}
