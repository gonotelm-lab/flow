package audit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuditStore struct {
	mu      sync.Mutex
	logs    []*schema.AdminAuditLog
	batches int

	// entered/release 用来把 writer 精确地钉在 write() 里，避免时序测试靠 sleep 猜。
	entered chan struct{}
	release chan struct{}
}

var _ store.AdminAuditLog = (*fakeAuditStore)(nil)

func (f *fakeAuditStore) Create(_ context.Context, log *schema.AdminAuditLog) error {
	return f.CreateBatch(context.Background(), []*schema.AdminAuditLog{log})
}

func (f *fakeAuditStore) CreateBatch(_ context.Context, logs []*schema.AdminAuditLog) error {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches++
	f.logs = append(f.logs, logs...)
	return nil
}

func (f *fakeAuditStore) List(
	_ context.Context,
	_ *store.AdminAuditLogListParams,
) ([]*schema.AdminAuditLog, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs, int64(len(f.logs)), nil
}

func (f *fakeAuditStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logs)
}

func (f *fakeAuditStore) batchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.batches
}

func TestRecorder_FlushesAllEntriesOnClose(t *testing.T) {
	f := &fakeAuditStore{}
	r := New(f, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	for _, action := range []string{"CREATE_SCHEDULE", "STOP_SCHEDULE", "DELETE_SCHEDULE"} {
		r.Record(Entry{
			Action:     action,
			TargetType: schema.AuditTargetSchedule,
			TargetId:   "sched-1",
			Namespace:  "ns",
			Source:     schema.AuditSourceAdminHTTP,
			Result:     schema.AuditResultOk,
		})
	}

	r.Close()

	assert.Equal(t, 3, f.count(), "关闭后队列必须被刷干净")
	assert.Equal(t, int64(3), r.Written())
	assert.Equal(t, int64(0), r.Dropped())

	for _, log := range f.logs {
		assert.NotZero(t, log.CreateTime, "create_time 必须由记录器填上")
		// 主键也必须由记录器生成：admin_audit_logs.id 的数据库默认值不会生效
		// （GORM 会把 Go 零值 UUID 显式写进 INSERT），漏了就会全表撞主键。
		assert.NotEqual(t, uuid.Nil, log.Id, "主键必须由记录器生成")
		assert.Equal(t, schema.AuditSourceAdminHTTP, log.Source)
	}
}

func TestRecorder_WritesWithoutExplicitClose(t *testing.T) {
	f := &fakeAuditStore{}
	r := New(f, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	r.Record(Entry{Action: "CREATE_SCHEDULE", TargetType: schema.AuditTargetSchedule})

	require.Eventually(t, func() bool { return f.count() == 1 }, 2*time.Second, 5*time.Millisecond)
	r.Close()
}

// 先塞满队列再放行 writer：队列里积压的条目必须一条不少地落库。
func TestRecorder_WritesBacklogEnqueuedBeforeWriterRuns(t *testing.T) {
	f := &fakeAuditStore{}
	// BatchSize 大于条目数，且 writer 先被钉住，保证这些条目会凑成一批。
	f.entered = make(chan struct{}, 16)
	f.release = make(chan struct{})

	r := New(f, Config{QueueSize: 64, BatchSize: 32, EnqueueTimeout: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	// 先塞满队列，再放行 writer，让它一次收走
	for i := 0; i < 5; i++ {
		r.Record(Entry{Action: "CREATE_SCHEDULE", TargetId: "x"})
	}
	close(f.release)
	r.Close()

	assert.Equal(t, 5, f.count())
	assert.GreaterOrEqual(t, f.batchCount(), 1)
}

// 审计重要，但绝不能把业务拖死：队列满时必须短暂等待后丢弃，而不是无限阻塞。
func TestRecorder_NeverBlocksBusinessWhenQueueFull(t *testing.T) {
	f := &fakeAuditStore{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
	}

	r := New(f, Config{
		QueueSize:      2,
		BatchSize:      1,
		EnqueueTimeout: 20 * time.Millisecond,
		FlushTimeout:   time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	// 第一条会触发 write 并卡在 fakeStore 里
	r.Record(Entry{Action: "A1"})
	<-f.entered

	// 队列容量 2，这两条立即入队
	r.Record(Entry{Action: "A2"})
	r.Record(Entry{Action: "A3"})

	// 队满：这一条应当等待约 EnqueueTimeout 后被丢弃，而不是一直等
	start := time.Now()
	r.Record(Entry{Action: "A4"})
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 15*time.Millisecond, "应当至少等待到超时")
	assert.Less(t, elapsed, time.Second, "绝不能无限阻塞业务")
	assert.Greater(t, r.Dropped(), int64(0), "被丢弃的条目必须计数，不能静默消失")

	close(f.release)
	r.Close()
	assert.GreaterOrEqual(t, f.count(), 3, "已入队的条目仍要落库")
}
