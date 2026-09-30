package postgres

import (
	"testing"

	"github.com/gonotelm-lab/flow/server/migration"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/gonotelm-lab/flow/server/pkg/sql/testsuite"
	"gorm.io/gorm"
)

var (
	gTestDB                  *gorm.DB
	gTestInstanceStore       *InstanceStoreImpl
	gTestNamespaceStore      *NamespaceStoreImpl
	gTestGlobalRevisionStore *GlobalRevisionStoreImpl
	gTestInstanceEventStore  *InstanceEventStoreImpl
	gTestTaskStore           *TaskStoreImpl
	gTestTaskWorkerStore     *TaskWorkerStoreImpl
	gTestTaskEventStore      *TaskEventStoreImpl
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
	gTestInstanceStore = &InstanceStoreImpl{db: gTestDB}
	gTestNamespaceStore = &NamespaceStoreImpl{db: gTestDB}
	gTestGlobalRevisionStore = &GlobalRevisionStoreImpl{db: gTestDB}
	gTestInstanceEventStore = &InstanceEventStoreImpl{db: gTestDB}
	gTestTaskStore = &TaskStoreImpl{db: gTestDB}
	gTestTaskWorkerStore = &TaskWorkerStoreImpl{db: gTestDB}
	gTestTaskEventStore = &TaskEventStoreImpl{db: gTestDB}

	m.Run()

	testdb.Cleanup()
}
