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

### 高频 RPC 过滤（poll / heartbeat）

worker 的 `Poll`（长轮询）与 `Heartbeat` 每几秒一次，若每次生成 span 会导致 trace 量爆炸。
两端（worker SDK 与 server）通过 `otelgrpc.WithFilter` 排除这两个方法：
**成功不产生 span，失败时由 worker runtime 手动记录错误 span**（`worker.poll` / `worker.heartbeat`，
带 worker.id 属性、error 状态与 `RecordError`）。其余 RPC（Register/Report/Submit 等）保持全量追踪。

## Client SDK 接入（零配置，取全局）

`flow/client/task` 与 `flow/client/worker` 的 `New()` 默认携带
`otelgrpc.NewClientHandler()`；worker 分发前自动恢复 traceparent（见下）。
SDK 与 `gorm.io/plugin/opentelemetry` 采用同样的模式：**只取全局
`otel.GetTracerProvider()` / Propagator，不初始化、不拥有生命周期**。

宿主进程自行初始化 OTel（任意实现，如 `gonotelm/pkg/trace.Init`、
官方 SDK `sdktrace.NewTracerProvider`、或其他），SDK 即自动参与：

```go
// 宿主进程自己的 OTel 初始化（示例：官方 SDK + OTLP）
exporter, _ := otlptracegrpc.New(ctx)
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
otel.SetTracerProvider(tp)
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{}, propagation.Baggage{}))

client, err := task.New("localhost:7091") // 直接使用，无需任何 flow 侧初始化
```

未初始化时全局 provider 为 no-op，SDK 行为与未接入追踪前完全一致（零成本）。

**唯一要求**：全局 propagator 需包含 W3C TraceContext（标准初始化都有），
否则提交方向（client→server 注入）无法工作。

完整示例见 `flow/client/example/worker/raw/main.go`。

### 任务链路延续（traceparent 落库 + worker 自动恢复）

Submit 时若请求 ctx 存在 span（otelgrpc 标准注入），server 将 W3C `traceparent`
随任务写入 `tasks.traceparent` 列（无 span 则忽略，存空串）。worker Poll 领取任务时，
traceparent 通过 gRPC response trailer 下发，SDK **自动**在调用 Handler 前创建
`task.handle` span（带 task.id/namespace/task_type/worker_id 属性），无需任何配置：

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

使用方只需更新 SDK 并保持宿主进程已有的 OTel 初始化（provider + W3C propagator），
即可开箱获得跨进程链路；未接入 OTel 时自动 no-op。

## 验证

1. 启动 OTLP Collector（或 Jaeger/otel-demo）暴露 4317
2. 配置 server 或环境变量，重启 flow-server
3. 提交任务并观察：`api.task.v1.TaskService/Submit`（client span）→ 同名 server span → gRPC span → SQL span 形成完整链路
