package runtime

import (
	"context"
	"log/slog"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
)

// 这一组守的是「关停时结果不能丢」：
//
// 服务端把超过 staleTaskTimeout 仍处于 RUNNING 的任务直接判成 FAILED（终态、不重试），
// 所以一个已经跑完的任务如果因为关停没回报，最终会变成失败。

const shutdownTaskID = "550e8400-e29b-41d4-a716-446655440001"

// blockingHandler 在 started 上放行、等 release 才返回，用来把「handler 正在跑」这一刻钉住。
func blockingHandler(started chan struct{}, release chan struct{}) TaskHandler {
	return func(context.Context, *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
		close(started)
		<-release
		return workerv1.ReportAction_SUCCESS, []byte("output"), false
	}
}

func newShutdownPollLoop(t *testing.T, mock *testutil.MockWorkerService, handler TaskHandler) (*PollLoop, *Semaphore, func()) {
	t.Helper()

	conn, cleanup := startMockServer(t, mock)
	sem := NewSemaphore(1)

	poll := NewPollLoop(PollLoopConfig{
		Conn:      conn,
		WorkerID:  1,
		Namespace: "ns",
		TaskType:  "render",
		Handler:   handler,
		Reporter:  NewReporter(workerv1.NewWorkerServiceClient(conn), slog.Default()),
		Semaphore: sem,
		Logger:    slog.Default(),
	})

	return poll, sem, cleanup
}

func pollOnce(taskID string) *testutil.MockWorkerService {
	return &testutil.MockWorkerService{
		PollResponses: [][]*schemav1.Task{
			{{Id: taskID, Payload: []byte("input")}},
			nil, // 第二次 poll 空返回，便于退出
		},
	}
}

// 关停（父 ctx 被取消）之后，已经跑完的任务仍然要把结果报回去。
func TestPollLoop_ReportsOutcomeAfterShutdown(t *testing.T) {
	mock := pollOnce(shutdownTaskID)

	started := make(chan struct{})
	release := make(chan struct{})
	poll, sem, cleanup := newShutdownPollLoop(t, mock, blockingHandler(started, release))
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	go poll.Run(ctx)

	<-started
	cancel()       // Runtime.Stop 做的第一件事就是取消运行 ctx
	close(release) // handler 正常跑完

	// Stop 会先 sem.Wait() 再 unregister：回报在 Release 之前完成，所以等到这里就够了。
	sem.Wait()

	reports := mock.ReportsSnapshot()
	require.Len(t, reports, 1, "关停不该丢掉已跑完任务的结果")
	require.Equal(t, workerv1.ReportAction_SUCCESS, reports[0].Action)
	require.Equal(t, shutdownTaskID, reports[0].TaskId)
	require.Equal(t, []byte("output"), reports[0].Payload)
}

// 服务端主动取消的任务不回报：它在服务端已经是 CANCELLED，回报只会被忽略。
func TestPollLoop_SkipsReportWhenCanceledByServer(t *testing.T) {
	mock := pollOnce(shutdownTaskID)

	started := make(chan struct{})
	release := make(chan struct{})
	poll, sem, cleanup := newShutdownPollLoop(t, mock, blockingHandler(started, release))
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go poll.Run(ctx)

	<-started
	require.Eventually(t, func() bool {
		return len(poll.RunningTaskIDs()) == 1
	}, time.Second, 10*time.Millisecond, "任务应当先登记成在飞状态")

	poll.CancelTask(shutdownTaskID)
	close(release)
	sem.Wait()

	require.Never(t, func() bool {
		return len(mock.ReportsSnapshot()) > 0
	}, 200*time.Millisecond, 20*time.Millisecond, "服务端取消的任务不该被回报")
}
