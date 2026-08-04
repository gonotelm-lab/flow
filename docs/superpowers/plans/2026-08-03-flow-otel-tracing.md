# Flow OpenTelemetry Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Flow（server + SDK client）接入 OpenTelemetry 最新标准端到端链路追踪（仅 Traces），新增共享模块 flow/otel 统一 SDK 初始化。

**Architecture:** 新建 flow/otel 模块（env 标准 OTEL_* + Option 覆盖，供 server/client 共用）。Server 侧：gRPC 服务端用 otelgrpc stats handler、admin HTTP gateway 用 otelhttp + otelgrpc client handler、GORM 用官方插件。Client 侧：task/worker SDK 默认带 otelgrpc client stats handler（可选注入，无全局 provider 时 no-op）。链路：client 发 span → traceparent 经 gRPC metadata 传到 server → server 续 span。

**Tech Stack:** Go 1.25、OpenTelemetry Go v1.44.0、otelgrpc/otelhttp v0.69.0、gorm.io/plugin/opentelemetry v0.1.16（tracing 子包）、gRPC v1.81.1、GORM v1.31.1

## Global Constraints

- OTel 依赖版本固定：`go.opentelemetry.io/otel`、`go.opentelemetry.io/otel/sdk`、`otlptracegrpc`、`otlptracehttp` 全部 v1.44.0；`otelgrpc`、`otelhttp` v0.69.0；`gorm.io/plugin/opentelemetry` v0.1.16
- semconv 一律用 `go.opentelemetry.io/otel/semconv/v1.41.0`（随 otel v1.44.0 内置，**不要**加独立依赖、不要用其他版本）
- 仅 Traces，不引入 metrics/logs 信号
- SDK 客户端**不**调用 flow/otel.Init（可选注入），不设置任何全局状态
- server 后台组件（sweeper/watcher/mender）不插桩
- 不改动 gonotelm 模块
- 所有命令在 `flow/` 目录（git 仓库）内执行；提交信息风格参考 `git log`（`feat: ...` / `fix: ...` / `chore: ...`）
- 测试命令统一 `go test ./...`（在对应模块目录）

---

### Task 1: flow/otel 模块（Init/Shutdown + env 标准 + Option）

**Files:**
- Create: `flow/otel/go.mod`
- Create: `flow/otel/option.go`
- Create: `flow/otel/env.go`
- Create: `flow/otel/otel.go`
- Create: `flow/otel/env_test.go`
- Create: `flow/otel/otel_test.go`
- Modify: `go.work`（仓库根目录）

**Interfaces:**
- Consumes: 无（第一个任务）
- Produces（后续任务依赖）:
  - `flowotel.Init(ctx context.Context, opts ...Option) error` — 幂等，env 标准 + Option 覆盖
  - `flowotel.Shutdown(ctx context.Context)` — 幂等，可重入（Shutdown 后允许再次 Init）
  - `func WithServiceName(name string) Option`
  - `func WithEndpoint(endpoint string) Option`
  - `func WithProtocol(p Protocol) Option`（`Protocol` 为 string 类型，`ProtocolGRPC="grpc"` / `ProtocolHTTP="http"`）
  - `func WithSamplerRatio(ratio float64) Option`

- [ ] **Step 1: 创建 flow/otel/go.mod 并加入 go.work**

创建 `flow/otel/go.mod`：

```
module github.com/gonotelm-lab/flow/otel

go 1.25.4

require (
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
)
```

修改仓库根 `go.work`，在 `use (` 块中 `./flow/server` 之后加 `./flow/otel`：

```
go 1.25.7

use (
	./flow/api
	./flow/client
	./flow/server
	./flow/otel
	./gonotelm
	./multimodal
)
```

- [ ] **Step 2: 创建 option.go**

```go
package otel

type Protocol string

const (
	ProtocolGRPC Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
)

type options struct {
	serviceName  string
	endpoint     string
	protocol     Protocol
	samplerRatio *float64
}

type Option func(*options)

func WithServiceName(name string) Option {
	return func(o *options) { o.serviceName = name }
}

func WithEndpoint(endpoint string) Option {
	return func(o *options) { o.endpoint = endpoint }
}

func WithProtocol(p Protocol) Option {
	return func(o *options) { o.protocol = p }
}

func WithSamplerRatio(ratio float64) Option {
	return func(o *options) { o.samplerRatio = &ratio }
}
```

- [ ] **Step 3: 创建 env.go**

```go
package otel

import (
	"os"
	"strings"
)

const (
	envOTELSDKDisabled          = "OTEL_SDK_DISABLED"
	envOTELExporterOTLPProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"
)

func sdkDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(envOTELSDKDisabled)), "true")
}

func protocolFromEnv() (Protocol, bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envOTELExporterOTLPProtocol)))
	switch v {
	case string(ProtocolGRPC), string(ProtocolHTTP):
		return Protocol(v), true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: 创建 otel.go**

```go
package otel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

var (
	mu          sync.Mutex
	initialized bool
	provider    *sdktrace.TracerProvider
)

// Init 初始化 OpenTelemetry SDK 并设置全局 TracerProvider 与 Propagator。
// 环境变量优先（OTEL_SERVICE_NAME / OTEL_EXPORTER_OTLP_ENDPOINT /
// OTEL_EXPORTER_OTLP_PROTOCOL / OTEL_TRACES_SAMPLER 由 SDK 原生支持），
// Option 显式覆盖。幂等：已初始化时后续调用为 no-op。
func Init(ctx context.Context, opts ...Option) error {
	if sdkDisabled() {
		slog.InfoContext(ctx, "[otel] init skipped: OTEL_SDK_DISABLED=true")
		return nil
	}

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	exporter, err := newExporter(ctx, o)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		slog.InfoContext(ctx, "[otel] already initialized, skip")
		return nil
	}

	var attrs []attribute.KeyValue
	if o.serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(o.serviceName))
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, attrs...)

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	}
	// 仅当显式指定采样率时覆盖环境变量；否则 OTEL_TRACES_SAMPLER 由 SDK 原生处理
	if o.samplerRatio != nil {
		tpOpts = append(tpOpts, sdktrace.WithSampler(
			sdktrace.ParentBased(sdktrace.TraceIDRatioBased(*o.samplerRatio)),
		))
	}

	provider = sdktrace.NewTracerProvider(tpOpts...)
	initialized = true

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.InfoContext(ctx, "[otel] tracer provider initialized",
		slog.String("serviceName", o.serviceName),
		slog.String("protocol", string(o.protocol)),
		slog.String("endpoint", o.endpoint),
	)
	return nil
}

// Shutdown 刷新并关闭 TracerProvider。幂等，可重入（之后可再次 Init）。
func Shutdown(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	if provider == nil {
		return
	}
	if err := provider.Shutdown(ctx); err != nil {
		slog.ErrorContext(ctx, "[otel] shutdown failed", slog.Any("err", err))
		return
	}
	slog.InfoContext(ctx, "[otel] shutdown complete")
	provider = nil
	initialized = false
}

// newExporter 按 protocol 创建 OTLP exporter。protocol 优先级：Option > OTEL_EXPORTER_OTLP_PROTOCOL > grpc。
// endpoint 为空时由 exporter 原生读取 OTEL_EXPORTER_OTLP_ENDPOINT。
func newExporter(ctx context.Context, o *options) (sdktrace.SpanExporter, error) {
	protocol := o.protocol
	if protocol == "" {
		protocol = ProtocolGRPC
		if p, ok := protocolFromEnv(); ok {
			protocol = p
		}
	}

	switch protocol {
	case ProtocolGRPC:
		var opts []otlptracegrpc.Option
		if o.endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(o.endpoint))
		}
		return otlptracegrpc.New(ctx, opts...)
	case ProtocolHTTP:
		var opts []otlptracehttp.Option
		if o.endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(o.endpoint))
		}
		return otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported otlp protocol: %q", protocol)
	}
}
```

- [ ] **Step 5: 创建 env_test.go**

```go
package otel

import "testing"

func TestSdkDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	if !sdkDisabled() {
		t.Fatal("expected sdkDisabled true for 'true'")
	}
	t.Setenv("OTEL_SDK_DISABLED", "TRUE")
	if !sdkDisabled() {
		t.Fatal("expected sdkDisabled true for 'TRUE'")
	}
	t.Setenv("OTEL_SDK_DISABLED", "false")
	if sdkDisabled() {
		t.Fatal("expected sdkDisabled false for 'false'")
	}
	t.Setenv("OTEL_SDK_DISABLED", "")
	if sdkDisabled() {
		t.Fatal("expected sdkDisabled false when unset")
	}
}

func TestProtocolFromEnv(t *testing.T) {
	cases := []struct {
		env  string
		want Protocol
		ok   bool
	}{
		{"grpc", ProtocolGRPC, true},
		{"GRPC", ProtocolGRPC, true},
		{"http", ProtocolHTTP, true},
		{"", "", false},
		{"unsupported", "", false},
	}
	for _, c := range cases {
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", c.env)
		got, ok := protocolFromEnv()
		if ok != c.ok || got != c.want {
			t.Fatalf("protocolFromEnv(%q) = %q, %v; want %q, %v", c.env, got, ok, c.want, c.ok)
		}
	}
}
```

- [ ] **Step 6: 创建 otel_test.go（含 resetState 辅助，保证用例间状态隔离）**

```go
package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	noop "go.opentelemetry.io/otel/trace/noop"
)

func resetState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
}

func TestInitDisabled(t *testing.T) {
	resetState(t)
	t.Setenv("OTEL_SDK_DISABLED", "true")
	if err := Init(context.Background()); err != nil {
		t.Fatalf("Init should not error when disabled, got %v", err)
	}
	if provider != nil {
		t.Fatal("provider should be nil when disabled")
	}
}

func TestInitSetsGlobalProvider(t *testing.T) {
	resetState(t)
	if err := Init(context.Background(), WithServiceName("test-svc")); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if provider == nil {
		t.Fatal("provider should be set")
	}
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatal("global tracer provider is not the sdk provider")
	}
}

func TestInitIdempotent(t *testing.T) {
	resetState(t)
	if err := Init(context.Background(), WithServiceName("a")); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	if err := Init(context.Background(), WithServiceName("b")); err != nil {
		t.Fatalf("second Init should be no-op, got %v", err)
	}
}

func TestShutdownAllowsReinit(t *testing.T) {
	resetState(t)
	if err := Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	Shutdown(context.Background())
	if provider != nil {
		t.Fatal("provider should be nil after shutdown")
	}
	if err := Init(context.Background()); err != nil {
		t.Fatalf("re-Init after shutdown failed: %v", err)
	}
}
```

- [ ] **Step 7: 运行测试**

Run（workdir `flow/otel`）: `go mod tidy && go test ./...`
Expected: `ok  github.com/gonotelm-lab/flow/otel`，全部 PASS

- [ ] **Step 8: 提交**

```bash
git add go.work flow/otel
git commit -m "feat(otel): add flow/otel module with env-standard otel init"
```

---

### Task 2: server TOML 配置与 app 生命周期

**Files:**
- Modify: `flow/server/internal/config/config.go`
- Modify: `flow/server/etc/conf.toml.tpl`
- Modify: `flow/server/internal/app/app.go`
- Create: `flow/server/internal/config/config_otel_test.go`

**Interfaces:**
- Consumes: Task 1 的 `flowotel.Init` / `flowotel.Shutdown` / `WithServiceName` / `WithEndpoint` / `WithProtocol` / `WithSamplerRatio` / `Protocol`（import 别名 `flowotel "github.com/gonotelm-lab/flow/otel"`）
- Produces: `config.Conf.Otel *OtelConfig`（nil 或 Enabled=false 时跳过 Init）

- [ ] **Step 1: 写失败测试 config_otel_test.go**

```go
package config

import "testing"

func TestOtelConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *OtelConfig
		wantErr bool
	}{
		{"nil", nil, false},
		{"disabled ignores bad protocol", &OtelConfig{Enabled: false, Protocol: "udp"}, false},
		{"valid grpc", &OtelConfig{Enabled: true, Protocol: "grpc", SamplerRatio: 1}, false},
		{"valid http", &OtelConfig{Enabled: true, Protocol: "http", SamplerRatio: 0.5}, false},
		{"valid empty protocol", &OtelConfig{Enabled: true, SamplerRatio: 0.5}, false},
		{"valid zero ratio", &OtelConfig{Enabled: true, Protocol: "grpc"}, false},
		{"bad protocol", &OtelConfig{Enabled: true, Protocol: "udp", SamplerRatio: 1}, true},
		{"ratio too high", &OtelConfig{Enabled: true, Protocol: "grpc", SamplerRatio: 1.5}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run（workdir `flow/server`）: `go test ./internal/config/ -run TestOtelConfigValidate`
Expected: FAIL — `OtelConfig` 未定义，编译错误

- [ ] **Step 3: config.go 增加 OtelConfig**

在 `type Config struct` 中 `ApiServer *ApiServer \`toml:"apiServer"\`` 之后加一行：

```go
	Otel *OtelConfig `toml:"otel"`
```

在文件末尾追加（`Validate()` 方法之后）：

```go
type OtelConfig struct {
	Enabled      bool    `toml:"enabled"`
	ServiceName  string  `toml:"serviceName"`
	Endpoint     string  `toml:"endpoint"`
	Protocol     string  `toml:"protocol"`
	SamplerRatio float64 `toml:"samplerRatio"`
}

func (cfg *OtelConfig) Validate() error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	switch cfg.Protocol {
	case "", "grpc", "http":
	default:
		return fmt.Errorf("otel.protocol must be one of grpc, http, got %q", cfg.Protocol)
	}
	if cfg.SamplerRatio < 0 || cfg.SamplerRatio > 1 {
		return fmt.Errorf("otel.samplerRatio must be in [0, 1], got %v", cfg.SamplerRatio)
	}
	return nil
}
```

在 `Config.Validate()` 中 `Registry` 校验之后追加：

```go
	if err := cfg.Otel.Validate(); err != nil {
		return fmt.Errorf("otel validate failed: %w", err)
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run（workdir `flow/server`）: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: conf.toml.tpl 增加 [otel] 段**

`flow/server/etc/conf.toml.tpl` 末尾追加：

```toml
[otel]
enabled = ${FLOW_OTEL_ENABLED:-false}
serviceName = "${FLOW_OTEL_SERVICE_NAME:-flow-server}"
endpoint = "${FLOW_OTEL_ENDPOINT:-}"
protocol = "${FLOW_OTEL_PROTOCOL:-}"
samplerRatio = ${FLOW_OTEL_SAMPLER_RATIO:-1.0}
```

- [ ] **Step 6: app.go 接入生命周期**

`flow/server/internal/app/app.go`：
- import 块增加 `flowotel "github.com/gonotelm-lab/flow/otel"` 和 `"time"`
- `bootstrap()` 第一行调用 `a.initTelemetry()`
- 新增方法：

```go
func (a *App) initTelemetry() {
	oc := config.Conf.Otel
	if oc == nil || !oc.Enabled {
		return
	}

	var opts []flowotel.Option
	if oc.ServiceName != "" {
		opts = append(opts, flowotel.WithServiceName(oc.ServiceName))
	}
	if oc.Endpoint != "" {
		opts = append(opts, flowotel.WithEndpoint(oc.Endpoint))
	}
	if oc.Protocol != "" {
		opts = append(opts, flowotel.WithProtocol(flowotel.Protocol(oc.Protocol)))
	}
	if oc.SamplerRatio > 0 {
		opts = append(opts, flowotel.WithSamplerRatio(oc.SamplerRatio))
	}

	// 初始化失败不中断启动，仅降级为无追踪
	if err := flowotel.Init(a.rootCtx, opts...); err != nil {
		slog.WarnContext(a.rootCtx, "[otel] init failed, tracing disabled", slog.Any("err", err))
		return
	}
	slog.InfoContext(a.rootCtx, "[otel] tracing enabled")
}
```

- `close()` 中在 `a.rootCancel()` 之后追加（所有组件停用后再 flush）：

```go
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flowotel.Shutdown(shutdownCtx)
```

- [ ] **Step 7: 依赖与编译**

Run（workdir `flow/server`）: `go mod tidy && go build ./...`
Expected: 编译通过（go.mod 自动加入 flow/otel 依赖与 `replace github.com/gonotelm-lab/flow/otel => ../otel`）

- [ ] **Step 8: 提交**

```bash
git add flow/server
git commit -m "feat(server): add otel config and lifecycle"
```

---

### Task 3: server gRPC / gateway 插桩 + 集成测试

**Files:**
- Modify: `flow/server/internal/endpoint/apiserver.go`
- Modify: `flow/server/internal/endpoint/adminserver.go`
- Create: `flow/server/internal/endpoint/otel_integration_test.go`

**Interfaces:**
- Consumes: Task 1 的依赖版本（otelgrpc v0.69.0）；现有 `interceptor.UnaryServerInterceptor()` 等不变
- Produces: 无（纯插桩）

- [ ] **Step 1: 写失败集成测试 otel_integration_test.go**

```go
package endpoint

import (
	"context"
	"net"
	"testing"

	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	noop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type stubTaskService struct {
	taskv1.UnimplementedTaskServiceServer
}

func (s *stubTaskService) Submit(ctx context.Context, req *taskv1.SubmitRequest) (*taskv1.SubmitResponse, error) {
	return &taskv1.SubmitResponse{}, nil
}

// TestGrpcTracingPropagation 验证 client 端 span -> server 端 span 的父子链路与 traceparent 透传。
func TestGrpcTracingPropagation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	taskv1.RegisterTaskServiceServer(server, &stubTaskService{})
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := taskv1.NewTaskServiceClient(conn)
	tracer := tp.Tracer("endpoint.test")

	ctx, parent := tracer.Start(context.Background(), "parent")
	_, err = client.Submit(ctx, &taskv1.SubmitRequest{Namespace: "ns", TaskType: "t"})
	require.NoError(t, err)
	parent.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 3, "expect parent + client + server spans")

	var parentSpan, clientSpan, serverSpan tracetest.SpanStub
	for _, s := range spans {
		switch {
		case s.Name == "parent":
			parentSpan = s
		case s.Name == "api.task.v1.TaskService/Submit" && s.Parent.SpanID() == parentSpan.SpanContext.SpanID():
			clientSpan = s
		case s.Name == "api.task.v1.TaskService/Submit" && s.Parent.SpanID() == clientSpan.SpanContext.SpanID():
			serverSpan = s
		}
	}

	require.NotZero(t, parentSpan.SpanContext.SpanID(), "parent span missing")
	require.NotZero(t, clientSpan.SpanContext.SpanID(), "client span missing")
	require.NotZero(t, serverSpan.SpanContext.SpanID(), "server span missing")
	require.Equal(t, oteltrace.SpanKindClient, clientSpan.SpanKind)
	require.Equal(t, oteltrace.SpanKindServer, serverSpan.SpanKind)
	require.Equal(t, clientSpan.SpanContext.TraceID(), serverSpan.SpanContext.TraceID(), "same trace id")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run（workdir `flow/server`）: `go test ./internal/endpoint/ -run TestGrpcTracingPropagation`
Expected: FAIL — 无传播时只有 client span，server 侧无 span，`require.Len` 报错

- [ ] **Step 3: apiserver.go 加 server stats handler**

`flow/server/internal/endpoint/apiserver.go`：
- import 增加 `"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"`
- `grpc.NewServer(...)` 调用改为：

```go
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		interceptor.UnaryServerInterceptor(),
		interceptor.StreamServerInterceptor(),
	)
```

- [ ] **Step 4: adminserver.go 加 server stats handler + gateway 插桩**

`flow/server/internal/endpoint/adminserver.go`：
- import 增加 `"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"` 与 `"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"`
- `grpc.NewServer(...)` 调用改为：

```go
	s.adminGrpcServer = grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		interceptor.UnaryServerInterceptor(),
	)
```

- `grpc.NewClient(...)` 调用改为（增加 stats handler，HTTP span 经 context 注入 traceparent 到 gRPC metadata）：

```go
	conn, err := grpc.NewClient(
		fmt.Sprintf("unix:///%s", unixSocketPath),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
```

- `s.httpServer` 的 Handler 改为：

```go
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Http.Port),
		Handler: otelhttp.NewHandler(mux, "flow.admin.http"),
	}
```

- [ ] **Step 5: 运行测试确认通过**

Run（workdir `flow/server`）: `go test ./internal/endpoint/ -run TestGrpcTracingPropagation -v`
Expected: PASS — 3 个 span，client→server 父子链正确

- [ ] **Step 6: 依赖与全量回归**

Run（workdir `flow/server`）: `go mod tidy && go test ./...`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add flow/server
git commit -m "feat(server): instrument grpc servers and admin gateway with otel"
```

---

### Task 4: GORM SQL 插桩（官方插件）

**Files:**
- Modify: `flow/server/pkg/sql/pgsql.go`
- Create: `flow/server/pkg/sql/otel_test.go`

**Interfaces:**
- Consumes: `gorm.io/plugin/opentelemetry/tracing` v0.1.16
- Produces: 无

- [ ] **Step 1: 写失败测试 otel_test.go**

```go
package sql

import (
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

// TestRegisterOtelTracingPlugin 验证 tracing 插件注册不依赖真实数据库连接（gorm 惰性连接）。
func TestRegisterOtelTracingPlugin(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=u password=p dbname=d sslmode=disable",
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm failed: %v", err)
	}

	if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		t.Fatalf("register tracing plugin failed: %v", err)
	}

	if _, ok := db.Plugins["otelgorm"]; !ok {
		t.Fatal("tracing plugin not registered under name otelgorm")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run（workdir `flow/server`）: `go test ./pkg/sql/ -run TestRegisterOtelTracingPlugin`
Expected: FAIL — `db.Plugins["otelgorm"]` 不存在

- [ ] **Step 3: pgsql.go 注册插件**

`flow/server/pkg/sql/pgsql.go`：
- import 增加 `"gorm.io/plugin/opentelemetry/tracing"`
- `OpenPgSqlWithLogger` 中 `gorm.Open(...)` 成功之后、连接池配置之前插入：

```go
	if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		return nil, fmt.Errorf("register otel tracing plugin failed: %w", err)
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run（workdir `flow/server`）: `go test ./pkg/sql/ -run TestRegisterOtelTracingPlugin`
Expected: PASS

- [ ] **Step 5: 依赖与全量回归**

Run（workdir `flow/server`）: `go mod tidy && go test ./...`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add flow/server
git commit -m "feat(server): instrument gorm sql with official otel plugin"
```

---

### Task 5: client SDK 插桩（可选注入）+ 示例

**Files:**
- Modify: `flow/client/task/task.go`
- Modify: `flow/client/worker/worker.go`
- Modify: `flow/client/example/worker/raw/main.go`

**Interfaces:**
- Consumes: otelgrpc v0.69.0、flow/otel（示例用）
- Produces: SDK 默认 dial 时携带 `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`；宿主设置全局 provider 后自动参与追踪，否则 no-op

- [ ] **Step 1: task.go 加默认 stats handler**

`flow/client/task/task.go`：
- import 增加 `"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"`
- `New()` 的 `baseOpts` 改为（用户 opts 在后，可覆盖）：

```go
	baseOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
```

- [ ] **Step 2: worker.go 加默认 stats handler**

`flow/client/worker/worker.go`：
- import 增加 `"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"`
- `New()` 的 `baseOpts` 改为：

```go
	baseOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
```

- [ ] **Step 3: 运行现有测试确认无回归（no-op 行为）**

Run（workdir `flow/client`）: `go mod tidy && go test ./...`
Expected: 全部 PASS（现有测试无全局 provider，追踪自动 no-op，行为不变）

- [ ] **Step 4: 更新 raw worker 示例**

`flow/client/example/worker/raw/main.go` 整体替换为：

```go
// Raw worker：使用底层 []byte handler，演示 OkResult / ErrorResult 约定。
//
//	payload 以 "fail:" 开头 → ErrorResult
//	其他 payload → OkResult，内容为 "echo:" + payload
//
//	go run ./example/raw -addr localhost:7091 -namespace demo -task-type raw
//
// 追踪：默认读取 OTEL_* 环境变量（如 OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317），
// 未配置时 SDK 初始化成功但不导出；也可通过 OTEL_SDK_DISABLED=true 完全关闭。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	flowotel "github.com/gonotelm-lab/flow/otel"
	"github.com/gonotelm-lab/flow/client/worker"
)

func main() {
	addr := flag.String("addr", "localhost:7091", "Flow worker gRPC 地址")
	namespace := flag.String("namespace", "demo", "任务 namespace")
	taskType := flag.String("task-type", "raw", "任务类型")
	name := flag.String("name", "raw-worker", "worker 名称")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := flowotel.Init(context.Background()); err != nil {
		logger.Error("otel init failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		flowotel.Shutdown(ctx)
	}()

	client, err := worker.New(*addr, worker.Config{
		Namespace: *namespace,
		TaskType:  *taskType,
		Name:      *name,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("dial flow server failed", "addr", *addr, "err", err)
		os.Exit(1)
	}

	client.Handle(func(ctx context.Context, payload []byte) (worker.Result, error) {
		text := string(payload)
		logger.Info("handling raw task", "payload", text)

		if strings.HasPrefix(text, "fail:") {
			return worker.ErrorResult{Data: []byte(text)}, nil
		}
		return worker.OkResult{Data: []byte("echo:" + text)}, nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("raw worker starting", "addr", *addr, "namespace", *namespace, "task_type", *taskType)
	if err := client.Run(ctx); err != nil {
		logger.Error("worker stopped with error", "err", err)
		os.Exit(1)
	}
	logger.Info("worker stopped")
}
```

- [ ] **Step 5: 编译示例并全量回归**

Run（workdir `flow/client`）: `go mod tidy && go build ./... && go test ./...`
Expected: 编译通过，测试全部 PASS

- [ ] **Step 6: 提交**

```bash
git add flow/client
git commit -m "feat(client): instrument sdk with otelgrpc client handler and update example"
```

---

### Task 6: 文档

**Files:**
- Create: `flow/docs/tracing.md`

**Interfaces:**
- Consumes: 前面所有任务的最终行为
- Produces: 无

- [ ] **Step 1: 创建 flow/docs/tracing.md**

```markdown
# Flow Tracing（OpenTelemetry）

Flow 使用 OpenTelemetry 最新标准（Go v1.44.0）实现端到端链路追踪（仅 Traces）。
链路贯穿：SDK client（task/worker）→ gRPC server → GORM SQL。

## 环境变量（标准 OTEL_*，优先）

| 变量 | 说明 | 默认 |
|------|------|------|
| `OTEL_SERVICE_NAME` | 服务名（resource service.name） | 空 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP Collector 地址 | grpc: `localhost:4317` / http: `localhost:4318` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` / `http` | `grpc` |
| `OTEL_TRACES_SAMPLER` | `always_on` / `always_off` / `traceidratio`（可加 `parentbased_` 前缀） | `parentbased_always_on` |
| `OTEL_TRACES_SAMPLER_ARG` | traceidratio 的比例参数 | - |
| `OTEL_SDK_DISABLED` | `true` 时完全关闭 SDK | `false` |

## Server 配置

`conf.toml.tpl` 的 `[otel]` 段（可选，显式值覆盖环境变量；`enabled=false` 跳过）：

```toml
[otel]
enabled = true
serviceName = "flow-server"     # 覆盖 OTEL_SERVICE_NAME
endpoint = "localhost:4317"     # 覆盖 OTEL_EXPORTER_OTLP_ENDPOINT
protocol = "grpc"               # grpc | http，覆盖 OTEL_EXPORTER_OTLP_PROTOCOL
samplerRatio = 1.0              # 覆盖 OTEL_TRACES_SAMPLER（traceidratio）
```

插桩范围：

- gRPC api/admin server：`otelgrpc.NewServerHandler()`（stats handler）
- admin HTTP gateway：`otelhttp.NewHandler` + `otelgrpc.NewClientHandler()`（HTTP → gRPC 三段 span 串链）
- GORM SQL：`gorm.io/plugin/opentelemetry/tracing.NewPlugin()`（db.system/db.statement 等 semconv 属性）

初始化失败仅告警降级，不影响服务启动。

## Client SDK 接入（可选注入）

`flow/client/task` 与 `flow/client/worker` 的 `New()` 默认携带
`otelgrpc.NewClientHandler()`。SDK 不强制初始化——宿主进程调用 `flow/otel.Init` 后自动参与追踪：

```go
import flowotel "github.com/gonotelm-lab/flow/otel"

func main() {
	ctx := context.Background()
	if err := flowotel.Init(ctx); err != nil {
		log.Fatal(err)
	}
	defer flowotel.Shutdown(ctx)

	client, err := task.New("localhost:7091")
	// ...
}
```

未 Init 时全局 provider 为 no-op，SDK 行为与未接入追踪前完全一致（零成本）。

完整示例见 `flow/client/example/worker/raw/main.go`。

## 验证

1. 启动 OTLP Collector（或 Jaeger/otel-demo）暴露 4317
2. 配置 server 或环境变量，重启 flow-server
3. 提交任务并观察：`api.task.v1.TaskService/Submit`（client span）→ 同名 server span → gRPC span → SQL span 形成完整链路
```

- [ ] **Step 2: 提交**

```bash
git add flow/docs/tracing.md
git commit -m "docs: add flow tracing guide"
```

---

## 验收清单（跨任务）

- [ ] `flow/otel`：`go test ./...` 全绿（env 解析 / 禁用跳过 / 幂等 / 可重入）
- [ ] `flow/server`：`go test ./...` 全绿，含 `TestGrpcTracingPropagation`（client→server 父子 span）与 `TestRegisterOtelTracingPlugin`
- [ ] `flow/client`：`go test ./...` 全绿（无 provider 时行为不变）
- [ ] `go build ./...`：flow/otel、flow/server、flow/client 三个模块均编译通过
- [ ] `go.work` 包含 `./flow/otel`；server/client go.mod 含 flow/otel 依赖与 replace
