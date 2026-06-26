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

type RetryMender struct {
	store     *repository.Store
	interval  time.Duration
	batchSize int

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRetryMender(store *repository.Store, cfg *config.WorkerConfig) *RetryMender {
	interval := cfg.RetryScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	batchSize := cfg.RetryScanBatch
	if batchSize <= 0 {
		batchSize = 100
	}
	return &RetryMender{
		store:     store,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (r *RetryMender) Run(ctx context.Context) {
	r.wg.Add(1)
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
			slog.ErrorContext(runCtx, "retry mender panic",
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
		case <-ticker.C:
			r.mend(runCtx)
		}
	}
}

func (r *RetryMender) Close() {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

func (r *RetryMender) mend(ctx context.Context) {
	tasks, err := r.store.Task.GetRetriableTasks(ctx, r.batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "retry mender get tasks failed", slog.Any("err", err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	nowMilli := time.Now().UnixMilli()
	for _, task := range tasks {
		task.AttemptNo++
		task.State = schemav1.TaskState_INITED.String()
		task.WorkerId = 0
		task.UpdateTime = nowMilli

		shift := task.AttemptNo - 1
		if shift > 10 {
			shift = 10
		}
		backoffMs := 30000 * (1 << shift)
		if backoffMs > 600000 {
			backoffMs = 600000
		}
		task.NextRunTime = nowMilli + int64(backoffMs)
	}

	if err := r.store.Task.BatchUpdate(ctx, tasks); err != nil {
		slog.ErrorContext(ctx, "retry mender batch update failed", slog.Any("err", err))
		return
	}

	for _, task := range tasks {
		_ = r.store.TaskEvent.Append(ctx, &reposchema.TaskEvent{
			TaskId:     task.Id,
			EventType:  "RETRIED",
			CreateTime: nowMilli,
			Payload:    []byte(task.State),
		})
	}
}
