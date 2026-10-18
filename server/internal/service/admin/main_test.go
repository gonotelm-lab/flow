package admin

import (
	"fmt"
	"os"
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
	// 没配库也照常跑测试，需要数据库的用例由 requireTestDB 自己跳过。
	// 这样本包里不用数据库的单测（比如命名空间脱敏）在任何机器上都能跑。
	if os.Getenv(testsuite.EnvDBHost) == "" && os.Getenv(testsuite.EnvDBName) == "" {
		fmt.Println("FLOW_DB_* is not configured: running only the tests that need no database")
		os.Exit(m.Run())
	}

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

	code := m.Run()

	testdb.Cleanup()
	os.Exit(code)
}

// requireTestDB 让需要数据库的用例在没配库时跳过，和 scheduler/dispatcher_test.go 的做法一致。
func requireTestDB(t *testing.T) {
	t.Helper()

	if gTestStore == nil {
		t.Skip("FLOW_DB_* is not configured, skipping the integration tests that need a database")
	}
}
