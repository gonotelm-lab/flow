package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/api/signature"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/impl"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/internal/sharding"
	"github.com/gonotelm-lab/flow/server/migration"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/gonotelm-lab/flow/server/pkg/sql/testsuite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 端到端：真库 + 真 httptest 接收方，验证
//   成功 / 重试 / 死信 / 迟到跳过 / 签名 / FireId 稳定
// 这几条语义确实是「跑起来之后」成立的，而不是只写在文档里。

const testNamespace = "ns-scheduler-e2e"
const testApiKey = "sk-e2e-secret"

// ---------------------------------------------------------------------------
// 测试环境
// ---------------------------------------------------------------------------

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if os.Getenv("FLOW_DB_HOST") == "" && os.Getenv("FLOW_DB_NAME") == "" {
		t.Skip("FLOW_DB_* 未配置，跳过需要数据库的集成测试")
	}

	testdb, err := testsuite.NewTestGormDBFromEnv("pgsql")
	require.NoError(t, err)

	fsys, ok := migration.FSFor(sql.DriverPgsql)
	require.True(t, ok, "pgsql migration fs not registered")
	require.NoError(t, testdb.Setup(fsys))

	t.Cleanup(func() { _ = testdb.Cleanup() })

	return testdb.GetDB()
}

func newTestStore(t *testing.T, db *gorm.DB) *repository.Store {
	t.Helper()

	scheduleStore, err := impl.NewScheduleStore(sql.DriverPgsql, db)
	require.NoError(t, err)
	deadLetterStore, err := impl.NewScheduleDeadLetterStore(sql.DriverPgsql, db)
	require.NoError(t, err)
	attemptStore, err := impl.NewScheduleAttemptFailureStore(sql.DriverPgsql, db)
	require.NoError(t, err)
	namespaceStore, err := impl.NewNamespaceStore(sql.DriverPgsql, db)
	require.NoError(t, err)

	return &repository.Store{
		Schedule:               scheduleStore,
		ScheduleDeadLetter:     deadLetterStore,
		ScheduleAttemptFailure: attemptStore,
		Namespace:              namespaceStore,
	}
}

func fastConfig() *config.SchedulerConfig {
	cfg := config.DefaultSchedulerConfig()
	cfg.TickInterval = 10 * time.Millisecond
	cfg.HTTPTimeout = 2 * time.Second
	cfg.LeaseDuration = 30 * time.Second
	cfg.LatenessTolerance = 100 * time.Millisecond
	cfg.ScanBatch = 50
	cfg.DeliverConcurrency = 8
	cfg.RetryJitterRatio = 0 // 测试里去掉抖动，保证时序可断言
	cfg.FastRetryIntervalsRaw = "5ms,5ms"
	cfg.DeadLetterIntervalsRaw = "5ms,5ms"
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return cfg
}

func fullShard() *sharding.Shard {
	return &sharding.Shard{Id: 1, Start: 0, End: sharding.ShardSize - 1}
}

func seedNamespace(t *testing.T, st *repository.Store) {
	t.Helper()
	ctx := context.Background()

	_, err := st.Namespace.Create(ctx, &schema.Namespace{
		Name:       testNamespace,
		ApiKey:     testApiKey,
		CreateTime: time.Now().UnixMilli(),
		UpdateTime: time.Now().UnixMilli(),
	})
	require.NoError(t, err)
}

func seedSchedule(
	t *testing.T,
	st *repository.Store,
	scheduleType string,
	fireAt int64,
	nextFireAt int64,
) *schema.Schedule {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	id, err := uuid.NewV7()
	require.NoError(t, err)

	sched := &schema.Schedule{
		Id:           id,
		Namespace:    testNamespace,
		Name:         "e2e",
		ScheduleType: scheduleType,
		State:        schema.ScheduleStateActive,
		CallbackUrl:  "http://127.0.0.1/placeholder",
		ContentType:  "application/json",
		Payload:      []byte(`{"hello":"world"}`),
		Shard:        ShardOf(id),
		NextFireAt:   nextFireAt,
		CreateTime:   now,
		UpdateTime:   now,
	}
	if scheduleType == schema.ScheduleTypeCron {
		sched.CronExpr = "* * * * *"
		sched.Timezone = "UTC"
	} else {
		sched.FireAt = fireAt
	}

	created, err := st.Schedule.Create(ctx, sched)
	require.NoError(t, err)

	return created
}

// ---------------------------------------------------------------------------
// httptest 接收方
// ---------------------------------------------------------------------------

type capturedRequest struct {
	body   []byte
	header http.Header
	method string
	url    *url.URL
}

type receiver struct {
	mu       sync.Mutex
	requests []capturedRequest

	// failFirst 表示前 N 次调用返回 500
	failFirst int
	// status 为 0 时按 failFirst 规则；否则固定返回该状态码
	status int
	// block 非 nil 时每次请求会等待它（用来模拟投递耗时）
	block chan struct{}
}

func (r *receiver) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body := make([]byte, req.ContentLength)
		if req.ContentLength > 0 {
			_, _ = req.Body.Read(body)
		}

		r.mu.Lock()
		r.requests = append(r.requests, capturedRequest{
			body:   body,
			header: req.Header.Clone(),
			method: req.Method,
			url:    req.URL,
		})
		n := len(r.requests)
		failFirst := r.failFirst
		status := r.status
		block := r.block
		r.mu.Unlock()

		if block != nil {
			<-block
		}

		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if n <= failFirst {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *receiver) at(i int) capturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[i]
}

// ---------------------------------------------------------------------------
// 驱动
// ---------------------------------------------------------------------------

func newDispatcher(t *testing.T, st *repository.Store) *Dispatcher {
	t.Helper()
	return NewDispatcher(st, fastConfig(), fullShard, "instance-test")
}

// pump 反复 tick 直到条件满足或超时。
//
// 直接调 tick 而不是跑 Run 的 ticker，是为了让测试确定：不等真实定时器，
// 也不会因为 tick 间隔与重试间隔竞争而闪断。
func pump(t *testing.T, d *Dispatcher, until func() bool, timeout time.Duration) {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		d.tick(ctx)
		d.wg.Wait() // 等本轮所有投递结束
		if until() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("条件在 %s 内未满足", timeout)
}

func getSchedule(t *testing.T, st *repository.Store, id uuid.UUID) *schema.Schedule {
	t.Helper()
	got, err := st.Schedule.Get(context.Background(), id)
	require.NoError(t, err)
	return got
}

// ---------------------------------------------------------------------------
// 用例
// ---------------------------------------------------------------------------

func TestDispatcher_SuccessPath(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	recv := &receiver{}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	sched := seedSchedule(t, st, schema.ScheduleTypeOneShot,
		time.Now().UnixMilli()-1000, time.Now().UnixMilli()-1000)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	d := newDispatcher(t, st)
	pump(t, d, func() bool { return recv.count() == 1 }, 5*time.Second)

	// body 必须是 payload 原样
	assert.Equal(t, `{"hello":"world"}`, string(recv.at(0).body))
	assert.Equal(t, "application/json", recv.at(0).header.Get("Content-Type"))

	// 元数据在 header 里
	assert.Equal(t, sched.Id.String(), recv.at(0).header.Get(HeaderScheduleId))
	assert.Equal(t, testNamespace, recv.at(0).header.Get(HeaderNamespace))
	fireId := recv.at(0).header.Get(HeaderFireId)
	require.NotEmpty(t, fireId)

	// 签名必须能被接收方按文档复算出来——用的就是客户端 SDK 里那份实现，
	// 所以这条断言同时锁住了「服务端签的」与「SDK 验的」是同一条规则。
	sigErr := signature.Verify(testApiKey, signature.Input{
		Method: recv.at(0).method,
		URL:    recv.at(0).url,
		Header: recv.at(0).header,
		Body:   recv.at(0).body,
	}, recv.at(0).header.Get(HeaderSignature))
	assert.NoError(t, sigErr, "接收方必须能用 namespace api_key 复算出签名")

	// 时间戳必须可解析，接收方才有防重放的基础。
	_, err := signature.Timestamp(recv.at(0).header)
	assert.NoError(t, err)

	// 一次性任务成功后进入终态，槽位释放
	pump(t, d, func() bool {
		got := getSchedule(t, st, sched.Id)
		return got.State == schema.ScheduleStateFinished
	}, 5*time.Second)

	got := getSchedule(t, st, sched.Id)
	assert.Nil(t, got.FireState, "成功后必须释放 fire 槽位")
	assert.NotZero(t, got.LastSuccessAt)
	assert.Equal(t, 0, got.ConsecutiveFailures)

	// 成功不落失败记录
	failures, err := st.ScheduleAttemptFailure.ListBySchedule(context.Background(), sched.Id, 10)
	require.NoError(t, err)
	assert.Empty(t, failures, "成功不应该产生失败记录")
}

func TestDispatcher_RetriesThenSucceedsWithStableFireId(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	recv := &receiver{failFirst: 2}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	sched := seedSchedule(t, st, schema.ScheduleTypeOneShot,
		time.Now().UnixMilli()-1000, time.Now().UnixMilli()-1000)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	d := newDispatcher(t, st)
	pump(t, d, func() bool { return recv.count() >= 3 }, 10*time.Second)

	assert.Equal(t, 3, recv.count(), "应当恰好尝试 3 次：2 次失败 + 1 次成功")

	// FireId 在重试之间必须保持不变 —— 这是接收方去重的唯一依据
	first := recv.at(0).header.Get(HeaderFireId)
	for i := 1; i < recv.count(); i++ {
		assert.Equal(t, first, recv.at(i).header.Get(HeaderFireId),
			"第 %d 次尝试的 FireId 变了，接收方将无法识别为同一次 fire", i+1)
	}
	// ScheduledTime 同样应当稳定
	assert.Equal(t,
		recv.at(0).header.Get(HeaderScheduledTime),
		recv.at(2).header.Get(HeaderScheduledTime))

	pump(t, d, func() bool {
		return getSchedule(t, st, sched.Id).State == schema.ScheduleStateFinished
	}, 5*time.Second)

	// 两次失败各落一条记录
	failures, err := st.ScheduleAttemptFailure.ListBySchedule(context.Background(), sched.Id, 10)
	require.NoError(t, err)
	assert.Len(t, failures, 2)
	assert.Equal(t, 500, failures[0].HttpStatus)

	// 没有产生死信
	_, err = st.ScheduleDeadLetter.GetRetryingBySchedule(context.Background(), sched.Id)
	require.Error(t, err)
}

func TestDispatcher_ExhaustsRetriesIntoDeadLetter(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	recv := &receiver{status: http.StatusInternalServerError}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	sched := seedSchedule(t, st, schema.ScheduleTypeOneShot,
		time.Now().UnixMilli()-1000, time.Now().UnixMilli()-1000)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	d := newDispatcher(t, st)

	// 快速阶段：首次 + 2 次重试 = 3 次
	expected := len(fastConfig().FastRetryIntervals) + 1
	pump(t, d, func() bool { return recv.count() >= expected }, 10*time.Second)

	ctx := context.Background()

	// 进入死信，处于 RETRYING
	pump(t, d, func() bool {
		_, err := st.ScheduleDeadLetter.GetRetryingBySchedule(ctx, sched.Id)
		return err == nil
	}, 10*time.Second)

	dl, err := st.ScheduleDeadLetter.GetRetryingBySchedule(ctx, sched.Id)
	require.NoError(t, err)
	assert.Equal(t, schema.DeadLetterStateRetrying, dl.State)

	// 死信慢重试也全部失败 -> EXHAUSTED，槽位释放，一次性任务转 FAILED
	pump(t, d, func() bool {
		got := getSchedule(t, st, sched.Id)
		return got.FireState == nil
	}, 15*time.Second)

	final := getSchedule(t, st, sched.Id)
	assert.Equal(t, schema.ScheduleStateFailed, final.State, "一次性任务死信耗尽后应为 FAILED")
	assert.Nil(t, final.FireState)
	assert.NotEmpty(t, final.LastError)

	exhausted, err := st.ScheduleDeadLetter.Get(ctx, dl.Id)
	require.NoError(t, err)
	assert.Equal(t, schema.DeadLetterStateExhausted, exhausted.State)

	// 尝试总数 = 快速 3 次 + 死信 2 次
	assert.Equal(t, expected+len(fastConfig().DeadLetterIntervals), recv.count())

	failures, err := st.ScheduleAttemptFailure.ListBySchedule(ctx, sched.Id, 50)
	require.NoError(t, err)
	assert.Len(t, failures, recv.count(), "每一次失败都要有记录")
}

// 迟到容差：分片有一段时间没人扫（pod 挂了）时，错过的 occurrence 必须丢弃重算，
// 而不是迟到地补打一次。
func TestDispatcher_SkipsLateOccurrenceInsteadOfCatchingUp(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	recv := &receiver{}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	// next_fire_at 远早于现在（模拟停机期间错过）
	missedAt := time.Now().Add(-10 * time.Minute).UnixMilli()
	sched := seedSchedule(t, st, schema.ScheduleTypeCron, 0, missedAt)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	d := newDispatcher(t, st)
	pump(t, d, func() bool { return getSchedule(t, st, sched.Id).SkippedCount > 0 }, 5*time.Second)

	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, recv.count(), "迟到的 occurrence 绝不能被补打")

	got := getSchedule(t, st, sched.Id)
	assert.Greater(t, got.SkippedCount, int64(0), "错过的次数要计入 skipped_count")
	assert.Greater(t, got.NextFireAt, time.Now().UnixMilli(),
		"next_fire_at 应当对齐到当前时间之后")
	assert.Nil(t, got.FireState)
	assert.Equal(t, schema.ScheduleStateActive, got.State, "cron 跳过之后应当继续正常触发")
}

// stop 与「最后一次快重试失败」撞在一起时，不能再给已停止的 schedule 建死信：
// 槽位已经被 stop 清掉，那条死信永远不会被推进，只会在界面上一直显示「慢重试中」。
func TestDispatcher_StopDuringLastFastAttemptCreatesNoDeadLetter(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	ctx := context.Background()
	now := time.Now().UnixMilli()

	sched := seedSchedule(t, st, schema.ScheduleTypeCron, now-1000, now-1000)

	// 认领一次，模拟「最后一次快重试已经在飞」
	claimed, err := st.Schedule.ClaimDue(ctx, &store.ScheduleClaimParams{
		ShardStart: 0,
		ShardEnd:   sharding.ShardSize - 1,
		Now:        now,
		BatchSize:  10,
		LeaseOwner: "instance-test",
		LeaseUntil: now + 30_000,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	maxFast := len(fastConfig().FastRetryIntervals) + 1
	require.NoError(t, db.Exec("UPDATE schedules SET attempt_no = ? WHERE id = ?",
		maxFast, sched.Id).Error)

	inflight := getSchedule(t, st, sched.Id)
	fireId := *inflight.FireId

	// 投递还在飞的时候 schedule 被 stop：槽位清空、转 STOPPED
	stopped, err := st.Schedule.Stop(ctx, sched.Id, now)
	require.NoError(t, err)
	require.True(t, stopped)

	d := newDispatcher(t, st)
	d.enterDeadLetter(ctx, inflight, fireId, now, deliveryResult{errText: "boom"})

	_, err = st.ScheduleDeadLetter.GetRetryingBySchedule(ctx, sched.Id)
	assert.Error(t, err, "已停止的 schedule 不该再有死信")

	final := getSchedule(t, st, sched.Id)
	assert.Equal(t, schema.ScheduleStateStopped, final.State)
	assert.Nil(t, final.FireState, "stop 清掉的槽位不能被这次投递重新占上")
}

// 阻塞中的 fire 占着槽位时，到点的下一次 occurrence 不会被投递（Temporal 重叠语义）。
func TestDispatcher_DoesNotFireWhileSlotBusy(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	release := make(chan struct{})
	recv := &receiver{block: release}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	// 刚好到期而不是早就过期：过期超过迟到容差会走「跳过」路径（那是另一个用例），
	// 这里要的是一次真正会发出去的投递。
	now := time.Now().UnixMilli()
	sched := seedSchedule(t, st, schema.ScheduleTypeCron, 0, now)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	cfg := fastConfig()
	cfg.HTTPTimeout = 5 * time.Second
	require.NoError(t, cfg.Validate())

	ctx := context.Background()
	d := NewDispatcher(st, cfg, fullShard, "instance-test")

	// 第一次 tick：认领并阻塞在投递里
	go d.tick(ctx)

	// 等它真的进入投递（槽位被占）
	require.Eventually(t, func() bool {
		got, err := st.Schedule.Get(context.Background(), sched.Id)
		return err == nil && got.Busy()
	}, 5*time.Second, 5*time.Millisecond)

	// 槽位被占用期间再 tick 若干次：不应新增投递
	for i := 0; i < 3; i++ {
		d.tick(ctx)
	}
	time.Sleep(30 * time.Millisecond)

	assert.Equal(t, 1, recv.count(), "同一 schedule 同时只允许一个 in-flight fire")

	close(release)
	d.wg.Wait()
	d.Close()
}

func TestDispatcher_TriggerNowDoesNotAdvanceNextFireAt(t *testing.T) {
	db := newTestDB(t)
	st := newTestStore(t, db)
	seedNamespace(t, st)

	recv := &receiver{}
	srv := httptest.NewServer(recv.handler())
	defer srv.Close()

	// next_fire_at 设在未来：正常不会触发
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	sched := seedSchedule(t, st, schema.ScheduleTypeCron, 0, future)
	require.NoError(t, db.Exec("UPDATE schedules SET callback_url = ? WHERE id = ?",
		srv.URL+"/hook", sched.Id).Error)

	ctx := context.Background()
	fireId, err := uuid.NewV7()
	require.NoError(t, err)

	// 模拟 TriggerNow 占槽（fire_scheduled_time = 0 是手动触发标记）
	ok, err := st.Schedule.ClaimByID(ctx, sched.Id, &store.ClaimByIDParams{
		FireId:            fireId,
		FireState:         schema.FireStatePending,
		FireScheduledTime: 0,
		Now:               time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	require.True(t, ok)

	d := newDispatcher(t, st)
	pump(t, d, func() bool { return recv.count() == 1 }, 5*time.Second)

	got := getSchedule(t, st, sched.Id)
	assert.Equal(t, future, got.NextFireAt,
		"手动触发绝不能推进 next_fire_at——否则会打乱 cron 的自然节奏")
	assert.Nil(t, got.FireState)
}

// ShardOf 必须把 id 均匀打散。UUIDv7 的前 48 位是毫秒时间戳，如果拿前几字节取模，
// 同一秒注册的 schedule 会全部挤进同一个分片。
func TestShardOf_SpreadsIdsCreatedInTheSameMillisecond(t *testing.T) {
	const n = 500
	seen := map[int16]int{}

	for i := 0; i < n; i++ {
		id, err := uuid.NewV7()
		require.NoError(t, err)

		shard := ShardOf(id)
		require.GreaterOrEqual(t, shard, int16(0))
		require.Less(t, shard, int16(sharding.ShardSize))
		seen[shard]++
	}

	assert.Greater(t, len(seen), n/10,
		"分片分布过于集中（%d 条只落进 %d 个分片），分片键大概用错了字节", n, len(seen))
}

func TestJittered(t *testing.T) {
	const base = 100 * time.Millisecond

	// ratio 为 0 时原样返回
	assert.Equal(t, base, jittered(base, 0))

	// 抖动必须落在 [base*(1-ratio), base*(1+ratio)] 内，且确实有波动
	low := time.Duration(float64(base) * 0.8)
	high := time.Duration(float64(base) * 1.2)

	distinct := map[time.Duration]struct{}{}
	for i := 0; i < 200; i++ {
		got := jittered(base, 0.2)
		require.GreaterOrEqual(t, got, low)
		require.LessOrEqual(t, got, high)
		distinct[got] = struct{}{}
	}

	assert.Greater(t, len(distinct), 1, "抖动没有随机性，同点 schedule 会一起重试")
}
