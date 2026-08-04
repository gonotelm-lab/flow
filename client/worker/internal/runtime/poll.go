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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// traceContextPropagator 固定使用 W3C TraceContext，不依赖宿主全局 propagator 配置。
var traceContextPropagator = propagation.TraceContext{}

// spanContextFromTraceparent 解析 W3C traceparent 为远程 SpanContext；无效时 ok=false。
func spanContextFromTraceparent(tp string) (oteltrace.SpanContext, bool) {
	if tp == "" {
		return oteltrace.SpanContext{}, false
	}
	carrier := propagation.MapCarrier{"traceparent": tp}
	ctx := traceContextPropagator.Extract(context.Background(), carrier)
	sc := oteltrace.SpanContextFromContext(ctx)
	return sc, sc.IsValid()
}

type TaskHandler func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool)

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
	TraceMode TraceMode
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

		var md metadata.MD
		resp, err := p.client.Poll(ctx, &workerv1.PollRequest{
			Id:        p.cfg.WorkerID,
			Namespace: p.cfg.Namespace,
			TaskType:  p.cfg.TaskType,
		}, grpc.Trailer(&md))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.cfg.Logger.Error("poll failed", "err", err)
			recordFailureSpan(ctx, "worker.poll", p.cfg.WorkerID, err)
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
		traceparent := ""
		if vals := md.Get("traceparent"); len(vals) > 0 {
			traceparent = vals[0]
		}
		go p.runTask(ctx, taskCopy, traceparent)
	}
}

// startTaskSpan 在调用 Handler 前恢复任务的 traceparent：
// child 模式续接存储 span（remote parent），link 模式开新 trace 并 link 关联。
// 无 traceparent / 无效 / 无全局 provider 时返回原 ctx 与 noop span（End 安全）。
func (p *PollLoop) startTaskSpan(ctx context.Context, task *schemav1.Task, traceparent string) (context.Context, oteltrace.Span) {
	sc, ok := spanContextFromTraceparent(traceparent)
	if !ok {
		return ctx, oteltrace.SpanFromContext(ctx)
	}

	opts := []oteltrace.SpanStartOption{
		oteltrace.WithAttributes(
			attribute.String("flow.task.id", task.GetId()),
			attribute.String("flow.task.namespace", task.GetNamespace()),
			attribute.String("flow.task.type", task.GetTaskType()),
			attribute.Int64("flow.worker.id", p.cfg.WorkerID),
		),
	}

	if p.cfg.TraceMode == TraceModeLink {
		opts = append(opts, oteltrace.WithLinks(oteltrace.Link{SpanContext: sc}))
	} else {
		ctx = oteltrace.ContextWithRemoteSpanContext(ctx, sc)
	}

	ctx, span := tracer().Start(ctx, "task.handle", opts...)
	return ctx, span
}

func (p *PollLoop) runTask(ctx context.Context, task *schemav1.Task, traceparent string) {
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
			_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, workerv1.ReportAction_FAIL, []byte("panic"), false)
		}
	}()

	p.cfg.Logger.Info("task started", "task_id", taskID)
	handlerCtx, taskSpan := p.startTaskSpan(taskCtx, task, traceparent)
	defer taskSpan.End()
	action, payload, skipRetry := p.cfg.Handler(handlerCtx, task)
	if taskCtx.Err() == nil {
		p.cfg.Logger.Info("task finished", "task_id", taskID, "action", action.String())
		_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, action, payload, skipRetry)
	}
}
