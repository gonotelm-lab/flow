// flow/client/worker/internal/runtime/heartbeat.go
package runtime

import (
	"context"
	"log/slog"
	"time"

	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"google.golang.org/grpc"
)

type HeartbeatLoop struct {
	client      workerv1.WorkerServiceClient
	workerID    int64
	interval    time.Duration
	logger      *slog.Logger
	runningIDs  func() []string
	onCancelled func([]string)
}

func NewHeartbeatLoop(
	conn grpc.ClientConnInterface,
	workerID int64,
	interval time.Duration,
	logger *slog.Logger,
	runningIDs func() []string,
	onCancelled func([]string),
) *HeartbeatLoop {
	return &HeartbeatLoop{
		client:      workerv1.NewWorkerServiceClient(conn),
		workerID:    workerID,
		interval:    interval,
		logger:      logger,
		runningIDs:  runningIDs,
		onCancelled: onCancelled,
	}
}

func (h *HeartbeatLoop) filterCancelledByRunning(cancelled, running []string) []string {
	if len(cancelled) == 0 || len(running) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(running))
	for _, id := range running {
		set[id] = struct{}{}
	}
	result := make([]string, 0, len(cancelled))
	for _, id := range cancelled {
		if _, ok := set[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func (h *HeartbeatLoop) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runningIDs := h.runningIDs()
			resp, err := h.client.Heartbeat(ctx, &workerv1.HeartbeatRequest{
				Id:             h.workerID,
				RunningTaskIds: runningIDs,
			})
			if err != nil {
				h.logger.Error("heartbeat failed", "worker_id", h.workerID, "err", err)
				recordFailureSpan(ctx, "worker.heartbeat", h.workerID, err)
				continue
			}

			if cancelled := resp.GetCancelledTaskIds(); len(cancelled) > 0 {
				filtered := h.filterCancelledByRunning(cancelled, runningIDs)
				if len(filtered) > 0 {
					h.logger.Info("received cancelled tasks", "task_ids", filtered)
					h.onCancelled(filtered)
				}
			}
		}
	}
}
