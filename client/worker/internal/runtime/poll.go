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
}

func NewPollLoop(cfg PollLoopConfig) *PollLoop {
	return &PollLoop{
		cfg:    cfg,
		client: workerv1.NewWorkerServiceClient(cfg.Conn),
	}
}

func (p *PollLoop) Run(ctx context.Context) {
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
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

	defer func() {
		if r := recover(); r != nil {
			p.cfg.Logger.Error("task handler panic",
				"task_id", task.GetId(),
				"panic", r,
				"stack", string(debug.Stack()),
			)
			_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, workerv1.ReportAction_FAIL, []byte("panic"))
		}
	}()

	p.cfg.Logger.Info("task started", "task_id", task.GetId())
	action, payload := p.cfg.Handler(ctx, task)
	p.cfg.Logger.Info("task finished", "task_id", task.GetId(), "action", action.String())
	_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, action, payload)
}
