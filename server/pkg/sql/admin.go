package sql

import (
	stderrors "errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var pgIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

func ValidatePgIdentifier(identifier string) error {
	if !pgIdentifierPattern.MatchString(identifier) {
		return fmt.Errorf("invalid postgres identifier: %s", identifier)
	}
	return nil
}

func QuotePgIdentifier(identifier string) (string, error) {
	if err := ValidatePgIdentifier(identifier); err != nil {
		return "", err
	}
	return fmt.Sprintf(`"%s"`, identifier), nil
}

func OpenPgAdminDB(cfg *Config, lg gormlogger.Interface) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("db config is nil")
	}

	candidates := make([]string, 0, 3)
	candidates = append(candidates, "postgres", "template1")
	if name := strings.TrimSpace(cfg.DbName); name != "" {
		candidates = append(candidates, name)
	}

	var errs []error
	for _, dbName := range candidates {
		adminConfig := *cfg
		adminConfig.DbName = dbName

		db, err := OpenPgSqlWithLogger(&adminConfig, lg)
		if err == nil {
			return db, nil
		}
		errs = append(errs, fmt.Errorf("connect %s failed: %w", dbName, err))
	}

	return nil, stderrors.Join(errs...)
}

func PgDatabaseExists(db *gorm.DB, name string) (bool, error) {
	var count int64
	if err := db.Raw("SELECT COUNT(1) FROM pg_database WHERE datname = ?", name).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("query pg_database failed: %w", err)
	}
	return count > 0, nil
}

func CreatePgDatabase(cfg *Config, dbName string, lg gormlogger.Interface) error {
	name := strings.TrimSpace(dbName)
	if err := ValidatePgIdentifier(name); err != nil {
		return err
	}

	adminDB, err := OpenPgAdminDB(cfg, lg)
	if err != nil {
		return fmt.Errorf("open admin db failed: %w", err)
	}
	defer closeGorm(adminDB)

	exists, err := PgDatabaseExists(adminDB, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	quotedName, err := QuotePgIdentifier(name)
	if err != nil {
		return err
	}

	if err := adminDB.Exec("CREATE DATABASE " + quotedName).Error; err != nil {
		if exists, checkErr := PgDatabaseExists(adminDB, name); checkErr == nil && exists {
			return nil
		}
		return fmt.Errorf("create database %s failed: %w", name, err)
	}

	return nil
}

func DropPgDatabase(cfg *Config, dbName string, lg gormlogger.Interface) error {
	quotedName, err := QuotePgIdentifier(dbName)
	if err != nil {
		return err
	}

	adminDB, err := OpenPgAdminDB(cfg, lg)
	if err != nil {
		return fmt.Errorf("open admin db failed: %w", err)
	}
	defer closeGorm(adminDB)

	dropWithForceSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quotedName)
	if err := adminDB.Exec(dropWithForceSQL).Error; err != nil {
		dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s", quotedName)
		if fallbackErr := adminDB.Exec(dropSQL).Error; fallbackErr != nil {
			return fmt.Errorf("drop database %s failed, force=%v fallback=%v", dbName, err, fallbackErr)
		}
	}

	return nil
}

func EnsurePgDatabase(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("db config is nil")
	}
	return CreatePgDatabase(cfg, cfg.DbName, nil)
}

func closeGorm(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
