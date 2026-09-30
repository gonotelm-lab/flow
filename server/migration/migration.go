package migration

import (
	"io/fs"

	"github.com/gonotelm-lab/flow/server/migration/postgres"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
)

func FSFor(driver sql.Driver) (fs.FS, bool) {
	switch driver {
	case sql.DriverPgsql:
		return postgres.FS, true
	default:
		return nil, false
	}
}
