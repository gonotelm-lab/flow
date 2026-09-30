package repository

import (
	"context"
	"fmt"
	"log/slog"

	serverotel "github.com/gonotelm-lab/flow/server/internal/otel"
	"github.com/gonotelm-lab/flow/server/migration"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"gorm.io/gorm"
)

var (
	gRepo *Impl
	gDb   *gorm.DB
)

func MustInit(driver sql.Driver, c *sql.Config, autoInit bool) {
	ctx := context.Background()

	db, err := sql.Open(driver, c)
	if err != nil && autoInit {
		if driver != sql.DriverPgsql {
			panic(fmt.Errorf("open db failed: %w", err))
		}
		if ensureErr := sql.EnsurePgDatabase(c); ensureErr != nil {
			panic(fmt.Errorf("open db failed: %w, ensure database failed: %v", err, ensureErr))
		}
		db, err = sql.Open(driver, c)
	}
	if err != nil {
		panic(err)
	}

	if err := serverotel.InstallGormTrace(db); err != nil {
		panic(err)
	}

	if autoInit {
		if err := migrateUp(ctx, driver, db); err != nil {
			panic(err)
		}
	}

	gDb = db
	gRepo, err = newRepository(driver, db)
	if err != nil {
		panic(fmt.Errorf("new repository failed: %w", err))
	}

	slog.Info("repository initialized", "driver", driver)
}

func migrateUp(ctx context.Context, driver sql.Driver, db *gorm.DB) error {
	fsys, ok := migration.FSFor(driver)
	if !ok {
		slog.WarnContext(ctx, "no migrations registered for driver, skip auto init", "driver", driver)
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db failed: %w", err)
	}
	if err := sql.MigrateUp(ctx, driver, sqlDB, fsys); err != nil {
		return fmt.Errorf("migrate up failed: %w", err)
	}

	return nil
}

func Repo() *Impl {
	return gRepo
}

func Close() {
	gRepo.Close()
}
