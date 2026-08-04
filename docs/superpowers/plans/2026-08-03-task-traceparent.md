# Task Traceparent 跨进程链路延续 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Submit 时将 traceparent 存入 tasks 表，worker 分发时自动恢复链路（child 延续 / link 关联可配置，默认 child），使用方仅更新 SDK 即开箱接入。

**Architecture:** server 在 Submit service 内用 W3C propagator 从 ctx 提取 traceparent 随任务落库；worker SDK 的 poll runtime 在调用 Handler 前用 `flow/otel.SpanContextFromTraceparent` 恢复远程 span context，child 模式作 remote parent 续链、link 模式作根 span + link。解析/提取纯函数放 `flow/otel` 集中实现并单测。

**Tech Stack:** Go 1.25、OTel v1.44.0（otel/trace、otel/propagation）、gRPC v1.81.1、buf v1.71.0、PostgreSQL

## Global Constraints

- 版本对齐 gonotelm：otel 核心 v1.44.0、otlptracegrpc/http v1.20.0、otelgrpc/otelhttp v0.59.0、semconv v1.41.0（内置）
- W3C traceparent 单列存储：`tasks.traceparent VARCHAR(55)`
- worker SDK 默认零配置接入：`TraceMode` 空值 = child；无 traceparent / 无全局 provider 时 no-op
- 无 span 时 Submit 忽略（存空串），不产生语义
- 集成测试放 `tests/` 目录；包内单测（tracetest）留在业务 package
- 迁移用 ALTER（已有数据），沿用 pgsql18.sql 内联风格
- 测试命令统一 `go test ./...`（对应模块目录）；server 排除 `repository/impl/postgres`（需真实 DB）

---

### Task 1: flow/otel traceprop 纯函数

**Files:**
- Create: `flow/otel/traceprop.go`
- Create: `flow/otel/traceprop_test.go`

**Interfaces:**
- Consumes: 无（依赖已在 flow/otel go.mod）
- Produces（后续任务依赖）:
  - `flowotel.TraceparentFromContext(ctx context.Context) string` — 无 span 时返回空串
  - `flowotel.SpanContextFromTraceparent(tp string) (trace.SpanContext, bool)` — 无效时 ok=false

- [ ] **Step 1: 写失败测试 traceprop_test.go**

```go
package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTraceparentFromContext_NoSpan(t *testing.T) {
	if got := TraceparentFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty traceparent without span, got %q", got)
	}
}

func TestTraceparentFromContext_WithSpan(t *testing.T) {
	sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{0x01},
		SpanID:     oteltrace.SpanID{0x02},
		TraceFlags: oteltrace.FlagsSampled,
	})
	ctx := oteltrace.ContextWithRemoteSpanContext(context.Background(), sc)

	got := TraceparentFromContext(ctx)
	if got == "" {
		t.Fatal("expected non-empty traceparent")
	}

	// 恢复后必须能还原同一 trace/span
	restored, ok := SpanContextFromTraceparent(got)
	if !ok {
		t.Fatalf("round-trip failed for %q", got)
	}
	if restored.TraceID() != sc.TraceID() || restored.SpanID() != sc.SpanID() {
		t.Fatalf("round-trip mismatch: got trace=%s span=%s, want trace=%s span=%s",
			restored.TraceID(), restored.SpanID(), sc.TraceID(), sc.SpanID())
	}
	if restored.TraceFlags()&oteltrace.FlagsSampled == 0 {
		t.Fatal("sampled flag lost in round-trip")
	}
}

func TestSpanContextFromTraceparent_Invalid(t *testing.T) {
	cases := []string{"", "garbage", "00-zz-zz-01", "00-0000-0000-00"}
	for _, tp := range cases {
		if _, ok := SpanContextFromTraceparent(tp); ok {
			t.Fatalf("expected invalid for %q", tp)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run（workdir `flow/otel`）: `go test ./...`
Expected: FAIL — `TraceparentFromContext` / `SpanContextFromTraceparent` undefined

- [ ] **Step 3: 实现 traceprop.go**

```go
package otel

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// traceContextPropagator 固定使用 W3C TraceContext，不依赖宿主全局 propagator 配置。
var traceContextPropagator = propagation.TraceContext{}

// TraceparentFromContext 用 W3C TraceContext 从 ctx 提取 traceparent；无 span 时返回空串。
func TraceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	traceContextPropagator.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// SpanContextFromTraceparent 解析 W3C traceparent 为远程 SpanContext；无效时 ok=false。
func SpanContextFromTraceparent(tp string) (trace.SpanContext, bool) {
	if tp == "" {
		return trace.SpanContext{}, false
	}
	carrier := propagation.MapCarrier{"traceparent": tp}
	ctx := traceContextPropagator.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)
	return sc, sc.IsValid()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run（workdir `flow/otel`）: `go test ./...`
Expected: PASS（若 `TestSpanContextFromTraceparent_Invalid` 的 "00-0000-0000-00" 被判定为有效，将其从用例中移除——TraceContext 解析对该格式返回无效）

- [ ] **Step 5: 提交**

```bash
git add otel/traceprop.go otel/traceprop_test.go
git commit -m "feat(otel): add traceparent extract and parse helpers"
```

---

### Task 2: 数据层（migration + schema + poll trailer）

**Files:**
- Modify: `migration/pgsql18.sql`
- Modify: `server/internal/repository/schema/task.go`
- Modify: `server/internal/service/worker/service.go`（Poll 响应 trailer）

**Interfaces:**
- Consumes: 无
- Produces（后续任务依赖）:
  - `reposchema.Task.Traceparent string`
  - 数据库列 `tasks.traceparent VARCHAR(55)`
  - 行为：`Service.Poll`/`tryPoll` 返回任务时，若 `task.Traceparent != ""`，通过 `grpc.SetTrailer(ctx, metadata.Pairs("traceparent", task.Traceparent))` 下发（gRPC 标准 metadata 载体，不加 proto 字段）

- [ ] **Step 1: migration 加 ALTER（已有数据，必须 ALTER）**

`migration/pgsql18.sql` 在 `COMMENT ON COLUMN tasks.last_heartbeat_time ...` 之后追加：

```sql
ALTER TABLE tasks ADD COLUMN traceparent VARCHAR(55);
COMMENT ON COLUMN tasks.traceparent IS 'task traceparent from submit context';
```

- [ ] **Step 2: gorm schema 加字段**

`server/internal/repository/schema/task.go` 的 `Task` 结构加一行：

```go
	Traceparent string `gorm:"column:traceparent"`
```

- [ ] **Step 3: Poll 响应设置 trailer**

`server/internal/service/worker/service.go`：
- import 增加 `"google.golang.org/grpc/metadata"`
- `tryPoll` 中 `if task == nil { return nil, nil }` 之后、`return &workerv1.PollResponse{Task: toProtoTask(task)}, nil` 之前插入：

```go
	if task.Traceparent != "" {
		_ = grpc.SetTrailer(requestCtx, metadata.Pairs("traceparent", task.Traceparent))
	}
```

（注意：必须使用 `requestCtx`（客户端 ctx），不能用 `pollCtx`——`grpc.SetTrailer` 需要调用方 ctx 才能在 RPC 结束时带上 trailer）

- [ ] **Step 4: 编译验证**

Run（workdir `flow/server`）: `go build ./... && go test ./internal/service/... ./internal/repository/... ./pkg/sql/ 2>&1 | tail -3`
Expected: 编译通过，测试 PASS（postgres 集成测试包除外）

- [ ] **Step 5: 提交**

```bash
git add migration server/internal/repository/schema/task.go server/internal/service/worker/service.go
git commit -m "feat: store task traceparent column and deliver via poll trailer"
```

---

### Task 3: server Submit 捕获

**Files:**
- Modify: `server/internal/service/task/impl.go`

**Interfaces:**
- Consumes: Task 1 的 `flowotel.TraceparentFromContext(ctx)`
- Produces: 无（行为：Submit 请求 ctx 有 span 则落库，无则空串）

- [ ] **Step 1: Submit 写入 traceparent**

`server/internal/service/task/impl.go`：
- import 增加 `flowotel "github.com/gonotelm-lab/flow/otel"`
- `Submit` 中 `task := &reposchema.Task{...}` 加一行：

```go
		Traceparent: flowotel.TraceparentFromContext(ctx),
```

- [ ] **Step 2: 编译 + 回归**

Run（workdir `flow/server`）: `go build ./... && go test ./internal/service/task/ ./internal/endpoint/ 2>&1 | tail -3`
Expected: 编译通过，测试 PASS

- [ ] **Step 3: 提交**

```bash
git add server/internal/service/task/impl.go
git commit -m "feat(server): capture traceparent on task submit"
```

---

### Task 4: worker SDK 自动恢复（开箱即用）

**Files:**
- Modify: `client/worker/config.go`
- Modify: `client/worker/worker.go`
- Modify: `client/worker/internal/runtime/runtime.go`
- Modify: `client/worker/internal/runtime/poll.go`
- Create: `client/worker/internal/runtime/trace_test.go`

**Interfaces:**
- Consumes:
  - Task 1 的 `flowotel.SpanContextFromTraceparent(tp string) (trace.SpanContext, bool)`
  - 现有 `startMockServer(t, svc)`（reporter_test.go）、`testutil.MockWorkerService`、`PollLoop` / `PollLoopConfig`
- Produces:
  - `runtime.TraceMode`（`TraceModeChild="child"` / `TraceModeLink="link"`，定义在 runtime 包，**避免 import cycle**）
  - `worker.TraceMode` / `worker.TraceModeChild` / `worker.TraceModeLink`（类型别名，`client/worker` 对外 API）
  - `worker.Config.TraceMode`（空值默认 child）
  - `runtime.RuntimeConfig.TraceMode TraceMode`（经 `client/worker/worker.go` 传入）
  - 行为：runTask 在调用 Handler 前创建 `task.handle` span（attrs: task.id/namespace/task_type/worker_id）；child 模式 remote parent 续链、link 模式根 span + link；无 traceparent/无 provider 时 no-op

- [ ] **Step 1: 写失败测试 trace_test.go（先扩展 mock 支持 trailer）**

`client/worker/internal/runtime/testutil/mock_server.go`：`MockWorkerService` 加字段，`Poll` 方法设置响应 trailer：

```go
	PollTraceparent string // 非空时在 Poll 响应 trailer 下发 "traceparent"
```

`Poll` 方法在 `m.mu.Unlock()` 之前（有任务返回时）插入：

```go
	if m.PollTraceparent != "" {
		_ = grpc.SetTrailer(ctx, metadata.Pairs("traceparent", m.PollTraceparent))
	}
```

（import 增加 `"google.golang.org/grpc/metadata"`）

创建 `trace_test.go`（package runtime，白盒）：

```go
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
	flowotel "github.com/gonotelm-lab/flow/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
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
	tp := carrier.Get("traceparent")
	require.NotEmpty(t, tp)
	return tp
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
	defer cleanup()

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

	storedSC, ok := flowotel.SpanContextFromTraceparent(stored)
	require.True(t, ok)
	require.Equal(t, storedSC.TraceID(), handle.SpanContext.TraceID())
	require.Equal(t, storedSC.SpanID(), handle.Parent.SpanID(), "handler span must be child of stored span")
	require.Equal(t, handlerSC.SpanID(), handle.SpanContext.SpanID())
}

func TestRunTask_LinkMode_NewTraceWithLink(t *testing.T) {
	exporter, tp := setupTraceTest(t)
	stored := storedTraceparent(t, tp)

	var handlerSC oteltrace.SpanContext
	runPollWithTask(t, stored, "link", func(ctx context.Context) {
		handlerSC = oteltrace.SpanContextFromContext(ctx)
	})

	require.Eventually(t, func() bool { return handlerSC.IsValid() }, time.Second, 10*time.Millisecond)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	handle := spans[0]
	require.Equal(t, "task.handle", handle.Name)

	storedSC, ok := flowotel.SpanContextFromTraceparent(stored)
	require.True(t, ok)
	require.NotEqual(t, storedSC.TraceID(), handle.SpanContext.TraceID(), "link mode must start a new trace")
	require.NotZero(t, handle.Parent.SpanID(), "link mode span must be a root span")
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
```

- [ ] **Step 2: 运行测试确认失败**

Run（workdir `flow/client`）: `go test ./worker/internal/runtime/ -run TestRunTask_ -count=1 2>&1 | tail -5`
Expected: FAIL — `TraceMode` 字段不存在（PollLoopConfig 编译错误）与恢复逻辑未实现

- [ ] **Step 3: runtime.go 定义 TraceMode 并加字段**

`client/worker/internal/runtime/runtime.go` 的 `RuntimeConfig` 结构前加类型定义、结构中加字段：

```go
// TraceMode 控制 worker 恢复任务 traceparent 的方式。
// 定义在 runtime 包：worker 包通过类型别名导出，避免 import cycle。
type TraceMode string

const (
	// TraceModeChild 默认：handler span 作为存储 span 的子 span（remote parent），延续同一条链路。
	TraceModeChild TraceMode = "child"
	// TraceModeLink：handler span 为根 span，通过 link 关联存储 span。
	TraceModeLink TraceMode = "link"
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
	TraceMode         TraceMode
}
```

- [ ] **Step 4: worker/config.go 导出别名并加 Config 字段**

`client/worker/config.go`：

```go
package worker

import (
	"log/slog"
	"time"

	"github.com/gonotelm-lab/flow/client/worker/internal/runtime"
)

// TraceMode 及其常量由 runtime 包定义，此处类型别名导出为 worker 对外 API。
type TraceMode = runtime.TraceMode

const (
	TraceModeChild = runtime.TraceModeChild
	TraceModeLink  = runtime.TraceModeLink
)

type Config struct {
	Namespace         string
	TaskType          string
	Name              string
	MaxConcurrency    int
	HeartbeatInterval time.Duration
	Codec             Codec
	Logger            *slog.Logger
	TraceMode         TraceMode
}

func ConfigWithDefaults(cfg Config) Config {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	if cfg.Codec == nil {
		cfg.Codec = JSONCodec{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.TraceMode == "" {
		cfg.TraceMode = TraceModeChild
	}
	return cfg
}
```

- [ ] **Step 5: worker.go 透传 TraceMode**

`client/worker/worker.go` 的 `Start()` 中 `runtime.New(runtime.RuntimeConfig{...})` 增加：

```go
		TraceMode:          c.cfg.TraceMode,
```

- [ ] **Step 6: poll.go 实现恢复（分发前创建 span）**

`client/worker/internal/runtime/poll.go`：
- import 增加：

```go
	flowotel "github.com/gonotelm-lab/flow/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
```

- `PollLoopConfig` 加 `TraceMode TraceMode`
- `Run` 中读取 Poll 响应 trailer（`resp, err := p.client.Poll(...)` 改为）：

```go
		var md metadata.MD
		resp, err := p.client.Poll(ctx, &workerv1.PollRequest{
			Id:        p.cfg.WorkerID,
			Namespace: p.cfg.Namespace,
			TaskType:  p.cfg.TaskType,
		}, grpc.Trailer(&md))
```

- `go p.runTask(ctx, taskCopy)` 改为：

```go
		taskCopy := task
		traceparent := ""
		if vals := md.Get("traceparent"); len(vals) > 0 {
			traceparent = vals[0]
		}
		go p.runTask(ctx, taskCopy, traceparent)
```

- `runTask` 签名改为 `func (p *PollLoop) runTask(ctx context.Context, task *schemav1.Task, traceparent string)`，内部 `p.cfg.Handler(taskCtx, task)` 处改为：

```go
	handlerCtx, taskSpan := p.startTaskSpan(taskCtx, task, traceparent)
	action, payload, skipRetry := p.cfg.Handler(handlerCtx, task)
	if taskSpan != nil {
		taskSpan.End()
	}
```

- 新增方法（在 `runTask` 之前）：

```go
// startTaskSpan 在调用 Handler 前恢复任务的 traceparent：
// child 模式续接存储 span（remote parent），link 模式开新 trace 并 link 关联。
// 无 traceparent / 无效 / 无全局 provider 时返回原 ctx 与 noop span。
func (p *PollLoop) startTaskSpan(ctx context.Context, task *schemav1.Task, traceparent string) (context.Context, oteltrace.Span) {
	tracer := otel.Tracer("flow.worker")
	opts := []oteltrace.SpanStartOption{
		oteltrace.WithAttributes(
			attribute.String("task.id", task.GetId()),
			attribute.String("namespace", task.GetNamespace()),
			attribute.String("task_type", task.GetTaskType()),
			attribute.Int64("worker_id", p.cfg.WorkerID),
		),
	}

	if sc, ok := flowotel.SpanContextFromTraceparent(traceparent); ok {
		if p.cfg.TraceMode == TraceModeLink {
			opts = append(opts, oteltrace.WithLinks(oteltrace.Link{SpanContext: sc}))
		} else {
			ctx = oteltrace.ContextWithRemoteSpanContext(ctx, sc)
		}
	}

	ctx, span := tracer.Start(ctx, "task.handle", opts...)
	return ctx, span
}
```

- [ ] **Step 7: 依赖调整**

Run（workdir `flow/client`）: `go mod tidy`
Expected: `go.opentelemetry.io/otel` / `go.opentelemetry.io/otel/trace` 从 indirect 转为 direct；flow/otel 保持 direct

- [ ] **Step 8: 修正测试并运行**

`trace_test.go` 无需改动 `TraceMode` 用法（Step 1 已直接用本包 `TraceMode`）。将 Step 1 中暂缺的 `TestRunTask_NoTraceparent_NoSpan` 完善为：

```go
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
```

Run（workdir `flow/client`）: `go test ./worker/internal/runtime/ -count=1 2>&1 | tail -3`
Expected: 全部 PASS（含原有测试）

- [ ] **Step 9: 全量回归**

Run（workdir `flow/client`）: `go build ./... && go test ./...`
Expected: 全部 PASS

- [ ] **Step 10: 提交**

```bash
git add client
git commit -m "feat(client): auto restore task traceparent before handler dispatch"
```

---

### Task 5: 文档

**Files:**
- Modify: `docs/tracing.md`

**Interfaces:**
- Consumes: 前面任务的最终行为
- Produces: 无

- [ ] **Step 1: tracing.md 补充**

在「Client SDK 接入（可选注入）」小节末尾追加：

```markdown
### 任务链路延续（traceparent 落库 + worker 自动恢复）

Submit 时若请求 ctx 存在 span（otelgrpc 标准注入），server 将 W3C `traceparent`
随任务写入 `tasks.traceparent` 列（无 span 则忽略，存空串）。worker Poll 领取任务后，
SDK **自动**在调用 Handler 前创建 `task.handle` span（带 task.id/namespace/task_type/worker_id
属性），无需任何配置：

- **默认 child 模式**：handler span 作为提交侧 span 的子 span（remote parent），
  提交方 → server → worker 构成一条完整 trace
- **link 模式**：handler span 为根 span，通过 link 关联提交侧 span（新 trace）

```go
client, err := worker.New(addr, worker.Config{
	Namespace: "demo",
	TaskType:  "raw",
	TraceMode: worker.TraceModeChild, // 可省略，默认 child
})
```

使用方只需更新 SDK 并保持 `flow/otel.Init`（或自定义 provider + W3C propagator），
即可开箱获得跨进程链路；未接入 OTel 时自动 no-op。
```

- [ ] **Step 2: 提交**

```bash
git add docs/tracing.md
git commit -m "docs: document task traceparent continuation"
```

---

## 验收清单（跨任务）

- [ ] `flow/otel`：`go test ./...` 全绿（traceparent 提取/解析单测）
- [ ] `flow/server`：`go build ./...` + 非 postgres 集成包测试全绿；Submit 写入 `Traceparent`
- [ ] `flow/client`：`go test ./...` 全绿，含 child/link/no-traceparent 三个恢复单测
- [ ] `api`：`buf generate` 产物与手写代码一致，`Task` 含 `traceparent` 字段
- [ ] migration：`ALTER TABLE tasks ADD COLUMN traceparent VARCHAR(55);` + COMMENT
