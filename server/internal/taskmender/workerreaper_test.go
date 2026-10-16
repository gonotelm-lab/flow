package taskmender

import (
	"context"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/config"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRetention = 7 * 24 * time.Hour

func TestWorkerReaper_ReapDeletesOnlyPastRetention(t *testing.T) {
	ctx := context.Background()
	cleanWorkers(t)

	now := time.Now().UnixMilli()
	fresh := createWorker(t, ctx, "fresh", now)
	recentlyExpired := createWorker(t, ctx, "recently-expired", now-testRetention.Milliseconds()/2)
	longGoneA := createWorker(t, ctx, "long-gone-a", now-testRetention.Milliseconds()-time.Hour.Milliseconds())
	longGoneB := createWorker(t, ctx, "long-gone-b", now-testRetention.Milliseconds()-2*time.Hour.Milliseconds())

	mender := NewWorkerReaper(gTestStore, &config.WorkerConfig{
		WorkerRetention: testRetention,
		WorkerReapBatch: 500,
	})
	mender.reap(ctx)

	ids := listWorkerIds(t, ctx)
	assert.ElementsMatch(t, []int64{fresh.Id, recentlyExpired.Id}, ids, "只应删掉超过保留期的两行")
	assert.NotContains(t, ids, longGoneA.Id)
	assert.NotContains(t, ids, longGoneB.Id)
}

func TestWorkerReaper_ReapLoopsUntilDrained(t *testing.T) {
	ctx := context.Background()
	cleanWorkers(t)

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		createWorker(t, ctx, "backlog", now-testRetention.Milliseconds()-int64(i+1)*time.Hour.Milliseconds())
	}

	mender := NewWorkerReaper(gTestStore, &config.WorkerConfig{
		WorkerRetention: testRetention,
		WorkerReapBatch: 2,
	})
	mender.reap(ctx)

	assert.Empty(t, listWorkerIds(t, ctx), "batch=2 时应循环 3 轮把 5 行全部删完")
}

func cleanWorkers(t *testing.T) {
	t.Helper()
	require.NoError(t, gTestDB.Exec("DELETE FROM task_workers").Error)
}

func createWorker(t *testing.T, ctx context.Context, name string, heartbeat int64) *reposchema.TaskWorker {
	t.Helper()
	worker := &reposchema.TaskWorker{
		Name:          name,
		Namespace:     "ns-reaper",
		TaskType:      "artifact",
		CreateTime:    heartbeat,
		HeartbeatTime: heartbeat,
	}
	created, err := gTestStore.TaskWorker.Create(ctx, worker)
	require.NoError(t, err)

	return created
}

func listWorkerIds(t *testing.T, ctx context.Context) []int64 {
	t.Helper()
	workers, _, err := gTestStore.TaskWorker.List(ctx, &store.WorkerListParams{Limit: 100})
	require.NoError(t, err)

	ids := make([]int64, 0, len(workers))
	for _, w := range workers {
		ids = append(ids, w.Id)
	}

	return ids
}

// Close 必须真正停住 Start 起的循环，而且要能重复调用。
func TestWorkerReaper_StartThenCloseIsPromptAndIdempotent(t *testing.T) {
	for i := 0; i < 20; i++ {
		mender := NewWorkerReaper(gTestStore, &config.WorkerConfig{
			WorkerRetention:    testRetention,
			WorkerReapInterval: time.Hour,
			WorkerReapBatch:    10,
		})
		mender.Start(context.Background())

		done := make(chan struct{})
		go func() {
			mender.Close()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Close 没能停住回收循环")
		}

		mender.Close() // 幂等
	}
}
