# Flow OpenTelemetry Tracing 设计

日期：2026-08-03
状态：已批准
模块：flow（api / client / server / otel）

## 背景与目标

Flow 分布式任务队列需要端到端链路追踪：SDK client 发起请求 → server 处理，在任意一层都能观测。采用 OpenTelemetry 最新标准（Go），信号仅限 **Traces**（不做 Metrics/Logs）。

范围：

- 新建共享模块 `flow/otel`：统一 SDK 初始化（env 标准 + Option 覆盖）
- `flow/server`：gRPC 服务端插桩（stats handler 风格）、admin HTTP gateway（otelhttp）、GORM SQL 插桩（官方插件）
- `flow/client`：task / worker SDK 默认带 otelgrpc client stats handler（可选注入，无全局 provider 时 no-op）
- 内部后台组件（sweeper/watcher/mender）**不**插桩

## 1. flow/otel 共享模块

新模块 `github.com/gonotelm-lab/flow/otel`，加入 `go.work`，server 与 client 均依赖。

### API

```go
package otel

func Init(ctx context.Context, opts ...Option) error
func Shutdown(ctx context.Context)

type Option func(*options)
func WithServiceName(name string) Option
func WithEndpoint(endpoint string) Option
func WithProtocol(p Protocol) Option            // grpc | http
func WithSamplerRatio(ratio float64) Option
func WithResourceAttributes(attrs []attribute.KeyValue) Option
```

### 行为

- **env 标准优先**：读取 `OTEL_SERVICE_NAME`、`OTEL_EXPORTER_OTLP_ENDPOINT`、`OTEL_EXPORTER_OTLP_PROTOCOL`(grpc/http)、`OTEL_TRACES_SAMPLER`(always_on/off/traceidratio)、`OTEL_SDK_DISABLED`（跳过初始化）
- **Option 覆盖** env：server 通过 TOML 配置注入
- 组合：resource（semconv v1.41.0 的 service.name / sdk.name / sdk.version / sdk.language）+ ParentBased(TraceIDRatioBased) sampler + OTLP trace exporter（按 protocol 选 grpc/http）+ batch span processor + TraceContext/Baggage 复合传播器，设置全局 TracerProvider 与 Propagator
- `Init` / `Shutdown` 幂等（sync.Once）
- `OTEL_SDK_DISABLED` 或未配置 endpoint 时跳过（no-op，不报错）

## 2. flow/server 插桩

### 配置（可选，覆盖 env）

`internal/config/config.go` 新增：

```toml
[otel]
enabled = true
serviceName = "flow-server"
endpoint = "localhost:4317"
protocol = "grpc"
samplerRatio = 1.0
```

`Config` 增加 `Otel *OtelConfig`。`enabled=false` 或段缺失 → 跳过 Init。

### 生命周期

`internal/app/app.go`：`bootstrap()` 中调用 `otel.Init`（失败仅 slog.Warn，不中断启动）；`close()` 中调用 `otel.Shutdown`。

### gRPC 服务端插桩

- `internal/endpoint/apiserver.go`：`grpc.NewServer` 增加 `grpc.StatsHandler(otelgrpc.NewServerHandler())`
- `internal/endpoint/adminserver.go`：
  - admin gRPC server 同样加 `otelgrpc.NewServerHandler()`
  - HTTP gateway：`otelhttp.NewHandler(mux, "flow.admin.http")` 包住 handler
  - `proxyConn` dial 加 `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`
  - 效果：HTTP → gateway 转发 → admin gRPC 三段 span 串成一条链

### GORM SQL 插桩

`flow/server/pkg/sql/pgsql.go` 的 `OpenPgSql` / `OpenPgSqlWithLogger` 中注册官方插件：

```go
import "gorm.io/plugin/opentelemetry"
db.Use(opentelemetry.NewPlugin())
```

无全局 provider 时 no-op。不引第三方 otelgorm（过时）。

### 不插桩

sweeper / watcher / taskmender 等后台循环。

## 3. flow/client 插桩（可选注入）

### task SDK

`flow/client/task/task.go` `New()`：`baseOpts` 先追加 `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`，再追加用户 opts（用户可覆盖）。

### worker SDK

`flow/client/worker/worker.go` `New()`：同样追加。worker 内部 runtime（poll/heartbeat/report）与 task 调用均携带 ctx，链路自动透传，runtime 代码零改动。

### 启用方式

SDK 不强制初始化。宿主程序调用 `flow/otel.Init` 设置全局 provider + propagator 后自动参与追踪；不 Init 则一切 no-op（otel 全局默认即 no-op provider / 空传播器）。

### 示例

`flow/client/example/worker/raw/main.go` 演示 Init → Run → Shutdown，展示 traceparent 从 client 传到 server。

## 4. 依赖版本

| 依赖 | 版本 |
|------|------|
| `go.opentelemetry.io/otel` | v1.44.0 |
| `go.opentelemetry.io/otel/sdk` | v1.44.0 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | v1.44.0 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | v1.44.0 |
| `go.opentelemetry.io/otel/semconv/v1.41.0` | v1.41.0（随 otel v1.44.0 内置，无需单独依赖） |
| `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` | v0.69.0 |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | v0.69.0 |
| `gorm.io/plugin/opentelemetry` | v0.1.16 |

新代码统一 semconv v1.41.0（最新可用版本，与 gonotelm 一致），不混用其他版本。`gonotelm/pkg/trace` 保持原样不动。

## 5. 构建变更

- `go.work` 增加 `./flow/otel`
- `flow/otel/go.mod`：module `github.com/gonotelm-lab/flow/otel`
- `flow/server`、`flow/client` 的 go.mod 增加 flow/otel 依赖与 `replace github.com/gonotelm-lab/flow/otel => ../otel`

## 6. 测试

- flow/otel：env 解析（service name / protocol / sampler）、`OTEL_SDK_DISABLED` 跳过、Init 幂等
- server：bufconn 集成测试，tracetest exporter 断言 client→server parent-child span 与 traceparent 透传
- client：确认无全局 provider 时行为不变（no-op）
- 回归：`go test ./...`（server / client / api）

## 7. 文档

`flow/docs` 新增 `tracing.md`：OTEL_* 环境变量、server TOML 配置、client 接入方式。

## 不做的事

- Metrics / Logs 信号
- 后台组件循环插桩
- 修改 gonotelm 模块
