<a id="readme-top"></a>

<h1 align="center">
  <img src="web/public/brand/flow-lockup.svg" alt="Flow" height="34" />
</h1>

<p align="center">
  <a href="https://github.com/gonotelm-lab/flow/releases"><img src="https://img.shields.io/github/v/release/gonotelm-lab/flow" alt="Release"></a>
  <a href="https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml"><img src="https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml/badge.svg" alt="Publish"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/gonotelm-lab/flow" alt="License"></a>
</p>

Flow 是一个分布式的**任务队列 / 工作流引擎**。它由三部分组成：

- **Server** — Go 服务，提供 gRPC API、Admin HTTP API、服务实例注册与分片、任务调度/重试/过期检测。
- **Client SDK** — Go 客户端，包含任务提交（task）、定时回调（schedule）与工作节点（worker）运行时。
- **Admin Web** — React 管理后台，用于可视化 namespace、task、worker 的运行状态。

Flow 用 namespace 隔离任务，用 worker 心跳感知节点健康，并通过 OpenTelemetry 打通全链路追踪。

## 核心能力

- **命名空间隔离** — 每个 namespace 独立管理任务与 worker，携带独立 API Key。
- **任务生命周期** — `INITED → RUNNING → DONE / FAILED`，支持延迟执行（`next_run_time`）、最大重试次数、取消与删除。
- **定时回调** — 与 Task 完全独立的 Schedule 资源：注册回调 URL 与 payload，支持一次性（绝对时刻）与 cron（标准 5 字段）两类触发，到期由 server 主动 POST。内置有界重试、死信与管理员审计日志。
- **Worker 注册与心跳** — worker 上报心跳，server 侧检测过期/卡住任务并重新调度；节点处理量由 server 按 `tasks.worker_id` 聚合（只看 `DONE` / `FAILED` / `CANCELLED`），节点清理分两段：心跳超过 `workerTtl` 从列表隐藏，超过 `workerRetention` 硬删除。
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
| `FLOW_WORKER_*` | — | 轮询、过期扫描、重试扫描、节点清理（`FLOW_WORKER_TTL` / `FLOW_WORKER_RETENTION` / `FLOW_WORKER_REAP_INTERVAL` / `FLOW_WORKER_REAP_BATCH`）等 worker 参数 |
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

// 一个 client 对应一个 namespace
c, _ := scheduleclient.New("localhost:7091", scheduleclient.Config{Namespace: "demo"})
defer c.Close()

// 一次性：绝对时刻
c.Register(ctx, scheduleclient.OneShot(time.Now().Add(10*time.Minute),
    "https://example.com/hook", []byte(`{"id":1}`)))

// 重复：标准 5 字段 cron；给了幂等键，重复注册拿回同一条
c.Register(ctx, scheduleclient.Cron("*/5 * * * *", "Asia/Shanghai",
    "https://example.com/hook", []byte(`{"id":1}`),
    scheduleclient.WithIdempotencyKey("nightly-report")))
```

SDK 只收发自己的类型，调用方不需要引入 proto：注册提交的是 `Spec`（只有定义字段，
namespace 在 `Config` 里、运行态字段不在里面），服务端返回的是 `Schedule`（定义 + namespace
+ 运行态）。**定义注册后不可修改**（两端都没有 Update 接口），要改就用同一个幂等键重新注册
一条。时间字段都是 `time.Time`，零值表示「没有这个时刻」。

幂等键没显式设置时，SDK 会自动生成一个随机键，这样 RPC 层的重试不会建出两条；要跨调用去重
（超时后自己重发、进程重启后重放）就显式给一个稳定的键。

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
X-Flow-Namespace: <该 schedule 所属 namespace>
X-Flow-Signature: FLOW-HMAC-SHA256 Scope=<YYYYMMDD>/<namespace>/flow_request, \
                  Signed=content-type;x-flow-fire-id;..., Signature=<hex>

<body = 注册时传入的 payload 原样字节>
```

- **成功判定**：HTTP 2xx。3xx（不跟随重定向）、4xx、5xx、超时（默认 10s）与连接失败都算失败。

#### 签名（FLOW-HMAC-SHA256）

密钥是 namespace 的 `api_key`，但**不直接拿它做 HMAC**，而是先按日期与 namespace 派生：

```
signingKey = HMAC(HMAC(HMAC("FLOW" + api_key, YYYYMMDD), namespace), "flow_request")
```

请求本身先规范化，再对规范化结果签名（下面是逐字节的规则，手写接收方照这个来）：

```
canonicalRequest = "POST" + "\n"
                 + 路径 + "\n"                    # URL.EscapedPath()，空则 "/"
                 + 规范化查询串 + "\n"              # RFC3986 编码，按编码后的名、再按值排序，& 连接
                 + 规范化头块                      # 每个头一行："小写名:值\n"
                 + "\n"                           # 头块本身以 \n 结尾，所以这里有一个空行
                 + Signed 清单 + "\n"              # 分号连接，固定六项，升序
                 + hex(sha256(body))
stringToSign     = "FLOW-HMAC-SHA256" + "\n"
                 + 时刻 + "\n"                     # ISO8601 basic，UTC：YYYYMMDDTHHMMSSZ
                 + Scope + "\n"                    # <YYYYMMDD>/<namespace>/flow_request
                 + hex(sha256(canonicalRequest))
Signature        = hex(HMAC(signingKey, stringToSign))
```

- 分隔符是 `\n`；**拼接时不要给任何一行加尾随空格**，也别忘了头块后面那个空行。
- 规范化头的值：去掉首尾空白、连续空白折成一个空格；同名多头用逗号连接。
- 规范化查询串：先解码再按 RFC3986 编码（空格是 `%20` 不是 `+`），同名参数按值升序，空值保留 `=`。

参与签名的头是固定的六个：`content-type`、`x-flow-fire-id`、`x-flow-namespace`、
`x-flow-schedule-id`、`x-flow-scheduled-time`、`x-flow-timestamp`。**清单是闭合的**：验签方
要求它们全部出现，否则签名方可以「少签 timestamp」再重放而不被察觉。

几点取舍：

- **不签 host**：反向代理普遍会改写 Host（nginx 默认 `proxy_set_header Host $proxy_host`），
  签了它就得让所有接收方改代理配置；而路径、查询串与 schedule id 已足以约束请求归属。
- **scope 里的日期取自 `X-Flow-Timestamp`**：用另一个日期的派生密钥重放，会在验签阶段被判不一致。
- **namespace 在 scope 里编码、在派生密钥里不编码**：`signingKey` 用的是 `X-Flow-Namespace`
  的原始值；只有写进 `Scope=` 的那一份按 RFC3986 编码（namespace 只校验非空，可能带逗号或空格，
  不编码会撑破签名头「逗号分参数」的语法）。常见名字（`demo` 之类）编码后原样不变。
  Scope 必须写成这个规范形式，非规范写法（例如把空格原样写进去）会被拒。
- `X-Flow-Signature` 的参数不允许重复：同名参数出现两次会被拒。
- 时间戳本身参与签名，接收方**必须**校验它的新鲜度，否则旧回调可被原样重放。

接收方直接用 SDK，一行完成「复算签名 + 判时间戳新鲜度」：

```go
import (
    "net/http"
    scheduleclient "github.com/gonotelm-lab/flow/client/schedule"
)

// 接收方只用验签：地址留空即可（gRPC 懒连接，不调接口就不会建连）。
// 带上 Namespace 就会顺带核对 X-Flow-Namespace，不配则跳过这项。
var verify, _ = scheduleclient.New("", scheduleclient.Config{
    Namespace: namespace,
    APIKey:    apiKey,
})

func hook(w http.ResponseWriter, r *http.Request) {
    // 读完 body 会还原回 r.Body；默认容差前后各 5 分钟，默认最多读 10 MiB
    if err := verify.VerifyRequest(r); err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    // 到这里签名与时效都已确认，body 仍可正常读；
    // X-Flow-Fire-Id 去重仍然必须自己做
    w.WriteHeader(http.StatusOK)
}
```

- 容差与 body 上限：`WithTolerance`（传 0 关闭新鲜度校验）、`WithMaxBodySize`。
- 失败原因用 `errors.Is` 分类：`ErrStaleTimestamp`、`ErrBodyTooLarge`、`ErrNoAPIKey`、
  `signature.ErrMismatch`、`signature.ErrMissingHeader`、`signature.ErrMalformedSignature`、
  `signature.ErrScopeMismatch`（`Config.Namespace` 与回调头不一致时返回它）、
  `signature.ErrUnsupportedAlgorithm`。
- 调用方已经自己读过 body 时用 `VerifyRequestWithBody`（不会碰 `r.Body`）。
- 可运行示例：`client/example/schedule-receiver`（含验签 + Fire-Id 去重骨架）。

> **破坏性变更**：早期的一行式签名 `X-Flow-Signature: sha256=<hex>`（`HMAC(api_key, timestamp + "." + body)`）
> 已被本方案取代，旧格式会被明确拒绝（`signature.ErrMalformedSignature`）。升级时两侧必须同时更新。

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
「重投」——重投会**沿用原 `X-Flow-Fire-Id`**，接收方据此判断这是补投而非新事件，成功后死信
记为 `REPLAYED`。没有「放弃」这个操作：不想再投就把 schedule 停掉，它的 `RETRYING` 死信会
一并变成 `STOPPED`。

### 重叠与漏触发（Temporal 语义）

- 一条 schedule 在任一时刻**最多一个未完成的 fire**。整个生命周期（含重试与死信慢重试，
  最坏约 38 分钟）都算「在跑」，期间到点的 occurrence 全部**跳过**并计入 `skipped_count`。
- 分片有一段时间没人扫（pod 挂掉）导致 occurrence 迟到超过容差（默认 30s）时，**丢弃并重算
  下次触发，不补发**；一次性任务在停机期间到期则在恢复后补打一次。
- 因此 `TriggerNow`（仅运维面）与死信「重投」在槽位被占用时会返回 `FailedPrecondition`。

### 状态与停止

`state` 只有 `ACTIVE` 会触发回调，其余三个都是终态，进去之后只能读和删除：

| 状态 | 含义 |
|---|---|
| `ACTIVE` | 正常触发 |
| `STOPPED` | 人为单向停止，`Stop` / `StopSchedule` 进入 |
| `FINISHED` | 一次性任务投递成功 |
| `FAILED` | 一次性任务死信重试耗尽 |

- **cron 没有自动终态**，只会停在 `STOPPED`（人为）或随整条记录被删除。
- **stop 不可逆**：没有恢复接口。要再触发就用同一个幂等键重新注册一条（stop 会释放幂等键）。
- stop 只清掉「还没触发」的部分：触发计划、fire 槽位、租约，以及慢重试中的死信（转
  `STOPPED`）。**已经发出的 HTTP 撤不回来**，它的重试与记账到此为止。
- 终态是墓碑：`TriggerNow`、死信「重投」在终态下都返回
  `FailedPrecondition`（错误码 `SCHEDULE_ENDED`）。
- **删除是硬删**，定义、失败记录与死信一并消失，服务端会把快照写进审计日志。

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

审计**只覆盖管理接口（Admin HTTP）的操作**：创建、停止、删除、手动触发、
死信重投。客户端走 gRPC 的注册不产生记录。`operator` 字段当前恒为空（无用户
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
