// flow/client/worker/internal/runtime/runtime_test.go
package runtime

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// startMockServerWithTrace 同 startMockServer，但 conn 带 otelgrpc client handler，
// 用于验证 RPC 的 trace 关联行为。
func startMockServerWithTrace(t *testing.T, svc *testutil.MockWorkerService) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	testutil.Register(s, svc)
	go func() { _ = s.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	require.NoError(t, err)
	return conn, func() { _ = conn.Close(); s.Stop() }
}

func TestRuntime_StartStop(t *testing.T) {
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	defer cleanup()

	rt := New(RuntimeConfig{
		Conn:              conn,
		Namespace:         "ns",
		TaskType:          "render",
		Name:              "w1",
		MaxConcurrency:    1,
		HeartbeatInterval: 20 * time.Millisecond,
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			return workerv1.ReportAction_SUCCESS, []byte("ok"), false
		},
		Logger: slog.Default(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, rt.Start(ctx))
	require.Greater(t, rt.WorkerID(), int64(0))

	cancel()
	require.NoError(t, rt.Stop(context.Background()))
}

func TestRuntime_TraceModeLink_PropagatesToPoll(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)
	exporter.Reset()

	mock := &testutil.MockWorkerService{
		PollTraceparent: stored,
		PollResponses: [][]*schemav1.Task{
			{{Id: "task-1", Namespace: "ns", TaskType: "render"}},
			nil,
		},
	}
	conn, cleanup := startMockServer(t, mock)
	t.Cleanup(cleanup)

	var handlerSC oteltrace.SpanContext
	rt := New(RuntimeConfig{
		Conn:              conn,
		Namespace:         "ns",
		TaskType:          "render",
		Name:              "w1",
		MaxConcurrency:    1,
		HeartbeatInterval: 20 * time.Millisecond,
		TraceMode:         TraceModeLink,
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			handlerSC = oteltrace.SpanContextFromContext(ctx)
			return workerv1.ReportAction_SUCCESS, nil, false
		},
		Logger: slog.Default(),
	})
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, rt.Start(ctx))

	require.Eventually(t, func() bool { return handlerSC.IsValid() }, time.Second, 10*time.Millisecond)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	handle := spans[0]
	require.Equal(t, "task.handle", handle.Name)

	storedSC, ok := spanContextFromTraceparent(stored)
	require.True(t, ok)
	require.NotEqual(t, storedSC.TraceID(), handle.SpanContext.TraceID(), "link mode must start a new trace")
	require.Zero(t, handle.Parent.SpanID(), "link mode span must be a root span")
	require.Len(t, handle.Links, 1)
	require.Equal(t, storedSC.SpanID(), handle.Links[0].SpanContext.SpanID())
	require.Equal(t, handlerSC.SpanID(), handle.SpanContext.SpanID())
}

func TestRuntime_TraceModeChild_PropagatesToPoll(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)
	exporter.Reset()

	mock := &testutil.MockWorkerService{
		PollTraceparent: stored,
		PollResponses: [][]*schemav1.Task{
			{{Id: "task-1", Namespace: "ns", TaskType: "render"}},
			nil,
		},
	}
	conn, cleanup := startMockServer(t, mock)
	t.Cleanup(cleanup)

	var handlerSC oteltrace.SpanContext
	rt := New(RuntimeConfig{
		Conn:              conn,
		Namespace:         "ns",
		TaskType:          "render",
		Name:              "w1",
		MaxConcurrency:    1,
		HeartbeatInterval: 20 * time.Millisecond,
		TraceMode:         TraceModeChild,
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			handlerSC = oteltrace.SpanContextFromContext(ctx)
			return workerv1.ReportAction_SUCCESS, nil, false
		},
		Logger: slog.Default(),
	})
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, rt.Start(ctx))

	require.Eventually(t, func() bool { return handlerSC.IsValid() }, time.Second, 10*time.Millisecond)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	handle := spans[0]

	storedSC, ok := spanContextFromTraceparent(stored)
	require.True(t, ok)
	require.Equal(t, storedSC.TraceID(), handle.SpanContext.TraceID(), "child mode must continue the stored trace")
	require.Equal(t, storedSC.SpanID(), handle.Parent.SpanID())
	require.Equal(t, handlerSC.SpanID(), handle.SpanContext.SpanID())
}

func TestRuntime_ReportSpanChildOfTaskSpan(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)
	exporter.Reset()

	mock := &testutil.MockWorkerService{
		PollTraceparent: stored,
		PollResponses: [][]*schemav1.Task{
			{{Id: "task-1", Namespace: "ns", TaskType: "render"}},
			nil,
		},
	}
	conn, cleanup := startMockServerWithTrace(t, mock)
	t.Cleanup(cleanup)

	rt := New(RuntimeConfig{
		Conn:              conn,
		Namespace:         "ns",
		TaskType:          "render",
		Name:              "w1",
		MaxConcurrency:    1,
		HeartbeatInterval: 20 * time.Millisecond,
		TraceMode:         TraceModeChild,
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			return workerv1.ReportAction_SUCCESS, []byte("ok"), false
		},
		Logger: slog.Default(),
	})
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, rt.Start(ctx))

	require.Eventually(t, func() bool {
		for _, s := range exporter.GetSpans() {
			if strings.Contains(s.Name, "WorkerService/Report") {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)

	var handleSpanID oteltrace.SpanID
	for _, s := range exporter.GetSpans() {
		if s.Name == "task.handle" {
			handleSpanID = s.SpanContext.SpanID()
		}
	}
	require.NotZero(t, handleSpanID, "task.handle span must exist")

	found := false
	for _, s := range exporter.GetSpans() {
		if strings.Contains(s.Name, "WorkerService/Report") {
			require.Equal(t, handleSpanID, s.Parent.SpanID(), "report span must be child of task.handle span")
			found = true
		}
	}
	require.True(t, found)
}