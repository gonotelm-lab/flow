// flow/client/worker/internal/runtime/poll.go
package runtime

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
)

type TaskHandler func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte)

type Semaphore struct {
	sem *semaphore.Weighted
	wg  sync.WaitGroup
}

func NewSemaphore(maxConcurrency int) *Semaphore {
	return &Semaphore{sem: semaphore.NewWeighted(int64(maxConcurrency))}
}

func (s *Semaphore) Acquire(ctx context.Context) error {
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	s.wg.Add(1)
	return nil
}

func (s *Semaphore) IsFull() bool {
	if s.sem.TryAcquire(1) {
		s.sem.Release(1)
		return false
	}
	return true
}

func (s *Semaphore) Release() {
	s.sem.Release(1)
	s.wg.Done()
}

func (s *Semaphore) Wait() {
	s.wg.Wait()
}

type PollLoopConfig struct {
	Conn      grpc.ClientConnInterface
	WorkerID  int64
	Namespace string
	TaskType  string
	Handler   TaskHandler
	Reporter  *Reporter
	Semaphore *Semaphore
	Logger    *slog.Logger
}

type PollLoop struct {
	cfg    PollLoopConfig
	client workerv1.WorkerServiceClient

	mu          sync.Mutex
	runningIDs  map[string]struct{}
	cancelFuncs map[string]context.CancelFunc
}

func NewPollLoop(cfg PollLoopConfig) *PollLoop {
	return &PollLoop{
		cfg:         cfg,
		client:      workerv1.NewWorkerServiceClient(cfg.Conn),
		runningIDs:  make(map[string]struct{}),
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}

func (p *PollLoop) RunningTaskIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.runningIDs))
	for id := range p.runningIDs {
		ids = append(ids, id)
	}
	return ids
}

func (p *PollLoop) CancelTask(taskID string) {
	p.mu.Lock()
	cancel, ok := p.cancelFuncs[taskID]
	p.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

func (p *PollLoop) Run(ctx context.Context) {
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		if p.cfg.Semaphore.IsFull() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		resp, err := p.client.Poll(ctx, &workerv1.PollRequest{
			Id:        p.cfg.WorkerID,
			Namespace: p.cfg.Namespace,
			TaskType:  p.cfg.TaskType,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.cfg.Logger.Error("poll failed", "err", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		task := resp.GetTask()
		if task == nil || task.GetId() == "" {
			continue
		}

		if err := p.cfg.Semaphore.Acquire(ctx); err != nil {
			return
		}

		taskCopy := task
		go p.runTask(ctx, taskCopy)
	}
}

func (p *PollLoop) runTask(ctx context.Context, task *schemav1.Task) {
	defer p.cfg.Semaphore.Release()

	taskID := task.GetId()
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	p.mu.Lock()
	p.runningIDs[taskID] = struct{}{}
	p.cancelFuncs[taskID] = cancel
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.runningIDs, taskID)
		delete(p.cancelFuncs, taskID)
		p.mu.Unlock()
	}()

	defer func() {
		if r := recover(); r != nil {
			p.cfg.Logger.Error("task handler panic",
				"task_id", taskID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
			_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, workerv1.ReportAction_FAIL, []byte("panic"))
		}
	}()

	p.cfg.Logger.Info("task started", "task_id", taskID)
	action, payload := p.cfg.Handler(taskCtx, task)
	if taskCtx.Err() == nil {
		p.cfg.Logger.Info("task finished", "task_id", taskID, "action", action.String())
		_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, action, payload)
	}
}
