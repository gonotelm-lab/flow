package postgres

import (
	"testing"

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
	const migrationFilePath = "../../../../../migration/pgsql18.sql"

	testdb, err := testsuite.NewTestGormDBFromEnv("pgsql")
	if err != nil {
		panic(err)
	}
	if err := testdb.Setup(migrationFilePath); err != nil {
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
