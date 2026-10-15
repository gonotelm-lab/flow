<a id="readme-top"></a>

# Flow

[![Release](https://img.shields.io/github/v/release/gonotelm-lab/flow)](https://github.com/gonotelm-lab/flow/releases)
[![Publish](https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml/badge.svg)](https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml)
[![License](https://img.shields.io/github/license/gonotelm-lab/flow)](LICENSE)

Flow 是一个分布式的**任务队列 / 工作流引擎**。它由三部分组成：

- **Server** — Go 服务，提供 gRPC API、Admin HTTP API、服务实例注册与分片、任务调度/重试/过期检测。
- **Client SDK** — Go 客户端，包含任务提交（task）、定时回调（schedule）与工作节点（worker）运行时。
- **Admin Web** — React 管理后台，用于可视化 namespace、task、worker 的运行状态。

Flow 用 namespace 隔离任务，用 worker 心跳感知节点健康，并通过 OpenTelemetry 打通全链路追踪。

## 核心能力

- **命名空间隔离** — 每个 namespace 独立管理任务与 worker，携带独立 API Key。
- **任务生命周期** — `INITED → RUNNING → DONE / FAILED`，支持延迟执行（`next_run_time`）、最大重试次数、取消与删除。
- **定时回调** — 与 Task 完全独立的 Schedule 资源：注册回调 URL 与 payload，支持一次性（绝对时刻）与 cron（标准 5 字段）两类触发，到期由 server 主动 POST。内置有界重试、死信与管理员审计日志。
- **Worker 注册与心跳** — worker 上报心跳与统计，server 侧检测过期/卡住任务并重新调度。
- **多实例与分片** — 服务实例注册到注册表，通过分片计算分配工作，支撑水平扩展。
- **可观测性** — 内建 OpenTelemetry tracing（gRPC / HTTP / GORM），可对接 OTLP collector。
- **版本化 Schema 迁移** — 基于 [goose](https://github.com/pressly/goose) 的迁移内嵌进二进制，启动时可自动建表。
- **运维后台** — 任务列表（状态筛选 + 详情 Dialog）、定时任务管理与死信处理、worker 健康、namespace / API Key 管理、操作审计日志。

## 仓库结构

```
.
├── api/                 # Protobuf 定义与生成代码（Go）
├── client/              # Go SDK：task 提交 + worker 运行时（独立 module）
├── server/              # Go 服务端
│   ├── cmd/flowserver/  # 入口
│   ├── internal/        # config / repository / service / instance / taskmender / endpoint ...
│   ├── pkg/             # 可复用包（sql、errors ...）
│   ├── migration/       # 内嵌迁移：migration/postgres/*.sql
│   └── etc/             # 配置模板 conf.toml.tpl
├── web/                 # React + Vite 管理后台
├── Makefile
└── .github/workflows/   # publish：发布 Release 时构建并推送镜像
```

## 快速开始

### 前置条件

- Go **1.25.4+**
- Node **22+**（仅前端）
- PostgreSQL **18**（使用 `uuidv7()`，见迁移）
- Docker（可选，用于构建/运行镜像）

### 本地运行

1. 准备配置。复制模板并按需修改数据库连接：

   ```bash
   cp server/.env.example server/.env
   ```

   `conf.toml.tpl` 会读取 `FLOW_*` 环境变量；未设置时使用模板默认值（`127.0.0.1:5432`、`postgres/postgres`、`flowdb`）。

2. 启动 server：

   ```bash
   make run-server        # 等价于 cd server && go run ./cmd/flowserver
   ```

   默认 `FLOW_DB_AUTO_INIT=true`：目标库不存在会自动创建，随后执行内嵌迁移建表。

3. 启动管理后台（新开一个终端）：

   ```bash
   make web-install       # 首次运行前安装依赖
   make run-web           # Vite dev server: http://localhost:7089
   ```

   开发模式下 Vite 将 `/api/admin` 代理到 `http://localhost:7090`。

### 使用 Docker

```bash
make docker-build        # 构建 server / web 镜像（默认 TAG=dev）
make docker-push         # 多架构构建并推送到 ghcr.io/gonotelm-lab/flow/{server,web}
```

运行 server 容器（通过环境变量注入数据库连接）：

```bash
docker run --rm -p 7090:7090 -p 7091:7091 \
  -e FLOW_DB_HOST=host.docker.internal -e FLOW_DB_PORT=5432 \
  -e FLOW_DB_USER=postgres -e FLOW_DB_PASS=postgres -e FLOW_DB_NAME=flowdb \
  ghcr.io/gonotelm-lab/flow/server:latest
```

> web 镜像为纯静态站点，不含反代；`/api/admin` 需由外部 ingress / 网关转发到 server 的 7090。

## 配置

配置来自 `server/etc/conf.toml.tpl`（`envsubst` 展开）。常用环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `FLOW_DB_DRIVER` | `pgsql` | 数据库驱动（当前仅支持 pgsql） |
| `FLOW_DB_HOST` / `FLOW_DB_PORT` | `127.0.0.1` / `5432` | 数据库地址 |
| `FLOW_DB_USER` / `FLOW_DB_PASS` | `postgres` / `postgres` | 数据库账号 |
| `FLOW_DB_NAME` | `flowdb` | 数据库名 |
| `FLOW_DB_AUTO_INIT` | `true` | 启动时自动建库（缺失时）并执行迁移 |
| `FLOW_API_SERVER_HTTP_PORT` | `7090` | Admin HTTP API 端口 |
| `FLOW_API_SERVER_GRPC_PORT` | `7091` | gRPC 端口 |
| `FLOW_WORKER_*` | — | 轮询、过期扫描、重试扫描等 worker 参数 |
| `FLOW_SCHEDULER_*` | — | 定时回调调度器参数，见下方「定时回调」；删除整个 `[scheduler]` 段则不启动调度器 |
| `FLOW_OTEL_ENABLED` | `false` | 是否开启 OpenTelemetry |
| `FLOW_OTEL_ENDPOINT` / `FLOW_OTEL_PROTOCOL` | — | OTLP collector 地址 / 协议 |

完整清单见 [`server/.env.example`](server/.env.example) 与 [`server/etc/conf.toml.tpl`](server/etc/conf.toml.tpl)。

> **重要**：`FLOW_DB_AUTO_INIT=true` 且目标库不存在时，server 会自动 `CREATE DATABASE`，这需要数据库账号具备 `CREATEDB` 权限。生产环境建议显式设置该变量，或预先建库后设为 `false`（迁移仍会执行）。自动初始化只会在库缺失时创建，**不会删除任何数据**。

## 数据库与迁移

迁移使用 goose，文件内嵌在二进制中，目录为 `server/migration/postgres/`：

```
server/migration/
├── migration.go               # FSFor(driver)：按方言返回迁移文件系统
└── postgres/
    ├── embed.go               # //go:embed *.sql
    └── 00001_init.sql         # 基线 schema（幂等 DDL）
```

新增迁移：在 `server/migration/postgres/` 下添加 `NNNNN_name.sql`，遵循 goose 格式：

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tasks DROP COLUMN IF EXISTS priority;
```

多实例并发启动由 goose 的 PostgreSQL session lock 串行化，不会重复执行。

## 定时回调

Schedule 是与 Task **完全独立**的一类资源：不共用表、不经过 worker 轮询。客户端注册一条
定时任务（回调 URL + payload + 触发规则），到期后由 flowserver 主动 POST 到该 URL。

### 两类触发

```go
import scheduleclient "github.com/gonotelm-lab/flow/client/schedule"

c, _ := scheduleclient.New("localhost:7091")
defer c.Close()

// 一次性：绝对时刻
c.Register(ctx, scheduleclient.OneShot(time.Now().Add(10*time.Minute),
    "https://example.com/hook", []byte(`{"id":1}`)))

// 重复：标准 5 字段 cron
c.Register(ctx, scheduleclient.Cron("*/5 * * * *", "Asia/Shanghai",
    "https://example.com/hook", []byte(`{"id":1}`),
    scheduleclient.WithIdempotencyKey("nightly-report")))
```

- **一次性**：`fire_at` 为 unix 毫秒，必须严格晚于当前时间（服务端用**数据库时间**判定，避免多 pod 时钟偏移），过期注册被拒绝。
- **重复**：标准 5 字段 `分 时 日 月 周`，最小粒度 1 分钟。每条 schedule 带独立 IANA 时区，缺省 `UTC`。

> **cron 方言警示**：本实现基于 `robfig/cron/v3`，星期是 `0-6` 且 **`0` 表示周日**，月份是
> `1-12`，不支持 `L` / `W` / `#` 与 `@daily` 这类描述符，也不支持 6 字段（秒级）。
> 这与 Quartz **三处都不同**（Quartz 是 7 字段、星期 `1-7` 且 `1=Sunday`、月份 `0-11`），
> 请勿按 Quartz 语义书写。表达式内也不允许自带 `TZ=` / `CRON_TZ=` 前缀。

### 回调契约

```
POST <callback_url>
Content-Type: <注册时指定，缺省 application/octet-stream>
X-Flow-Schedule-Id: <schedule id>
X-Flow-Fire-Id: <本次 fire 的 id，重试与接管之间不变>
X-Flow-Scheduled-Time: <本次触发计划时刻，unix 毫秒>
X-Flow-Timestamp: <unix 秒>
X-Flow-Signature: sha256=<hex>

<body = 注册时传入的 payload 原样字节>
```

- **成功判定**：HTTP 2xx。3xx（不跟随重定向）、4xx、5xx、超时（默认 10s）与连接失败都算失败。
- **签名**：HMAC-SHA256，密钥为 namespace 的 `api_key`，签名串 `timestamp + "." + rawBody`。
  接收方应校验 `X-Flow-Timestamp` 的新鲜度以防重放。

### 投递是 at-least-once

**`X-Flow-Fire-Id` 在同一次 fire 的所有重试与实例接管之间保持不变，接收方必须用它做幂等去重。**
服务端保证「同一次 fire 不会被两个实例同时投递」，唯一可能重复的时刻是持有租约的实例崩溃、
租约过期后被其他实例接管重投。

### 重试与死信

| 阶段 | 次数 | 间隔 |
|---|---|---|
| 快速重试 | 含首次共 6 次尝试 | `1s / 2s / 4s / 8s / 16s`（±20% 抖动） |
| 死信慢重试 | 3 次 | `1m / 5m / 30m` |

全部是服务端全局配置，客户端无法指定。慢重试也全部失败后，死信转 `EXHAUSTED` 等待人工
「重投」或「放弃」——重投会**沿用原 `X-Flow-Fire-Id`**，接收方据此判断这是补投而非新事件。

### 重叠与漏触发（Temporal 语义）

- 一条 schedule 在任一时刻**最多一个未完成的 fire**。整个生命周期（含重试与死信慢重试，
  最坏约 38 分钟）都算「在跑」，期间到点的 occurrence 全部**跳过**并计入 `skipped_count`。
- 分片有一段时间没人扫（pod 挂掉）导致 occurrence 迟到超过容差（默认 30s）时，**丢弃并重算
  下次触发，不补发**；一次性任务在停机期间到期则在恢复后补打一次。
- 因此 `TriggerNow`（仅运维面）与死信「重投」在槽位被占用时会返回 `FailedPrecondition`。
- `Pause` 只停未来的 occurrence；已发出的 HTTP 与快速重试会跑完，死信慢重试继续；
  恢复后按当前时间重算下次触发。**删除是硬删**，定义、失败记录与死信一并消失，服务端会把
  快照写进审计日志。

### 分布式执行

`schedules.shard` 是注册时按 id 算一次、**永不重算**的静态分区键（0..1023）。每个实例从
实例注册表推导自己负责的分片区间，因此扩缩容不需要重写任何数据行。但分片只决定「谁有资格扫」：
真正决定「谁执行」的是数据库层的 `SELECT ... FOR UPDATE SKIP LOCKED` 认领加租约。

### 配置

| 变量 | 默认 | 说明 |
|---|---|---|
| `FLOW_SCHEDULER_TICK_INTERVAL` | `1s` | 到期判定粒度 |
| `FLOW_SCHEDULER_SCAN_BATCH` | `100` | 单次扫描认领上限 |
| `FLOW_SCHEDULER_DELIVER_CONCURRENCY` | `64` | 单实例并发投递上限 |
| `FLOW_SCHEDULER_HTTP_TIMEOUT` | `10s` | 单次回调超时 |
| `FLOW_SCHEDULER_LEASE_DURATION` | `30s` | 认领租约；**必须大于** HTTP 超时，否则正常投递会被自己的租约过期判成可接管 |
| `FLOW_SCHEDULER_LATENESS_TOLERANCE` | `30s` | 迟到容差，超过即视为停机错过，丢弃并重算 |
| `FLOW_SCHEDULER_RETRY_JITTER_RATIO` | `0.2` | 重试抖动比例 |
| `FLOW_SCHEDULER_FAST_RETRY_INTERVALS` | `1s,2s,4s,8s,16s` | 快速重试间隔 |
| `FLOW_SCHEDULER_DEAD_LETTER_INTERVALS` | `1m,5m,30m` | 死信慢重试间隔 |

### 安全须知

回调 URL **只做格式校验**（http/https、host 非空、不含内嵌凭据）：不探测可达性，不拦截私网
与云 metadata 地址，也不跟随重定向。叠加上当前服务端**没有任何鉴权**，这意味着任何能连到
7090 / 7091 的人都可以让 flow 以自身身份向任意内网地址持续 POST 任意内容。

> **部署上必须保证 7090 / 7091 不暴露到不可信网络。** 后续计划是给三个 service 统一加
> namespace API Key 鉴权；在此之前，`schedule` 与现有 `task` / `worker` 处于同一安全水位。

### 审计日志

审计**只覆盖管理接口（Admin HTTP）的操作**：创建、修改、暂停、恢复、删除、手动触发、
死信重投与放弃。客户端走 gRPC 的注册/修改不产生记录。`operator` 字段当前恒为空（无用户
体系），因此现阶段它是「协作记录」而不是「安全审计」；来源 IP 与 UA 是仅有的非自填痕迹。

## 端口

| 端口 | 用途 |
|------|------|
| `7090` | Admin HTTP API（gRPC-Gateway，路径前缀 `/api/admin/v1/...`） |
| `7091` | gRPC（task submit / worker poll 等） |
| `7089` | 前端 Vite dev server（仅开发） |

## 发布与镜像

- 镜像由 GitHub Actions 在**发布 Release 时**构建并推送（`release: published`）。
- 产物：`ghcr.io/gonotelm-lab/flow/server`、`ghcr.io/gonotelm-lab/flow/web`，多架构（`linux/amd64`、`linux/arm64`）。
- 镜像 tag 由 Release tag 派生：`X.Y.Z`（规范化版本号）、`sha-<short>`、`latest`。
- 只有 `vX.Y.Z` 形式的应用级 Release 会构建镜像；其他 tag（如 `server/vX.Y.Z`）即使发布 Release，流水线也会整体跳过。
- 发布流程：

  ```bash
  git tag v0.3.0 && git push origin v0.3.0      # 应用级 tag
  gh release create v0.3.0 --generate-notes
  ```

  Server Go module 版本使用 `server/vX.Y.Z` tag。

## 开发与测试

```bash
cd server
go build ./...
go test ./...          # 集成测试需要可用的 PostgreSQL
```

集成测试通过以下环境变量连接数据库（会创建/清理随机的临时库）：

```bash
FLOW_DB_HOST=127.0.0.1 FLOW_DB_PORT=5432 \
FLOW_DB_USER=postgres FLOW_DB_PASS=postgres FLOW_DB_NAME=flowdb \
go test ./...
```

客户端使用示例见 [`client/example`](client/example)（`echo` / `raw` worker、`schedule` 定时回调）。

## License

[Apache-2.0](LICENSE)

<p align="right">(<a href="#readme-top">back to top</a>)</p>
