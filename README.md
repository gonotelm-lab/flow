<a id="readme-top"></a>

<h1 align="center">
  <img src="web/public/brand/flow-lockup.svg" alt="Flow" width="204" />
</h1>

<p align="center">
  <a href="https://github.com/gonotelm-lab/flow/releases"><img src="https://img.shields.io/github/v/release/gonotelm-lab/flow" alt="Release"></a>
  <a href="https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml"><img src="https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml/badge.svg" alt="Publish"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/gonotelm-lab/flow" alt="License"></a>
</p>

分布式任务队列与定时回调引擎。server 暴露 gRPC 与 Admin HTTP API，Go SDK 负责提交任务、运行 worker、注册定时回调，另有一个 React 运维后台。

## 特性

- **任务队列** — namespace 隔离，支持延迟执行、重试上限、取消与删除。
- **定时回调** — 与任务独立的一等资源：一次性或 cron 触发，到期由 server 主动 POST，带请求签名；失败进入重试与死信，可人工重投。
- **Worker** — 长轮询取任务、心跳上报、并发控制；节点失联后由 server 检测并回收。
- **水平扩展** — 多实例注册与分片，同一任务不会被两个实例同时投递。
- **可观测** — OpenTelemetry tracing（gRPC / HTTP / SQL）。
- **运维后台** — 任务、定时任务与死信、worker、namespace / API Key、操作审计。

## 快速开始

需要 Go 1.25.4+ 与 PostgreSQL 18；跑前端另需 Node 22+。

```bash
cp server/.env.example server/.env   # 按需修改数据库连接
make run-server                      # Admin HTTP :7090，gRPC :7091
```

默认 `FLOW_DB_AUTO_INIT=true`：目标库不存在时会自动创建（要求账号有 `CREATEDB` 权限），随后执行内嵌迁移。

管理后台：

```bash
make web-install
make run-web                         # http://localhost:7089
```

Docker：

```bash
make docker-build
docker run --rm -p 7090:7090 -p 7091:7091 \
  -e FLOW_DB_HOST=host.docker.internal -e FLOW_DB_NAME=flowdb \
  ghcr.io/gonotelm-lab/flow/server:latest
```

> web 镜像只是静态站点，`/api/admin` 需由外部网关转发到 server 的 7090。

## 用法

### 提交与消费任务

```go
// import task "github.com/gonotelm-lab/flow/client/task/v2"
tasks, _ := task.New("localhost:7091", task.Config{Namespace: "demo", APIKey: apiKey})
t, _ := tasks.Submit(ctx, "echo", []byte(`{"message":"hi"}`), task.WithMaxRetry(3))

w, _ := worker.New("localhost:7091", worker.Config{
    Namespace: "demo", TaskType: "echo", MaxConcurrency: 4, APIKey: apiKey,
})
worker.RegisterTyped[echoInput, echoOutput](w, func(ctx context.Context, in echoInput) (echoOutput, error) {
    return echoOutput{Message: strings.ToUpper(in.Message)}, nil
})
w.Run(ctx)
```

`apiKey` 是 namespace 的 api_key：服务端开启鉴权时必填，关闭时留空即可（SDK 不会发出凭据头）。
`client/task`（v1，每次调用传 namespace）已废弃，保留仅为升级缓冲。

可运行示例见 [`client/example`](client/example)。

### 注册定时回调

```go
s, _ := schedule.New("localhost:7091", schedule.Config{Namespace: "demo", APIKey: apiKey})

s.Register(ctx, schedule.OneShot(time.Now().Add(10*time.Minute), hookURL, payload))
s.Register(ctx, schedule.Cron("*/5 * * * *", "Asia/Shanghai", hookURL, payload))

s.Stop(ctx, id)   // 单向停止：此后不再触发，不可恢复
```

- cron 是标准 5 字段 `分 时 日 月 周`，最小粒度 1 分钟；方言为 `robfig/cron`，星期 `0-6` 且 `0` 为周日，与 Quartz 不同。
- 定义注册后不可修改，要改就用同一个幂等键重新注册一条；SDK 未显式给幂等键时会自动生成。

### 接收回调

到期后 server 向你注册的 URL 发一次 POST：body 是注册时传入的 payload，请求头带签名与触发信息。

投递返回 2xx 即成功，否则重投。

用 SDK 一行验签：

```go
verifier, _ := schedule.New("", schedule.Config{Namespace: ns, APIKey: apiKey})
if err := verifier.VerifyRequest(r); err != nil {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
```

可运行的接收方见 [`client/example/schedule-receiver`](client/example/schedule-receiver)。

### 开启 API Key 鉴权

客户端面（7091 的 task / worker / schedule）支持基于 namespace api_key 的鉴权，**默认关闭**：

```bash
FLOW_API_AUTH_ENABLED=true
```

打开后，每个请求都要带 `authorization: Bearer <api_key>`，且只能访问自己 namespace 的资源；
客户端把 key 填进 `Config.APIKey` 即可，缺 key 或 key 无效返回 `UNAUTHENTICATED`。

## API

gRPC（`:7091`）：

| Service | 方法 |
|---|---|
| `api.task.v1.TaskService` | `Submit` / `Get` / `Cancel` |
| `api.worker.v1.WorkerService` | `Register` / `Unregister` / `Heartbeat` / `Poll` / `Report` |
| `api.schedule.v1.ScheduleService` | `Register` / `Get` / `List` / `Stop` / `Delete` |

Admin HTTP（`:7090`，前缀 `/api/admin/v1`，gRPC-Gateway）：

| 资源 | 路由 |
|---|---|
| namespace | `/namespaces`、`/namespaces/{id}` |
| task | `/tasks`、`/tasks/{id}`、`/tasks/{id}/cancel`、`/tasks/{id}/events` |
| worker | `/workers`、`/workers/{id}` |
| schedule | `/schedules`、`/schedules/{id}`、`/schedules/{id}/stop`、`/schedules/{id}/trigger` |
| 死信 | `/schedule-dead-letters`、`/schedule-dead-letters/{id}/replay` |
| 审计 | `/audit-logs` |

消息与字段以 [`api/`](api) 下的 `.proto` 为准。

## 配置

配置来自 [`server/etc/conf.toml.tpl`](server/etc/conf.toml.tpl)（`envsubst` 展开），全部通过 `FLOW_*` 环境变量注入；完整清单见 [`server/.env.example`](server/.env.example)。

| 变量 | 默认 | 说明 |
|---|---|---|
| `FLOW_DB_DRIVER` | `pgsql` | 目前仅支持 pgsql |
| `FLOW_DB_HOST` / `FLOW_DB_PORT` | `127.0.0.1` / `5432` | 数据库地址 |
| `FLOW_DB_USER` / `FLOW_DB_PASS` / `FLOW_DB_NAME` | `postgres` / `postgres` / `flowdb` | 数据库账号与库名 |
| `FLOW_DB_AUTO_INIT` | `true` | 库缺失时自动创建并执行迁移 |
| `FLOW_API_SERVER_HTTP_PORT` | `7090` | Admin HTTP |
| `FLOW_API_SERVER_GRPC_PORT` | `7091` | gRPC |
| `FLOW_API_AUTH_ENABLED` | `false` | 客户端面（7091）的 api_key 鉴权开关；打开前先升级客户端 |
| `FLOW_WORKER_*` | — | 轮询、心跳、过期扫描、节点回收 |
| `FLOW_SCHEDULER_*` | — | 定时回调调度器；删掉整个 `[scheduler]` 段则不启动 |
| `FLOW_OTEL_ENABLED` / `FLOW_OTEL_ENDPOINT` / `FLOW_OTEL_PROTOCOL` | `false` / — | 对接 OTLP collector |

## 部署

| 端口 | 用途 |
|---|---|
| `7090` | Admin HTTP API |
| `7091` | gRPC（task / worker / schedule） |
| `7089` | 前端 dev server（仅开发） |

- 数据库 schema 用 [goose](https://github.com/pressly/goose) 迁移，SQL 内嵌在二进制里、启动时执行；多实例并发启动不会重复执行。
- 镜像在发布 Release 时由 GitHub Actions 构建并推送到 `ghcr.io/gonotelm-lab/flow/{server,web}`（linux/amd64、linux/arm64），tag 取 `vX.Y.Z` 形式的应用级 tag。
- 客户端面（7091）可开启 api_key 鉴权（`FLOW_API_AUTH_ENABLED`，默认关闭）；**admin HTTP（7090）没有任何鉴权**，回调 URL 也只做格式校验（不探测可达性，不拦截私网地址）。**必须保证 7090 不暴露到不可信网络；7091 在关闭鉴权时同样如此。**

## 开发

```bash
cd server && go build ./... && go test ./...     # 集成测试需要可用的 PostgreSQL
```

集成测试通过环境变量连库，并自建/清理临时库：

```bash
FLOW_DB_HOST=127.0.0.1 FLOW_DB_PORT=5432 FLOW_DB_USER=postgres \
FLOW_DB_PASS=postgres FLOW_DB_NAME=flowdb go test ./...
```

### 端到端测试

`client/e2e` 跑在真 server + 真库上，默认跳过（没配 `FLOW_E2E_ADDR` 就不跑），所以 `go test ./...` 不受影响。

一条命令跑完（自己起临时 PostgreSQL，鉴权开 / 关两种模式各跑一遍）：

```bash
scripts/e2e.sh --docker-db
```

用已有的 PostgreSQL 就去掉 `--docker-db`（读 `FLOW_DB_*`，默认 `127.0.0.1:5432`）。
server 日志写在 `.e2e-logs/`（已 gitignore）。CI 由 [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
调用同一个脚本，数据库来自 service 容器。

```
api/      Protobuf 定义与生成代码
client/   Go SDK：task（v2）/ worker / schedule 客户端与回调验签
server/   Go 服务：config / repository / service / instance / taskmender / endpoint
scripts/  e2e 脚本（scripts/e2e.sh）
web/      React + Vite 运维后台
```

## License

[Apache-2.0](LICENSE)
