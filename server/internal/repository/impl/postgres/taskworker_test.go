package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
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
	assert.ErrorIs(t, err, pkgerr.NoRecord)
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

func TestTaskWorkerStore_List(t *testing.T) {
	cleanTaskWorkers(t)
	ctx := context.Background()

	w1 := newTestTaskWorker("w1")
	w1.Namespace = "ns-wl-aaa"
	w1.TaskType = "email"
	w2 := newTestTaskWorker("w2")
	w2.Namespace = "ns-wl-aaa"
	w2.TaskType = "sms"
	w3 := newTestTaskWorker("w3")
	w3.Namespace = "ns-wl-bbb"
	w3.TaskType = "email"

	for _, w := range []*schema.TaskWorker{w1, w2, w3} {
		_, err := gTestTaskWorkerStore.Create(ctx, w)
		require.NoError(t, err)
	}

	t.Run("all", func(t *testing.T) {
		got, total, err := gTestTaskWorkerStore.List(ctx, &store.WorkerListParams{Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 3)
	})

	t.Run("filter_by_namespace", func(t *testing.T) {
		got, total, err := gTestTaskWorkerStore.List(ctx, &store.WorkerListParams{Namespace: "ns-wl-aaa", Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, got, 2)
	})

	t.Run("pagination", func(t *testing.T) {
		got, total, err := gTestTaskWorkerStore.List(ctx, &store.WorkerListParams{Offset: 0, Limit: 1})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 1)
	})
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
	assert.ErrorIs(t, err, pkgerr.NoRecord)
}
