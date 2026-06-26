// flow/client/worker/internal/runtime/reporter.go
package runtime

import (
	"context"
	"log/slog"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
)

type Reporter struct {
	client workerv1.WorkerServiceClient
	logger *slog.Logger
}

func NewReporter(client workerv1.WorkerServiceClient, logger *slog.Logger) *Reporter {
	return &Reporter{client: client, logger: logger}
}

func (r *Reporter) ReportTask(
	ctx context.Context,
	workerID int64,
	task *schemav1.Task,
	action workerv1.ReportAction,
	payload []byte,
) error {
	_, err := r.client.Report(ctx, &workerv1.ReportRequest{
		WorkerId: workerID,
		TaskId:   task.GetId(),
		Action:   action,
		Payload:  payload,
	})
	if err != nil {
		r.logger.Error("report task failed",
			"worker_id", workerID,
			"task_id", task.GetId(),
			"action", action.String(),
			"err", err,
		)
	}
	return err
}
