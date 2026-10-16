package admin

import (
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/migration"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/gonotelm-lab/flow/server/pkg/sql/testsuite"
	"gorm.io/gorm"
)

var (
	gTestDB    *gorm.DB
	gTestStore *repository.Store
)

func TestMain(m *testing.M) {
	testdb, err := testsuite.NewTestGormDBFromEnv("pgsql")
	if err != nil {
		panic(err)
	}

	fsys, ok := migration.FSFor(sql.DriverPgsql)
	if !ok {
		panic("no migration fs registered for pgsql")
	}
	if err := testdb.Setup(fsys); err != nil {
		panic(err)
	}

	gTestDB = testdb.GetDB()

	repo, err := repository.New(sql.DriverPgsql, gTestDB)
	if err != nil {
		panic(err)
	}
	gTestStore = repo.Store()

	m.Run()

	testdb.Cleanup()
}
