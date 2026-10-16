// Package audit 提供 admin 操作审计的异步记录器。
//
// 写入是异步的，这是明确的设计选择：业务变更提交后把条目投进进程内的有界队列，
// 由后台 goroutine 批量落库。代价是进程被 kill 或崩溃时，已经提交的业务变更会丢掉
// 对应的审计记录，换来的是审计写入永远不会拖慢或阻塞业务请求。
package audit

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/google/uuid"
)

// Config 是记录器的可调参数，零值由 DefaultConfig 补齐。
type Config struct {
	// QueueSize 有界队列长度
	QueueSize int
	// BatchSize 单次批量写入的最大条数
	BatchSize int
	// EnqueueTimeout 队列满时允许阻塞的上限；超时即丢弃并计数
	EnqueueTimeout time.Duration
	// FlushTimeout 关闭时刷盘的超时
	FlushTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		QueueSize:      1024,
		BatchSize:      100,
		EnqueueTimeout: 50 * time.Millisecond,
		FlushTimeout:   5 * time.Second,
	}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	if c.QueueSize <= 0 {
		c.QueueSize = d.QueueSize
	}
	if c.BatchSize <= 0 {
		c.BatchSize = d.BatchSize
	}
	if c.EnqueueTimeout <= 0 {
		c.EnqueueTimeout = d.EnqueueTimeout
	}
	if c.FlushTimeout <= 0 {
		c.FlushTimeout = d.FlushTimeout
	}
	return c
}

// Entry 是一条审计记录的业务内容；id 与 create_time 由记录器补。
type Entry struct {
	Action     string
	TargetType string
	TargetId   string
	Namespace  string
	// Operator 保留字段，当前恒为空（无用户体系）
	Operator     string
	SourceIp     string
	UserAgent    string
	Source       string
	Snapshot     []byte
	Result       string
	ErrorMessage string
}

type Recorder struct {
	store store.AdminAuditLog
	cfg   Config

	ch chan *schema.AdminAuditLog

	// done 由 Close 关闭，run 见到它就退出。单靠 ctx 不行：run 可能还没开始跑，
	// Close 就已经在等了，那时没有任何东西能通知它退出。
	done      chan struct{}
	closeOnce sync.Once

	wg sync.WaitGroup

	mu     sync.Mutex
	cancel context.CancelFunc

	written atomic.Int64
	dropped atomic.Int64
}

func New(s store.AdminAuditLog, cfg Config) *Recorder {
	cfg = cfg.withDefaults()
	return &Recorder{
		store: s,
		cfg:   cfg,
		ch:    make(chan *schema.AdminAuditLog, cfg.QueueSize),
		done:  make(chan struct{}),
	}
}

// Record 投递一条审计条目。绝不阻塞业务：队列满时最多阻塞 EnqueueTimeout，
// 超时则丢弃并计入 Dropped()。
func (r *Recorder) Record(e Entry) {
	// 主键必须在这里生成。
	//
	// admin_audit_logs.id 虽然是 UUID DEFAULT uuidv7()，但 GORM 会把 Go 的零值 UUID
	// 显式写进 INSERT，数据库默认值就不生效了：第一条以全零主键成功，之后每一条都撞
	// 主键失败。而异步写入只把错误打到日志，接口照样返回 200，数据静默丢掉。
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}

	log := &schema.AdminAuditLog{
		Id:           id,
		Action:       e.Action,
		TargetType:   e.TargetType,
		TargetId:     e.TargetId,
		Namespace:    e.Namespace,
		Operator:     e.Operator,
		SourceIp:     e.SourceIp,
		UserAgent:    e.UserAgent,
		Source:       e.Source,
		Snapshot:     e.Snapshot,
		Result:       e.Result,
		ErrorMessage: e.ErrorMessage,
		CreateTime:   time.Now().UnixMilli(),
	}

	select {
	case r.ch <- log:
		return
	default:
	}

	// 队列已满：短暂等待，仍满则丢弃。审计重要但不能把业务拖死。
	timer := time.NewTimer(r.cfg.EnqueueTimeout)
	defer timer.Stop()

	select {
	case r.ch <- log:
	case <-timer.C:
		n := r.dropped.Add(1)
		slog.Warn("audit queue full, entry dropped",
			slog.String("action", e.Action),
			slog.String("target_id", e.TargetId),
			slog.Int64("total_dropped", n),
		)
	}
}

// Start 启动后台写入循环，配套 Close 关闭。
//
// WaitGroup 必须在这里登记，不能放进 goroutine 内部：若调用方写 `go r.Run(ctx)`，
// Close 可能在协程真正开跑之前就等到计数为零返回，之后那个协程才开始读 ctx——
// 关机等不到它退出，`-race` 下也是明确的数据竞争。
func (r *Recorder) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.run(ctx)
}

func (r *Recorder) run(ctx context.Context) {
	defer r.wg.Done()

	runCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		r.cancel = nil
		r.mu.Unlock()
	}()

	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(runCtx, "audit recorder panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	for {
		select {
		case <-runCtx.Done():
			r.drain()
			return
		case <-r.done:
			r.drain()
			return
		case first := <-r.ch:
			r.write(r.collect(first))
		}
	}
}

// Close 停掉写入循环、刷完队列、等 goroutine 退出。可以重复调用。
//
// drain 不能只靠 Run 的收尾：Run 可能还没开始跑，Close 的 wg.Wait() 就已经返回了，
// 那时队列里的条目只有靠这里自己刷。drain 从同一个 channel 取，不会重复写。
func (r *Recorder) Close() {
	r.closeOnce.Do(func() { close(r.done) })

	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	r.drain()
}

// Written 返回已成功落库的条数（仅用于测试与观测）。
func (r *Recorder) Written() int64 { return r.written.Load() }

// Dropped 返回被丢弃的条数（仅用于测试与观测）。
func (r *Recorder) Dropped() int64 { return r.dropped.Load() }

// collect 以 first 打头，尽可能多地凑满一批（非阻塞）。
func (r *Recorder) collect(first *schema.AdminAuditLog) []*schema.AdminAuditLog {
	batch := make([]*schema.AdminAuditLog, 0, r.cfg.BatchSize)
	batch = append(batch, first)

	for len(batch) < r.cfg.BatchSize {
		select {
		case next := <-r.ch:
			batch = append(batch, next)
		default:
			return batch
		}
	}
	return batch
}

func (r *Recorder) write(batch []*schema.AdminAuditLog) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.FlushTimeout)
	defer cancel()

	if err := r.store.CreateBatch(ctx, batch); err != nil {
		slog.ErrorContext(ctx, "audit batch write failed",
			slog.Any("err", err),
			slog.Int("count", len(batch)),
		)
		// 异步写入失败不向上报：调用方已经拿到业务结果，这里只能记录。
		// 计入 dropped，免得看起来像是写成功了。
		r.dropped.Add(int64(len(batch)))
		return
	}

	r.written.Add(int64(len(batch)))
}

// drain 在关闭时把队列剩余条目写完。
func (r *Recorder) drain() {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.FlushTimeout)
	defer cancel()

	for {
		select {
		case log := <-r.ch:
			batch := r.collect(log)
			if err := r.store.CreateBatch(ctx, batch); err != nil {
				slog.ErrorContext(ctx, "audit final flush failed",
					slog.Any("err", err),
					slog.Int("count", len(batch)),
				)
				r.dropped.Add(int64(len(batch)))
				continue
			}
			r.written.Add(int64(len(batch)))
		default:
			return
		}
	}
}
