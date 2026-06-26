// flow/client/worker/internal/runtime/poll_test.go
package runtime

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
)

func TestPollLoop_ProcessesTaskAndReports(t *testing.T) {
	taskID := "550e8400-e29b-41d4-a716-446655440000"
	mock := &testutil.MockWorkerService{
		PollResponses: [][]*schemav1.Task{
			{{Id: taskID, Payload: []byte("input")}},
			nil, // 第二次 poll 空返回，便于退出
		},
	}
	conn, cleanup := startMockServer(t, mock)
	defer cleanup()

	var handled atomic.Int32
	handler := func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte) {
		handled.Add(1)
		require.Equal(t, []byte("input"), task.GetPayload())
		return workerv1.ReportAction_SUCCESS, []byte("output")
	}

	ctx, cancel := context.WithCancel(context.Background())

	sem := NewSemaphore(1)
	reporter := NewReporter(workerv1.NewWorkerServiceClient(conn), slog.Default())
	poll := NewPollLoop(PollLoopConfig{
		Conn:       conn,
		WorkerID:   1,
		Namespace:  "ns",
		TaskType:   "render",
		Handler:    handler,
		Reporter:   reporter,
		Semaphore:  sem,
		Logger:     slog.Default(),
	})

	go poll.Run(ctx)

	require.Eventually(t, func() bool { return handled.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return len(mock.ReportsSnapshot()) == 1 }, time.Second, 10*time.Millisecond)

	report := mock.ReportsSnapshot()[0]
	require.Equal(t, workerv1.ReportAction_SUCCESS, report.Action)
	require.Equal(t, []byte("output"), report.Payload)
	require.Equal(t, taskID, report.TaskId)

	cancel()
	sem.Wait()
}
