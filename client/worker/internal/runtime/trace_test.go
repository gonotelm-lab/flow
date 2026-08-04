// flow/client/worker/internal/runtime/trace_test.go
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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	noop "go.opentelemetry.io/otel/trace/noop"
)

func setupTraceTest(t *testing.T) (*tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})
	return exporter, tp
}

// storedTraceparent 用 provider 造一个"已结束的提交侧 span"，返回其 traceparent。
func storedTraceparent(t *testing.T, tp *sdktrace.TracerProvider) string {
	t.Helper()
	ctx, span := tp.Tracer("test").Start(context.Background(), "submit")
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	span.End()
	tpVal := carrier.Get("traceparent")
	require.NotEmpty(t, tpVal)
	return tpVal
}
func runPollWithTask(t *testing.T, traceparent string, mode string, capture func(ctx context.Context)) {
	t.Helper()
	mock := &testutil.MockWorkerService{
		PollTraceparent: traceparent, // mock 在 Poll 响应 trailer 中下发
		PollResponses: [][]*schemav1.Task{
			{{Id: "550e8400-e29b-41d4-a716-446655440000", Namespace: "ns", TaskType: "render"}},
			nil,
		},
	}
	conn, cleanup := startMockServer(t, mock)
	t.Cleanup(cleanup)

	sem := NewSemaphore(1)
	reporter := NewReporter(workerv1.NewWorkerServiceClient(conn), slog.Default())
	poll := NewPollLoop(PollLoopConfig{
		Conn:       conn,
		WorkerID:   1,
		Namespace:  "ns",
		TaskType:   "render",
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			capture(ctx)
			return workerv1.ReportAction_SUCCESS, nil, false
		},
		Reporter:  reporter,
		Semaphore: sem,
		Logger:    slog.Default(),
		TraceMode: TraceMode(mode),
	})

	ctx, cancel := context.WithCancel(context.Background())
	go poll.Run(ctx)
	t.Cleanup(func() { cancel(); sem.Wait() })
}

func TestRunTask_ChildMode_ContinuesTrace(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)
	exporter.Reset() // 清掉构造用的 submit span

	var handlerSC oteltrace.SpanContext
	runPollWithTask(t, stored, "child", func(ctx context.Context) {
		handlerSC = oteltrace.SpanContextFromContext(ctx)
	})

	require.Eventually(t, func() bool { return handlerSC.IsValid() }, time.Second, 10*time.Millisecond)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	handle := spans[0]
	require.Equal(t, "task.handle", handle.Name)
	require.Equal(t, oteltrace.SpanKindInternal, handle.SpanKind)

	storedSC, ok := spanContextFromTraceparent(stored)
	require.True(t, ok)
	require.Equal(t, storedSC.TraceID(), handle.SpanContext.TraceID())
	require.Equal(t, storedSC.SpanID(), handle.Parent.SpanID(), "handler span must be child of stored span")
	require.Equal(t, handlerSC.SpanID(), handle.SpanContext.SpanID())
}

func TestRunTask_LinkMode_NewTraceWithLink(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)
	exporter.Reset() // 清掉构造用的 submit span

	var handlerSC oteltrace.SpanContext
	runPollWithTask(t, stored, "link", func(ctx context.Context) {
		handlerSC = oteltrace.SpanContextFromContext(ctx)
	})

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
}

func TestRunTask_NoTraceparent_NoSpan(t *testing.T) {
	exporter, _ := setupTraceTest(t)

	ran := false
	runPollWithTask(t, "", "child", func(ctx context.Context) {
		ran = true
	})

	require.Eventually(t, func() bool { return ran }, time.Second, 10*time.Millisecond)

	for _, s := range exporter.GetSpans() {
		require.NotEqual(t, "task.handle", s.Name, "no task.handle span expected without traceparent")
	}
}

func TestPollFailureRecordsErrorSpan(t *testing.T) {
	exporter, _ := setupTraceTest(t)
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	cleanup() // 立即关闭连接 → poll 必失败

	sem := NewSemaphore(1)
	poll := NewPollLoop(PollLoopConfig{
		Conn:      conn,
		WorkerID:  1,
		Namespace: "ns",
		TaskType:  "t",
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			return workerv1.ReportAction_SUCCESS, nil, false
		},
		Semaphore: sem,
		Logger:    slog.Default(),
		TraceMode: TraceModeChild,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go poll.Run(ctx)
	t.Cleanup(func() { cancel(); sem.Wait() })

	require.Eventually(t, func() bool {
		for _, s := range exporter.GetSpans() {
			if s.Name == "worker.poll" && s.Status.Code == codes.Error {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func TestPollFailureSpanChildOfParentContext(t *testing.T) {
	exporter, _ := setupTraceTest(t)
	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	defer parent.End()

	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	cleanup() // 立即关闭连接 → poll 必失败

	sem := NewSemaphore(1)
	poll := NewPollLoop(PollLoopConfig{
		Conn:      conn,
		WorkerID:  1,
		Namespace: "ns",
		TaskType:  "t",
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte, bool) {
			return workerv1.ReportAction_SUCCESS, nil, false
		},
		Semaphore: sem,
		Logger:    slog.Default(),
		TraceMode: TraceModeChild,
	})

	go poll.Run(parentCtx)
	t.Cleanup(func() { sem.Wait() })

	require.Eventually(t, func() bool {
		for _, s := range exporter.GetSpans() {
			if s.Name == "worker.poll" && s.Status.Code == codes.Error {
				return s.Parent.SpanID() == parent.SpanContext().SpanID()
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}
