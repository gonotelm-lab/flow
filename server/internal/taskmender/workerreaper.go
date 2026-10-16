package taskmender

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

const maxReapRounds = 20

type WorkerReaper struct {
	store     *repository.Store
	interval  time.Duration
	retention time.Duration
	batchSize int

	// done 由 Close 关闭，run 见到它就退出。单靠 ctx 不行：run 可能还没开始跑，
	// Close 就已经在等了，那时没有任何东西能通知它退出。
	done      chan struct{}
	closeOnce sync.Once

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWorkerReaper(store *repository.Store, cfg *config.WorkerConfig) *WorkerReaper {
	interval := cfg.WorkerReapInterval
	if interval <= 0 {
		interval = time.Hour
	}
	retention := cfg.WorkerRetention
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	batchSize := cfg.WorkerReapBatch
	if batchSize <= 0 {
		batchSize = 500
	}
	return &WorkerReaper{
		store:     store,
		interval:  interval,
		retention: retention,
		batchSize: batchSize,
		done:      make(chan struct{}),
	}
}

// Start 启动后台回收循环，配套 Close 关闭。
//
// WaitGroup 必须在这里登记，不能放进 goroutine 内部：若调用方写 `go r.Run(ctx)`，
// Close 可能在协程真正开跑之前就等到计数为零返回，之后那个协程会一直活着。
func (r *WorkerReaper) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.run(ctx)
}

func (r *WorkerReaper) run(ctx context.Context) {
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
			slog.ErrorContext(runCtx, "worker reaper panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return
		case <-r.done:
			return
		case <-ticker.C:
			r.reap(runCtx)
		}
	}
}

// Close 停掉回收循环并等它退出。可以重复调用。
func (r *WorkerReaper) Close() {
	r.closeOnce.Do(func() { close(r.done) })

	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

func (r *WorkerReaper) reap(ctx context.Context) {
	cutoff := time.Now().Add(-r.retention).UnixMilli()

	var total int64
	for round := 0; round < maxReapRounds; round++ {
		deleted, err := r.store.TaskWorker.DeleteExpired(ctx, cutoff, r.batchSize)
		if err != nil {
			slog.ErrorContext(ctx, "worker reaper delete expired failed",
				slog.Any("err", err),
				slog.Int64("deleted", total),
			)
			return
		}

		total += deleted
		if deleted < int64(r.batchSize) {
			break
		}

		if ctx.Err() != nil {
			break
		}
	}

	if total > 0 {
		slog.InfoContext(ctx, "worker reaper removed expired workers",
			slog.Int64("count", total),
			slog.Duration("retention", r.retention),
		)
	}
}
