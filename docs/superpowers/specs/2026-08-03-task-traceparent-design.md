# Task Traceparent 跨进程链路延续 设计

日期：2026-08-03
状态：已批准
模块：flow（api / otel / server / client）

## 背景与目标

Submit 时若请求 ctx 中存在 OTel span（由 otelgrpc 标准注入），将 W3C traceparent 随任务存入 tasks 表；Poll 分发任务时，worker 端据此恢复链路上下文，实现「提交方 → server → worker 处理」跨进程单条 trace 贯穿（或独立 trace + link 关联，可配置）。

## 1. 数据层（迁移需 ALTER，已有数据）

`flow/migration/pgsql18.sql` 追加（沿用 last_heartbeat_time 的 ALTER 风格）：

```sql
ALTER TABLE tasks ADD COLUMN traceparent VARCHAR(55);
COMMENT ON COLUMN tasks.traceparent IS 'task traceparent from submit context';
```

- `flow/api/schema/v1/task.proto`：`Task` 消息加 `string traceparent = 15;`（无校验），`buf generate` 重新生成 api 模块
- `flow/server/internal/repository/schema/task.go`：`Task` 结构加 `Traceparent string \`gorm:"column:traceparent"\``
- `flow/server/internal/service/task/impl.go` `toProtoTask`：透传 `Traceparent`

## 2. Server Submit 捕获（gRPC OTel 标准）

标准行为：Submit RPC 的 ctx 由 otelgrpc server handler 注入远程 span；有 span 则存储，**无 span 则忽略**（存空串，不产生任何语义）。

`flow/otel` 新增纯函数：

```go
// TraceparentFromContext 用全局 propagator 从 ctx 提取 W3C traceparent；无 span 时返回空串。
func TraceparentFromContext(ctx context.Context) string
```

- 实现：`propagation.MapCarrier{}` + `otel.GetTextMapPropagator().Inject(ctx, carrier)`，取 `carrier["traceparent"]`
- `flow/server/internal/service/task/impl.go` `Submit`：创建 task 时 `Traceparent: otel.TraceparentFromContext(ctx)`

## 3. Worker SDK 内置恢复（可配置模式）

`flow/client/worker`：

```go
type TraceMode string

const (
	TraceModeChild TraceMode = "child" // 默认：子 span 延续链路（remote parent）
	TraceModeLink  TraceMode = "link"  // 独立 trace + span link 关联
)
```

`worker.Config` 加 `TraceMode TraceMode`，空值默认 `TraceModeChild`，传入 runtime。

`flow/otel` 新增纯函数：

```go
// SpanContextFromTraceparent 解析 W3C traceparent 为远程 SpanContext；无效时 ok=false。
func SpanContextFromTraceparent(tp string) (trace.SpanContext, bool)
```

`runtime.runTask`（`flow/client/worker/internal/runtime/poll.go`）**在调用 Handler 之前**创建 span：

```go
sc, ok := flowotel.SpanContextFromTraceparent(task.GetTraceparent())
if ok {
	switch mode {
	case TraceModeChild:
		handlerCtx = trace.ContextWithRemoteSpanContext(taskCtx, sc)
	default: // link
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: sc}))
	}
}
ctx, span := tracer.Start(handlerCtx, "task.handle",
	trace.WithAttributes(
		attribute.String("task.id", taskID),
		attribute.String("namespace", ...),
		attribute.String("task_type", ...),
		attribute.Int64("worker_id", ...),
	))
// Handler(ctx, task) 在 span 内执行
// defer span.End()
```

- 无 traceparent / traceparent 无效 / 无全局 provider → 保持现状（no-op）
- Report 调用沿用 handler 的 ctx，自动成为 handler span 的子 span
- 重试场景：复用原始 traceparent，形成同一 trace 的多个分支

## 4. 测试

- `flow/otel`：`SpanContextFromTraceparent` round-trip（合法 tp → 同 trace_id/span_id；无效 tp → ok=false）；`TraceparentFromContext`（无 span → 空串；有 span → 合法 traceparent）
- `flow/client`：runtime 恢复单测（tracetest exporter）——child 模式 handler span 父链正确；link 模式 handler span 为根且含 link；无 traceparent 时无 span；TraceMode 空值默认 child
- `flow/server`：Submit 单测不依赖 DB 的纯函数部分；DB 相关沿用现有 postgres 集成测试环境

## 5. 文档

`flow/docs/tracing.md` 补充：traceparent 存储与恢复说明、`TraceMode`（child/link）配置示例。

## 不做的事

- Baggage 跨进程持久化
- task_events 记录 traceparent
- admin API 变更
