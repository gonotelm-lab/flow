package admin

import (
	"context"
	"testing"
	"time"

	adminv1 "github.com/gonotelm-lab/flow/api/admin/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorkers_HidesExpiredWorkersAndReportsDealtStats(t *testing.T) {
	ctx := context.Background()
	cleanWorkersAndTasks(t)

	const ns = "ns-admin-workers"
	now := time.Now().UnixMilli()

	alive := createWorker(t, ctx, ns, "alive", now)
	expired := createWorker(t, ctx, ns, "expired", now-30*time.Minute.Milliseconds())

	createTask(t, ctx, ns, "DONE", alive.Id, now-3_000)
	createTask(t, ctx, ns, "DONE", alive.Id, now-2_000)
	createTask(t, ctx, ns, "FAILED", alive.Id, now-1_000)
	createTask(t, ctx, ns, "RUNNING", alive.Id, now)
	createTask(t, ctx, ns, "DONE", expired.Id, now)

	svc := NewService(gTestStore, nil, ServiceConfig{WorkerTTL: time.Minute})

	resp, err := svc.ListWorkers(ctx, &adminv1.ListWorkersRequest{
		Page:      &adminv1.PageRequest{Page: 1, PageSize: 20},
		Namespace: ns,
	})
	require.NoError(t, err)

	assert.Equal(t, int64(1), resp.Page.GetTotalCount(), "过期节点不该计入 totalCount")
	require.Len(t, resp.Workers, 1)

	got := resp.Workers[0]
	assert.Equal(t, alive.Id, got.GetId())
	assert.Equal(t, int64(3), got.GetTotalDealt(), "DONE + FAILED 才是处理量")
	assert.Equal(t, int64(2), got.GetSuccessDealt())
	require.NotNil(t, got.GetLastWorkTime(), "最近工作应来自 tasks 聚合")
	assert.Equal(t, now-1_000, got.GetLastWorkTime().AsTime().UnixMilli())

	t.Run("直查过期节点仍可读", func(t *testing.T) {
		detail, err := svc.GetWorker(ctx, &adminv1.GetWorkerRequest{Id: expired.Id})
		require.NoError(t, err)
		assert.Equal(t, expired.Id, detail.GetId())
		assert.Equal(t, int64(1), detail.GetTotalDealt())
		assert.Equal(t, int64(1), detail.GetSuccessDealt())
	})

	t.Run("没有任务过的新节点显示 0/0", func(t *testing.T) {
		fresh := createWorker(t, ctx, ns, "fresh", now)
		detail, err := svc.GetWorker(ctx, &adminv1.GetWorkerRequest{Id: fresh.Id})
		require.NoError(t, err)
		assert.Equal(t, int64(0), detail.GetTotalDealt())
		assert.Equal(t, int64(0), detail.GetSuccessDealt())
		assert.Nil(t, detail.GetLastWorkTime())
	})
}

func TestListWorkers_ZeroTTLKeepsExpiredWorkers(t *testing.T) {
	ctx := context.Background()
	cleanWorkersAndTasks(t)

	const ns = "ns-admin-workers-ttl0"
	now := time.Now().UnixMilli()
	createWorker(t, ctx, ns, "expired", now-30*time.Minute.Milliseconds())

	svc := NewService(gTestStore, nil, ServiceConfig{})

	resp, err := svc.ListWorkers(ctx, &adminv1.ListWorkersRequest{
		Page:      &adminv1.PageRequest{Page: 1, PageSize: 20},
		Namespace: ns,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Page.GetTotalCount())
	assert.Len(t, resp.Workers, 1)
}

func cleanWorkersAndTasks(t *testing.T) {
	t.Helper()
	require.NoError(t, gTestDB.Exec("DELETE FROM tasks").Error)
	require.NoError(t, gTestDB.Exec("DELETE FROM task_workers").Error)
}

func createWorker(t *testing.T, ctx context.Context, namespace, name string, heartbeat int64) *reposchema.TaskWorker {
	t.Helper()
	worker := &reposchema.TaskWorker{
		Name:          name,
		Namespace:     namespace,
		TaskType:      "artifact",
		CreateTime:    heartbeat,
		HeartbeatTime: heartbeat,
	}
	created, err := gTestStore.TaskWorker.Create(ctx, worker)
	require.NoError(t, err)

	return created
}

func createTask(t *testing.T, ctx context.Context, namespace, state string, workerId, updateTime int64) {
	t.Helper()
	task := &reposchema.Task{
		Id:          uuid.New(),
		Namespace:   namespace,
		TaskType:    "artifact",
		State:       state,
		Payload:     []byte(`{}`),
		CreateTime:  updateTime,
		NextRunTime: updateTime,
		UpdateTime:  updateTime,
		WorkerId:    workerId,
		MaxRetry:    3,
	}
	_, err := gTestStore.Task.Create(ctx, task)
	require.NoError(t, err)
}
