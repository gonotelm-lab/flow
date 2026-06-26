// flow/client/worker/internal/runtime/runtime.go
package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"google.golang.org/grpc"
)

type RuntimeConfig struct {
	Conn              grpc.ClientConnInterface
	Namespace         string
	TaskType          string
	Name              string
	MaxConcurrency    int
	HeartbeatInterval time.Duration
	Handler           TaskHandler
	Logger            *slog.Logger
	OwnsConn          bool // true 时 Stop 关闭连接
}

type Runtime struct {
	cfg      RuntimeConfig
	client   workerv1.WorkerServiceClient
	workerID int64

	mu      sync.Mutex
	cancel  context.CancelFunc
	sem     *Semaphore
	poll    *PollLoop
	hb      *HeartbeatLoop
	started bool
}

func New(cfg RuntimeConfig) *Runtime {
	return &Runtime{
		cfg:    cfg,
		client: workerv1.NewWorkerServiceClient(cfg.Conn),
	}
}

func (r *Runtime) WorkerID() int64 {
	return r.workerID
}

func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return nil
	}

	worker, err := r.client.Register(ctx, &schemav1.Worker{
		Name:      r.cfg.Name,
		Namespace: r.cfg.Namespace,
		TaskType:  r.cfg.TaskType,
	})
	if err != nil {
		return err
	}
	r.workerID = worker.GetId()
	r.cfg.Logger.Info("worker registered", "worker_id", r.workerID)

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.sem = NewSemaphore(r.cfg.MaxConcurrency)
	reporter := NewReporter(r.client, r.cfg.Logger)

	r.poll = NewPollLoop(PollLoopConfig{
		Conn:      r.cfg.Conn,
		WorkerID:  r.workerID,
		Namespace: r.cfg.Namespace,
		TaskType:  r.cfg.TaskType,
		Handler:   r.cfg.Handler,
		Reporter:  reporter,
		Semaphore: r.sem,
		Logger:    r.cfg.Logger,
	})
	r.hb = NewHeartbeatLoop(r.cfg.Conn, r.workerID, r.cfg.HeartbeatInterval, r.cfg.Logger,
		func() []string { return r.poll.RunningTaskIDs() },
		func(ids []string) {
			for _, id := range ids {
				r.poll.CancelTask(id)
			}
		},
	)

	go r.hb.Run(runCtx)
	go r.poll.Run(runCtx)
	r.started = true
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	sem := r.sem
	workerID := r.workerID
	client := r.client
	ownsConn := r.cfg.OwnsConn
	conn, _ := r.cfg.Conn.(*grpc.ClientConn)
	r.mu.Unlock()

	if sem != nil {
		sem.Wait()
	}

	if workerID > 0 {
		_, err := client.Unregister(ctx, &workerv1.UnregisterRequest{Id: workerID})
		if err != nil {
			r.cfg.Logger.Error("unregister failed", "worker_id", workerID, "err", err)
		} else {
			r.cfg.Logger.Info("worker unregistered", "worker_id", workerID)
		}
	}

	if ownsConn && conn != nil {
		_ = conn.Close()
	}
	return nil
}
