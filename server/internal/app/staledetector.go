package app

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type StaleDetector struct {
	store     *repository.Store
	interval  time.Duration
	timeout   time.Duration
	batchSize int

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewStaleDetector(store *repository.Store, cfg *config.WorkerConfig) *StaleDetector {
	interval := cfg.StaleScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	timeout := cfg.StaleTaskTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	batchSize := cfg.StaleScanBatch
	if batchSize <= 0 {
		batchSize = 100
	}
	return &StaleDetector{
		store:     store,
		interval:  interval,
		timeout:   timeout,
		batchSize: batchSize,
	}
}

func (d *StaleDetector) Run(ctx context.Context) {
	d.wg.Add(1)
	defer d.wg.Done()

	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		d.cancel = nil
		d.mu.Unlock()
	}()

	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(runCtx, "stale detector panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			d.detect(runCtx)
		}
	}
}

func (d *StaleDetector) Close() {
	d.mu.Lock()
	cancel := d.cancel
	d.cancel = nil
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	d.wg.Wait()
}

func (d *StaleDetector) detect(ctx context.Context) {
	timeoutMs := int64(d.timeout / time.Millisecond)
	tasks, err := d.store.Task.GetStaleTasks(ctx, timeoutMs, d.batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "stale detector get tasks failed", slog.Any("err", err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	nowMilli := time.Now().UnixMilli()
	for _, task := range tasks {
		task.State = schemav1.TaskState_FAILED.String()
		task.WorkerId = 0
		task.UpdateTime = nowMilli
		task.Error = []byte("last heartbeat time exceeded timeout")
	}

	if err := d.store.Task.BatchUpdate(ctx, tasks); err != nil {
		slog.ErrorContext(ctx, "stale detector batch update failed", slog.Any("err", err))
		return
	}

	payload := []byte("last heartbeat time exceeded timeout")
	for _, task := range tasks {
		_ = d.store.TaskEvent.Append(ctx, &reposchema.TaskEvent{
			TaskId:     task.Id,
			EventType:  "STALE_DETECTED",
			CreateTime: nowMilli,
			Payload:    payload,
		})
	}
}
