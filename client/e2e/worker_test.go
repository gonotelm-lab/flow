package e2e

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	task "github.com/gonotelm-lab/flow/client/task/v2"
	"github.com/gonotelm-lab/flow/client/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	e2eTaskType     = "e2e-echo"
	consumeTimeout  = 20 * time.Second
	reportTimeout   = 15 * time.Second
	shutdownTimeout = 10 * time.Second
)

func e2eLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// waitForState 轮询任务状态，直到到达 want 或超时。
func waitForState(t *testing.T, ctx context.Context, client *task.Client, taskID string, want task.State, timeout time.Duration) task.State {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var state task.State
	for time.Now().Before(deadline) {
		got, err := client.Get(ctx, taskID)
		require.NoError(t, err)
		state = got.State
		if state == want {
			return state
		}
		time.Sleep(200 * time.Millisecond)
	}

	return state
}

// 不带 key / key 不认识 / namespace 与 key 不匹配时，注册就该被拒。
func TestWorkerClientRejectsBadCredential(t *testing.T) {
	requireE2E(t)

	if !authEnabled(t) {
		t.Skip("鉴权关闭：key 会被忽略，这组断言不适用")
	}

	nsA, keyA := newNamespace(t)
	nsB, _ := newNamespace(t)

	noop := func(context.Context, []byte) (worker.Result, error) { return worker.OkResult{}, nil }

	for _, tc := range []struct {
		name string
		ns   string
		key  string
		want codes.Code
	}{
		{name: "no key", ns: nsA, key: "", want: codes.Unauthenticated},
		{name: "unknown key", ns: nsA, key: "sk-not-a-real-key", want: codes.Unauthenticated},
		{name: "namespace mismatch", ns: nsB, key: keyA, want: codes.PermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := worker.New(serverAddr(t), worker.Config{
				Namespace: tc.ns,
				TaskType:  e2eTaskType,
				APIKey:    tc.key,
				Logger:    e2eLogger(),
			})
			require.NoError(t, err)
			client.Handle(noop)

			assert.Equal(t, tc.want, status.Code(client.Start()), "Start 应当被拒")
			require.NoError(t, client.Close())
		})
	}
}

// 提交任务 → worker 消费 → 上报 → 任务变 DONE（带 key 的 Register / Poll / Heartbeat / Report 全链路）。
func TestWorkerConsumesTask(t *testing.T) {
	requireE2E(t)

	ctx := context.Background()
	namespace, apiKey := newNamespace(t)

	tasks, err := task.New(serverAddr(t), task.Config{Namespace: namespace, APIKey: apiKey})
	require.NoError(t, err)
	defer func() { _ = tasks.Close() }()

	created, err := tasks.Submit(ctx, e2eTaskType, []byte(`{"msg":"hi"}`))
	require.NoError(t, err)

	// 鉴权关闭时不带 key，顺便验证 key 确实被忽略。
	workerKey := apiKey
	if !authEnabled(t) {
		workerKey = ""
	}

	handled := make(chan map[string]string, 1)
	w, err := worker.New(serverAddr(t), worker.Config{
		Namespace:         namespace,
		TaskType:          e2eTaskType,
		APIKey:            workerKey,
		MaxConcurrency:    1,
		HeartbeatInterval: time.Second,
		Logger:            e2eLogger(),
	})
	require.NoError(t, err)
	worker.RegisterTyped[map[string]string, map[string]string](w, func(_ context.Context, in map[string]string) (map[string]string, error) {
		handled <- in
		return map[string]string{"ok": "1"}, nil
	})

	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()

	select {
	case in := <-handled:
		assert.Equal(t, "hi", in["msg"])
	case <-time.After(consumeTimeout):
		t.Fatal("worker 没在超时前消费到任务")
	}

	assert.Equal(t, task.StateDone, waitForState(t, ctx, tasks, created.ID, task.StateDone, reportTimeout))

	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// 关停时已经跑完的任务必须把结果报回去：服务端会把停在 RUNNING 的任务交给 stale 检测，
// 而它直接判 FAILED（终态、不重试），跑成功的任务不能因为关停变成失败。
func TestWorkerShutdownStillReports(t *testing.T) {
	requireE2E(t)

	ctx := context.Background()
	namespace, apiKey := newNamespace(t)

	tasks, err := task.New(serverAddr(t), task.Config{Namespace: namespace, APIKey: apiKey})
	require.NoError(t, err)
	defer func() { _ = tasks.Close() }()

	created, err := tasks.Submit(ctx, e2eTaskType, []byte(`{"msg":"bye"}`))
	require.NoError(t, err)

	handled := make(chan struct{})
	w, err := worker.New(serverAddr(t), worker.Config{
		Namespace:         namespace,
		TaskType:          e2eTaskType,
		APIKey:            apiKey,
		MaxConcurrency:    1,
		HeartbeatInterval: time.Second,
		Logger:            e2eLogger(),
	})
	require.NoError(t, err)
	worker.RegisterTyped[map[string]string, map[string]string](w, func(_ context.Context, _ map[string]string) (map[string]string, error) {
		close(handled)
		return map[string]string{"ok": "1"}, nil
	})

	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()

	select {
	case <-handled:
	case <-time.After(consumeTimeout):
		t.Fatal("worker 没在超时前消费到任务")
	}

	// 关键：handler 一返回就关停，不给上报留「自然完成」的时间。
	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	assert.Equal(t, task.StateDone, waitForState(t, ctx, tasks, created.ID, task.StateDone, shutdownTimeout),
		"关停后任务仍应是 DONE，而不是停在 RUNNING")
}
