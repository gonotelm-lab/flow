package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanTaskWorkers(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM task_workers")
}

func newTestTaskWorker(name string) *schema.TaskWorker {
	now := time.Now().UnixMilli()
	return &schema.TaskWorker{
		Name:          fmt.Sprintf("%s-%d", name, now),
		Namespace:     "ns-test",
		TaskType:      "artifact",
		CreateTime:    now,
		HeartbeatTime: now,
		LastWorkTime:  0,
		TotalDealt:    0,
		SuccessDealt:  0,
	}
}

func TestTaskWorkerStore_Create(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	worker := newTestTaskWorker("create")
	got, err := gTestTaskWorkerStore.Create(ctx, worker)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotZero(t, got.Id)
	assert.Equal(t, worker.Name, got.Name)
	assert.Equal(t, worker.Namespace, got.Namespace)
	assert.Equal(t, worker.TaskType, got.TaskType)
}

func TestTaskWorkerStore_Get(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	worker := newTestTaskWorker("get")
	created, err := gTestTaskWorkerStore.Create(ctx, worker)
	require.NoError(t, err)

	got, err := gTestTaskWorkerStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, created.Namespace, got.Namespace)
	assert.Equal(t, created.TaskType, got.TaskType)
	assert.Equal(t, created.HeartbeatTime, got.HeartbeatTime)
}

func TestTaskWorkerStore_Get_NotFound(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	_, err := gTestTaskWorkerStore.Get(ctx, 999999)
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestTaskWorkerStore_UpdateHeartbeat(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	worker := newTestTaskWorker("heartbeat")
	created, err := gTestTaskWorkerStore.Create(ctx, worker)
	require.NoError(t, err)

	newHeartbeat := created.HeartbeatTime + 3000
	ok, err := gTestTaskWorkerStore.UpdateHeartbeat(ctx, created.Id, newHeartbeat)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestTaskWorkerStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, newHeartbeat, got.HeartbeatTime)
}

func TestTaskWorkerStore_Delete(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	worker := newTestTaskWorker("delete")
	created, err := gTestTaskWorkerStore.Create(ctx, worker)
	require.NoError(t, err)

	err = gTestTaskWorkerStore.Delete(ctx, created.Id)
	require.NoError(t, err)

	_, err = gTestTaskWorkerStore.Get(ctx, created.Id)
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}
