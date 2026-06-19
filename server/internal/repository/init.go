package repository

import (
	"fmt"
	"log/slog"

	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"gorm.io/gorm"
)

var (
	gRepo *repository
	gDb   *gorm.DB
)

func MustInit(driver sql.Driver, c *sql.Config) {
	db, err := sql.Open(driver, c)
	if err != nil {
		panic(err)
	}

	gDb = db
	gRepo, err = newRepository(driver, db)
	if err != nil {
		panic(fmt.Errorf("new repository failed: %w", err))
	}

	slog.Info("repository initialized", "driver", driver)
}

func Repository() *repository {
	return gRepo
}
