package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanGlobalRevisions(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM global_revisions")
}

func TestGetOrInitForUpdate_Init(t *testing.T) {
	cleanGlobalRevisions(t)
	ctx := context.Background()

	zero := &schema.GlobalRevision{
		Name:            "instance_events",
		CurrentRevision: 0,
		UpdateTime:      0,
	}
	got, err := gTestGlobalRevisionStore.GetOrInitForUpdate(ctx, zero)
	require.NoError(t, err)
	assert.Equal(t, "instance_events", got.Name)
	assert.Equal(t, int64(0), got.CurrentRevision)
}

func TestGetOrInitForUpdate_ExistingRow(t *testing.T) {
	cleanGlobalRevisions(t)
	ctx := context.Background()

	zero := &schema.GlobalRevision{
		Name:            "instance_events",
		CurrentRevision: 0,
		UpdateTime:      0,
	}
	_, err := gTestGlobalRevisionStore.GetOrInitForUpdate(ctx, zero)
	require.NoError(t, err)

	err = gTestGlobalRevisionStore.IncrRevision(ctx, "instance_events", nowMs())
	require.NoError(t, err)

	// 再次调用，冲突路径下 DO UPDATE 空操作加锁 + RETURNING 返回真实值
	got, err := gTestGlobalRevisionStore.GetOrInitForUpdate(ctx, zero)
	require.NoError(t, err)
	assert.Equal(t, "instance_events", got.Name)
	assert.Equal(t, int64(1), got.CurrentRevision)
}

func TestIncrRevision(t *testing.T) {
	cleanGlobalRevisions(t)
	ctx := context.Background()

	zero := &schema.GlobalRevision{
		Name:            "instance_events",
		CurrentRevision: 0,
		UpdateTime:      0,
	}
	_, err := gTestGlobalRevisionStore.GetOrInitForUpdate(ctx, zero)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err = gTestGlobalRevisionStore.IncrRevision(ctx, "instance_events", nowMs())
		require.NoError(t, err)
	}

	var rev schema.GlobalRevision
	err = gTestDB.Where("name = ?", "instance_events").First(&rev).Error
	require.NoError(t, err)
	assert.Equal(t, int64(5), rev.CurrentRevision)
}

func TestGetOrInitForUpdate_Concurrent(t *testing.T) {
	cleanGlobalRevisions(t)

	const N = 10
	errs := make([]error, N)
	results := make([]*schema.GlobalRevision, N)

	var wg sync.WaitGroup
	gate := make(chan struct{}) // 所有协程同时起跑

	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate

			tx := gTestDB.Begin()
			ctx := repository.WithTTx(context.Background(), tx)

			rev, err := gTestGlobalRevisionStore.GetOrInitForUpdate(
				ctx,
				&schema.GlobalRevision{Name: "concurrent_key", CurrentRevision: 0, UpdateTime: 0},
			)
			if err != nil {
				tx.Rollback()
				errs[idx] = err
				return
			}
			results[idx] = rev

			if err := tx.Commit().Error; err != nil {
				errs[idx] = err
			}
		}(i)
	}

	close(gate)
	wg.Wait()

	// 1. 所有协程不应报错
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d", i)
	}

	// 2. DB 中只有一行
	var count int64
	gTestDB.Model(&schema.GlobalRevision{}).Where("name = ?", "concurrent_key").Count(&count)
	assert.Equal(t, int64(1), count, "应该只有 1 行记录")

	// 3. 所有协程都应拿到有效返回（INSERT 或冲突路径均通过 RETURNING 返回）
	for i, r := range results {
		if assert.NotNil(t, r, "goroutine %d result", i) {
			assert.Equal(t, "concurrent_key", r.Name, "goroutine %d name", i)
		}
	}
}

func TestGetOrInitForUpdate_ConcurrentWithIncr(t *testing.T) {
	cleanGlobalRevisions(t)
	ctx := context.Background()

	// 先初始化行
	zero := &schema.GlobalRevision{Name: "incr_key", CurrentRevision: 0, UpdateTime: 0}
	_, err := gTestGlobalRevisionStore.GetOrInitForUpdate(ctx, zero)
	require.NoError(t, err)

	// N 个协程在各自事务中做 GetOrInitForUpdate + IncrRevision
	const N = 10
	errs := make([]error, N)
	gate := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate

			tx := gTestDB.Begin()
			txCtx := repository.WithTTx(context.Background(), tx)

			_, err := gTestGlobalRevisionStore.GetOrInitForUpdate(
				txCtx,
				&schema.GlobalRevision{Name: "incr_key", CurrentRevision: 0, UpdateTime: 0},
			)
			if err != nil {
				tx.Rollback()
				errs[idx] = err
				return
			}

			if err := gTestGlobalRevisionStore.IncrRevision(txCtx, "incr_key", nowMs()); err != nil {
				tx.Rollback()
				errs[idx] = err
				return
			}

			if err := tx.Commit().Error; err != nil {
				errs[idx] = err
			}
		}(i)
	}

	close(gate)
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d", i)
	}

	// 初始 0 + N 次 IncrRevision = N
	var rev schema.GlobalRevision
	err = gTestDB.Where("name = ?", "incr_key").First(&rev).Error
	require.NoError(t, err)
	assert.Equal(t, int64(N), rev.CurrentRevision, "revision 应该等于并发次数")
}
