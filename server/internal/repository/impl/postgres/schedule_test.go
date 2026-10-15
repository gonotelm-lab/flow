package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 这个文件覆盖整个特性最核心的几条保证：
//   - 同一次 fire 绝不被两个实例同时认领（分布式不重复投递的落点）
//   - 租约过期才允许接管，且接管后 FireId 不变（at-least-once 的去重依据）
//   - 部分唯一索引把「同一 schedule 最多一条重试中死信」钉死在数据库层
//   - 幂等键在 namespace 内唯一，且 NULL 不参与唯一性
//   - 硬删级联清理失败记录与死信
// ---------------------------------------------------------------------------

func cleanSchedules(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM schedule_attempt_failures")
	gTestDB.Exec("DELETE FROM schedule_dead_letters")
	gTestDB.Exec("DELETE FROM schedules")
	gTestDB.Exec("DELETE FROM admin_audit_logs")
}

// nowMs 复用 instance_test.go 里的同名 helper（同一 package）。

func strPtr(s string) *string {
	return &s
}

func newTestSchedule(namespace, scheduleType, state string, nextFireAt int64) *schema.Schedule {
	now := nowMs()
	id, _ := uuid.NewV7()

	return &schema.Schedule{
		Id:           id,
		Namespace:    namespace,
		Name:         "test-schedule",
		ScheduleType: scheduleType,
		CronExpr:     "* * * * *",
		Timezone:     "UTC",
		State:        state,
		CallbackUrl:  "http://example.com/hook",
		ContentType:  "application/json",
		Payload:      []byte(`{"a":1}`),
		Shard:        10,
		NextFireAt:   nextFireAt,
		CreateTime:   now,
		UpdateTime:   now,
	}
}

func claimParams(now int64, batch int) *store.ScheduleClaimParams {
	return &store.ScheduleClaimParams{
		ShardStart: 0,
		ShardEnd:   100,
		Now:        now,
		BatchSize:  batch,
		LeaseOwner: "instance-1",
		LeaseUntil: now + 30_000,
	}
}

func TestScheduleStore_CreateAndGet(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()

	sched := newTestSchedule("ns-create", schema.ScheduleTypeCron, schema.ScheduleStateActive, nowMs())
	created, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)
	assert.Equal(t, sched.Id, created.Id)

	got, err := gTestScheduleStore.Get(ctx, sched.Id)
	require.NoError(t, err)
	assert.Equal(t, "ns-create", got.Namespace)
	assert.Equal(t, schema.ScheduleTypeCron, got.ScheduleType)
	assert.Nil(t, got.FireState, "新注册的 schedule 槽位必须是空闲的")
	assert.False(t, got.Busy())
}

func TestScheduleStore_Get_NotFound(t *testing.T) {
	cleanSchedules(t)

	_, err := gTestScheduleStore.Get(context.Background(), uuid.New())
	require.Error(t, err)
	assert.True(t, errors.Is(err, pkgerr.NoRecord))
}

// 幂等注册依赖部分唯一索引 (namespace, idempotency_key) WHERE idempotency_key IS NOT NULL。
func TestScheduleStore_IdempotencyKey(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()

	first := newTestSchedule("ns-idem", schema.ScheduleTypeCron, schema.ScheduleStateActive, nowMs())
	first.IdempotencyKey = strPtr("key-1")
	_, err := gTestScheduleStore.Create(ctx, first)
	require.NoError(t, err)

	t.Run("同 namespace 重复键被唯一索引拦下", func(t *testing.T) {
		dup := newTestSchedule("ns-idem", schema.ScheduleTypeCron, schema.ScheduleStateActive, nowMs())
		dup.IdempotencyKey = strPtr("key-1")
		_, err := gTestScheduleStore.Create(ctx, dup)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pkgerr.DuplicatedResource))
	})

	t.Run("可以按键取回已有记录", func(t *testing.T) {
		found, err := gTestScheduleStore.GetByIdempotencyKey(ctx, "ns-idem", "key-1")
		require.NoError(t, err)
		assert.Equal(t, first.Id, found.Id)
	})

	t.Run("不同 namespace 可以用同一个键", func(t *testing.T) {
		other := newTestSchedule("ns-idem-2", schema.ScheduleTypeCron, schema.ScheduleStateActive, nowMs())
		other.IdempotencyKey = strPtr("key-1")
		_, err := gTestScheduleStore.Create(ctx, other)
		require.NoError(t, err)
	})

	t.Run("不传键时不受唯一性约束", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			noKey := newTestSchedule("ns-idem", schema.ScheduleTypeCron, schema.ScheduleStateActive, nowMs())
			_, err := gTestScheduleStore.Create(ctx, noKey)
			require.NoError(t, err)
		}
	})
}

func TestScheduleStore_ClaimDue_FiltersByShardStateAndTime(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	due := newTestSchedule("ns-claim", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
	future := newTestSchedule("ns-claim", schema.ScheduleTypeCron, schema.ScheduleStateActive, now+60_000)
	paused := newTestSchedule("ns-claim", schema.ScheduleTypeCron, schema.ScheduleStatePaused, now-1000)
	otherShard := newTestSchedule("ns-claim", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
	otherShard.Shard = 999

	for _, s := range []*schema.Schedule{due, future, paused, otherShard} {
		_, err := gTestScheduleStore.Create(ctx, s)
		require.NoError(t, err)
	}

	claimed, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 100))
	require.NoError(t, err)
	require.Len(t, claimed, 1, "只应认领本分片范围内、ACTIVE、空闲且已到期的行")
	assert.Equal(t, due.Id, claimed[0].Id)
	require.NotNil(t, claimed[0].FireId)
	assert.Equal(t, schema.FireStatePending, *claimed[0].FireState)
	assert.Equal(t, due.NextFireAt, claimed[0].FireScheduledTime,
		"fire_scheduled_time 必须记录被消费掉的那个 occurrence")
	assert.Equal(t, 1, claimed[0].AttemptNo)

	// 已认领的行不该被再次认领
	again, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 100))
	require.NoError(t, err)
	assert.Empty(t, again)
}

// 这是「分布式下绝不重复投递」的正面证据：
// 多个实例同时扫同一分片，每一行只会被其中一个认领。
func TestScheduleStore_ClaimDue_ConcurrentInstancesNeverDoubleClaim(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	const rows = 40
	for i := 0; i < rows; i++ {
		s := newTestSchedule("ns-concurrent", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
		_, err := gTestScheduleStore.Create(ctx, s)
		require.NoError(t, err)
	}

	const racers = 6

	var (
		mu      sync.Mutex
		claimed = map[uuid.UUID]int{}
	)
	var wg sync.WaitGroup

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(owner int) {
			defer wg.Done()

			p := claimParams(now, rows*2)
			p.LeaseOwner = fmt.Sprintf("instance-%d", owner)

			got, err := gTestScheduleStore.ClaimDue(context.Background(), p)
			if err != nil {
				return
			}

			mu.Lock()
			defer mu.Unlock()
			for _, s := range got {
				claimed[s.Id]++
			}
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, claimed, rows, "所有到期行都应被认领")
	for id, times := range claimed {
		assert.Equal(t, 1, times, "schedule %s 被认领了多次 —— 重复投递", id)
	}
}

func TestScheduleStore_ClaimRetry_RespectsLeaseAndPreservesState(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	fireId, _ := uuid.NewV7()
	delivering := schema.FireStateDelivering

	sched := newTestSchedule("ns-retry", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	sched.FireId = &fireId
	sched.FireState = &delivering
	sched.AttemptNo = 1
	sched.NextAttemptAt = now - 1000
	sched.LeaseUntil = now + 30_000 // 租约仍有效
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	t.Run("租约未过期时不可被接管", func(t *testing.T) {
		got, err := gTestScheduleStore.ClaimRetry(ctx, claimParams(now, 10))
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	// 模拟持有租约的实例崩溃
	require.NoError(t, gTestDB.Exec(
		"UPDATE schedules SET lease_until = ? WHERE id = ?", now-1000, sched.Id).Error)

	t.Run("租约过期后可接管且 FireId 不变", func(t *testing.T) {
		got, err := gTestScheduleStore.ClaimRetry(ctx, claimParams(now, 10))
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].FireId)
		assert.Equal(t, fireId, *got[0].FireId,
			"接管必须沿用原 FireId，否则接收方无法识别为同一次 fire 的重投")
	})
}

// PENDING 是「已认领但还没投递」；再次被认领时必须变成 DELIVERING，
// 调度器才不会拿迟到容差把它跳过（那会静默丢一次触发）。
func TestScheduleStore_ClaimRetry_PendingBecomesDelivering(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	fireId, _ := uuid.NewV7()
	pending := schema.FireStatePending

	sched := newTestSchedule("ns-pending", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	sched.FireId = &fireId
	sched.FireState = &pending
	sched.NextAttemptAt = now - 1000
	sched.LeaseUntil = now - 1
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	got, err := gTestScheduleStore.ClaimRetry(ctx, claimParams(now, 10))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, schema.FireStateDelivering, *got[0].FireState)

	fromDB, err := gTestScheduleStore.Get(ctx, sched.Id)
	require.NoError(t, err)
	assert.Equal(t, schema.FireStateDelivering, *fromDB.FireState)
}

// 死信阶段必须保持 DEAD_LETTER：调度器靠它决定用慢退避节奏而不是快退避。
func TestScheduleStore_ClaimRetry_PreservesDeadLetterPhase(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	fireId, _ := uuid.NewV7()
	deadLetter := schema.FireStateDeadLetter

	sched := newTestSchedule("ns-dl-phase", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	sched.FireId = &fireId
	sched.FireState = &deadLetter
	sched.NextAttemptAt = now - 1000
	sched.LeaseUntil = now - 1
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	got, err := gTestScheduleStore.ClaimRetry(ctx, claimParams(now, 10))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, schema.FireStateDeadLetter, *got[0].FireState)
}

func TestScheduleStore_ApplyOutcome_SuccessReleasesSlot(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	due := newTestSchedule("ns-outcome", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
	_, err := gTestScheduleStore.Create(ctx, due)
	require.NoError(t, err)

	claimed, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 1))
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	next := now + 60_000
	ok, err := gTestScheduleStore.ApplyOutcome(ctx, due.Id, *claimed[0].FireId, &store.AttemptOutcome{
		NextFireState: nil,
		NextFireAt:    next,
		Success:       true,
		Now:           now,
	})
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestScheduleStore.Get(ctx, due.Id)
	require.NoError(t, err)
	assert.Nil(t, got.FireState, "成功后必须释放槽位")
	assert.Nil(t, got.FireId)
	assert.Equal(t, 0, got.AttemptNo)
	assert.Equal(t, next, got.NextFireAt, "必须推进 next_fire_at，否则同一个 occurrence 会被重复触发")
	assert.Equal(t, now, got.LastSuccessAt)
	assert.Equal(t, 0, got.ConsecutiveFailures)
	assert.False(t, got.Busy())

	// 释放后且未到期，不应被再次认领
	again, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 10))
	require.NoError(t, err)
	assert.Empty(t, again)
}

func TestScheduleStore_ApplyOutcome_FailureKeepsSlotAndCounts(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	due := newTestSchedule("ns-fail", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
	_, err := gTestScheduleStore.Create(ctx, due)
	require.NoError(t, err)

	claimed, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 1))
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	delivering := schema.FireStateDelivering
	nextAt := now + 1000
	_, err = gTestScheduleStore.ApplyOutcome(ctx, due.Id, *claimed[0].FireId, &store.AttemptOutcome{
		NextFireState: &delivering,
		NextAttemptAt: nextAt,
		AttemptNo:     2,
		LastError:     "connection refused",
		Now:           now,
	})
	require.NoError(t, err)

	got, err := gTestScheduleStore.Get(ctx, due.Id)
	require.NoError(t, err)
	require.NotNil(t, got.FireState)
	assert.Equal(t, schema.FireStateDelivering, *got.FireState, "失败后槽位仍被占用——这正是重叠跳过的依据")
	assert.Equal(t, 2, got.AttemptNo)
	assert.Equal(t, nextAt, got.NextAttemptAt)
	assert.Equal(t, 1, got.ConsecutiveFailures)
	assert.Equal(t, int64(1), got.TotalFailures)
	assert.Equal(t, "connection refused", got.LastError)
	assert.True(t, got.Busy())
}

func TestScheduleStore_SkipDue_ReleasesAndCounts(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	due := newTestSchedule("ns-skip", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-3600_000)
	_, err := gTestScheduleStore.Create(ctx, due)
	require.NoError(t, err)

	claimed, err := gTestScheduleStore.ClaimDue(ctx, claimParams(now, 1))
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	next := now + 60_000
	ok, err := gTestScheduleStore.SkipDue(ctx, due.Id, *claimed[0].FireId, next, 37, now)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestScheduleStore.Get(ctx, due.Id)
	require.NoError(t, err)
	assert.Nil(t, got.FireState)
	assert.Equal(t, next, got.NextFireAt)
	assert.Equal(t, int64(37), got.SkippedCount)
	assert.False(t, got.Busy())
}

func TestScheduleStore_ClaimByID_RejectsBusySlot(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	sched := newTestSchedule("ns-byid", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	fireId, _ := uuid.NewV7()
	ok, err := gTestScheduleStore.ClaimByID(ctx, sched.Id, &store.ClaimByIDParams{
		FireId:            fireId,
		FireState:         schema.FireStatePending,
		FireScheduledTime: 0,
		Now:               now,
	})
	require.NoError(t, err)
	require.True(t, ok)

	// 槽位已被占用，手动触发必须被拒绝
	another, _ := uuid.NewV7()
	ok, err = gTestScheduleStore.ClaimByID(ctx, sched.Id, &store.ClaimByIDParams{
		FireId:    another,
		FireState: schema.FireStatePending,
		Now:       now,
	})
	require.NoError(t, err)
	assert.False(t, ok, "同一 schedule 同时最多一个 fire，TriggerNow 必须被拒绝")
}

func TestScheduleStore_DeleteCascade(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	due := newTestSchedule("ns-del", schema.ScheduleTypeCron, schema.ScheduleStateActive, now-1000)
	_, err := gTestScheduleStore.Create(ctx, due)
	require.NoError(t, err)

	fireId, _ := uuid.NewV7()

	require.NoError(t, gTestScheduleAttemptFailureStore.Create(ctx, &schema.ScheduleAttemptFailure{
		ScheduleId: due.Id,
		Namespace:  due.Namespace,
		FireId:     fireId,
		AttemptNo:  1,
		Phase:      schema.AttemptPhaseFast,
		CreateTime: now,
	}))

	dlId, _ := uuid.NewV7()
	_, err = gTestScheduleDeadLetterStore.Create(ctx, &schema.ScheduleDeadLetter{
		Id:         dlId,
		ScheduleId: due.Id,
		Namespace:  due.Namespace,
		FireId:     fireId,
		State:      schema.DeadLetterStateExhausted,
		CreateTime: now,
		UpdateTime: now,
	})
	require.NoError(t, err)

	require.NoError(t, gTestScheduleStore.DeleteCascade(ctx, due.Id))

	_, err = gTestScheduleStore.Get(ctx, due.Id)
	assert.True(t, errors.Is(err, pkgerr.NoRecord), "schedule 必须被硬删")

	_, err = gTestScheduleDeadLetterStore.Get(ctx, dlId)
	assert.True(t, errors.Is(err, pkgerr.NoRecord), "死信必须被级联删除")

	failures, err := gTestScheduleAttemptFailureStore.ListBySchedule(ctx, due.Id, 10)
	require.NoError(t, err)
	assert.Empty(t, failures, "失败记录必须被级联删除")
}

func TestScheduleStore_SetState_WithNextFireAt(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	sched := newTestSchedule("ns-state", schema.ScheduleTypeCron, schema.ScheduleStateActive, now+60_000)
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	next := now + 120_000
	ok, err := gTestScheduleStore.SetState(
		ctx, sched.Id, schema.ScheduleStateActive, schema.ScheduleStatePaused, next, now)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestScheduleStore.Get(ctx, sched.Id)
	require.NoError(t, err)
	assert.Equal(t, schema.ScheduleStatePaused, got.State)
	assert.Equal(t, next, got.NextFireAt, "暂停时也允许同时重算 next_fire_at")

	// 状态不匹配时条件更新应当不生效
	ok, err = gTestScheduleStore.SetState(
		ctx, sched.Id, schema.ScheduleStateActive, schema.ScheduleStatePaused, 0, now)
	require.NoError(t, err)
	assert.False(t, ok)
}

// Now() 是集群的统一时钟源：到期判定与「fire_at 必须晚于当前时间」都以它为准。
func TestScheduleStore_Now_UsesDatabaseClock(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()

	dbNow, err := gTestScheduleStore.Now(ctx)
	require.NoError(t, err)

	require.InDelta(t, float64(nowMs()), float64(dbNow), 5000,
		"数据库时间应与本地时间接近（同一台机器上跑测试）；差太多说明取错了时钟")
}

func TestScheduleDeadLetterStore_OnlyOneRetryingPerSchedule(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	sched := newTestSchedule("ns-dl", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	fireId, _ := uuid.NewV7()

	firstId, _ := uuid.NewV7()
	_, err = gTestScheduleDeadLetterStore.Create(ctx, &schema.ScheduleDeadLetter{
		Id: firstId, ScheduleId: sched.Id, Namespace: sched.Namespace, FireId: fireId,
		State: schema.DeadLetterStateRetrying, CreateTime: now, UpdateTime: now,
	})
	require.NoError(t, err)

	t.Run("同一 schedule 不能有第二条 RETRYING", func(t *testing.T) {
		secondId, _ := uuid.NewV7()
		_, err := gTestScheduleDeadLetterStore.Create(ctx, &schema.ScheduleDeadLetter{
			Id: secondId, ScheduleId: sched.Id, Namespace: sched.Namespace, FireId: fireId,
			State: schema.DeadLetterStateRetrying, CreateTime: now, UpdateTime: now,
		})
		require.Error(t, err, "部分唯一索引应当拦下第二条重试中的死信")
	})

	t.Run("EXHAUSTED 可以累积多条", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			id, _ := uuid.NewV7()
			_, err := gTestScheduleDeadLetterStore.Create(ctx, &schema.ScheduleDeadLetter{
				Id: id, ScheduleId: sched.Id, Namespace: sched.Namespace, FireId: fireId,
				State: schema.DeadLetterStateExhausted, CreateTime: now, UpdateTime: now,
			})
			require.NoError(t, err, "慢重试耗尽后的死信不再阻塞新触发，理应能累积")
		}
	})

	t.Run("GetRetryingBySchedule 只取重试中的那条", func(t *testing.T) {
		got, err := gTestScheduleDeadLetterStore.GetRetryingBySchedule(ctx, sched.Id)
		require.NoError(t, err)
		assert.Equal(t, firstId, got.Id)
		assert.Equal(t, schema.DeadLetterStateRetrying, got.State)
	})
}

func TestScheduleDeadLetterStore_SetState(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	sched := newTestSchedule("ns-dl-state", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	fireId, _ := uuid.NewV7()
	dlId, _ := uuid.NewV7()
	_, err = gTestScheduleDeadLetterStore.Create(ctx, &schema.ScheduleDeadLetter{
		Id: dlId, ScheduleId: sched.Id, Namespace: sched.Namespace, FireId: fireId,
		State: schema.DeadLetterStateExhausted, CreateTime: now, UpdateTime: now,
	})
	require.NoError(t, err)

	ok, err := gTestScheduleDeadLetterStore.SetState(
		ctx, dlId, schema.DeadLetterStateExhausted, schema.DeadLetterStateReplayed, now)
	require.NoError(t, err)
	require.True(t, ok)

	// 已经处理过的死信不能再次处理
	ok, err = gTestScheduleDeadLetterStore.SetState(
		ctx, dlId, schema.DeadLetterStateExhausted, schema.DeadLetterStateDiscarded, now)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestScheduleAttemptFailureStore_ListBySchedule(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	sched := newTestSchedule("ns-attempts", schema.ScheduleTypeCron, schema.ScheduleStateActive, now)
	_, err := gTestScheduleStore.Create(ctx, sched)
	require.NoError(t, err)

	fireId, _ := uuid.NewV7()
	for i := 1; i <= 3; i++ {
		require.NoError(t, gTestScheduleAttemptFailureStore.Create(ctx, &schema.ScheduleAttemptFailure{
			ScheduleId: sched.Id,
			Namespace:  sched.Namespace,
			FireId:     fireId,
			AttemptNo:  i,
			Phase:      schema.AttemptPhaseFast,
			HttpStatus: 500,
			DurationMs: 12,
			Error:      "boom",
			CreateTime: now + int64(i),
		}))
	}

	got, err := gTestScheduleAttemptFailureStore.ListBySchedule(ctx, sched.Id, 10)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, 3, got[0].AttemptNo, "按 create_time 倒序")
}

// 回归：不带 id 连续写入。
//
// 起因是一个真实事故：admin_audit_logs.id 有 UUID DEFAULT uuidv7()，但 GORM 会把 Go 的
// 零值 UUID 显式写进 INSERT，数据库默认值不生效 —— 第一条以全零主键成功，之后每条都撞
// 主键。审计是异步写的，失败只出现在日志里，接口照样 200，属于静默丢数据。
func TestAdminAuditLogStore_FillsMissingID(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	for i := 0; i < 3; i++ {
		require.NoError(t, gTestAdminAuditLogStore.Create(ctx, &schema.AdminAuditLog{
			// 故意不设 Id
			Action:     schema.AuditActionCreateSchedule,
			TargetType: schema.AuditTargetSchedule,
			Namespace:  "ns-audit-id",
			Source:     schema.AuditSourceAdminHTTP,
			Result:     schema.AuditResultOk,
			CreateTime: now + int64(i),
		}), "第 %d 条写入失败——大概率是主键没被兜底生成", i+1)
	}

	_, total, err := gTestAdminAuditLogStore.List(ctx, &store.AdminAuditLogListParams{
		Namespace: "ns-audit-id",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total, "不带 id 的三条必须都落库，不能互相撞主键")
}

func TestAdminAuditLogStore_FillsMissingIDInBatch(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	logs := []*schema.AdminAuditLog{
		{Action: schema.AuditActionPauseSchedule, TargetType: schema.AuditTargetSchedule,
			Namespace: "ns-audit-batch", Source: schema.AuditSourceAdminHTTP,
			Result: schema.AuditResultOk, CreateTime: now},
		{Action: schema.AuditActionResumeSchedule, TargetType: schema.AuditTargetSchedule,
			Namespace: "ns-audit-batch", Source: schema.AuditSourceAdminHTTP,
			Result: schema.AuditResultOk, CreateTime: now + 1},
	}
	require.NoError(t, gTestAdminAuditLogStore.CreateBatch(ctx, logs))

	for _, l := range logs {
		assert.NotEqual(t, uuid.Nil, l.Id, "批量写入也要给主键兜底")
	}

	_, total, err := gTestAdminAuditLogStore.List(ctx, &store.AdminAuditLogListParams{
		Namespace: "ns-audit-batch",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
}

func TestAdminAuditLogStore_CreateBatchAndList(t *testing.T) {
	cleanSchedules(t)
	ctx := context.Background()
	now := nowMs()

	logs := make([]*schema.AdminAuditLog, 0, 3)
	for i := 0; i < 3; i++ {
		id, _ := uuid.NewV7()
		logs = append(logs, &schema.AdminAuditLog{
			Id:         id,
			Action:     schema.AuditActionDeleteSchedule,
			TargetType: schema.AuditTargetSchedule,
			TargetId:   "sched-1",
			Namespace:  "ns-audit",
			Source:     schema.AuditSourceAdminHTTP,
			Result:     schema.AuditResultOk,
			CreateTime: now + int64(i),
		})
	}

	require.NoError(t, gTestAdminAuditLogStore.CreateBatch(ctx, logs))

	got, total, err := gTestAdminAuditLogStore.List(ctx, &store.AdminAuditLogListParams{
		TargetType: schema.AuditTargetSchedule,
		TargetId:   "sched-1",
		Limit:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, got, 3)

	// 过滤条件生效
	_, total, err = gTestAdminAuditLogStore.List(ctx, &store.AdminAuditLogListParams{
		Namespace: "no-such-ns",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
}
