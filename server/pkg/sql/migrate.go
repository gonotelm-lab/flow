package sql

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func MigrateUp(ctx context.Context, driver Driver, db *stdsql.DB, fsys fs.FS) error {
	if db == nil {
		return fmt.Errorf("migrate: db is nil")
	}
	if fsys == nil {
		return fmt.Errorf("migrate: migration fs is nil")
	}

	dialect, sessionLocker, err := gooseDialect(driver)
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		dialect,
		db,
		fsys,
		goose.WithSessionLocker(sessionLocker),
	)
	if err != nil {
		return fmt.Errorf("migrate: new provider failed: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: up failed: %w", err)
	}

	for _, result := range results {
		slog.InfoContext(ctx, "migration applied", slog.String("migration", result.String()))
	}

	return nil
}

func gooseDialect(driver Driver) (goose.Dialect, lock.SessionLocker, error) {
	switch driver {
	case DriverPgsql:
		sessionLocker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return "", nil, fmt.Errorf("migrate: new postgres session locker failed: %w", err)
		}
		return goose.DialectPostgres, sessionLocker, nil
	default:
		return "", nil, fmt.Errorf("migrate: driver %s is not supported", driver)
	}
}
