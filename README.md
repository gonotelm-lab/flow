<a id="readme-top"></a>

# Flow

[![Release](https://img.shields.io/github/v/release/gonotelm-lab/flow)](https://github.com/gonotelm-lab/flow/releases)
[![Publish](https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml/badge.svg)](https://github.com/gonotelm-lab/flow/actions/workflows/publish.yml)
[![License](https://img.shields.io/github/license/gonotelm-lab/flow)](LICENSE)

Flow 是一个分布式的**任务队列 / 工作流引擎**。它由三部分组成：

- **Server** — Go 服务，提供 gRPC API、Admin HTTP API、服务实例注册与分片、任务调度/重试/过期检测。
- **Client SDK** — Go 客户端，包含任务提交（task）与工作节点（worker）运行时。
- **Admin Web** — React 管理后台，用于可视化 namespace、task、worker 的运行状态。

Flow 用 namespace 隔离任务，用 worker 心跳感知节点健康，并通过 OpenTelemetry 打通全链路追踪。

## 核心能力

- **命名空间隔离** — 每个 namespace 独立管理任务与 worker，携带独立 API Key。
- **任务生命周期** — `INITED → RUNNING → DONE / FAILED`，支持延迟执行（`next_run_time`）、最大重试次数、取消与删除。
- **Worker 注册与心跳** — worker 上报心跳与统计，server 侧检测过期/卡住任务并重新调度。
- **多实例与分片** — 服务实例注册到注册表，通过分片计算分配工作，支撑水平扩展。
- **可观测性** — 内建 OpenTelemetry tracing（gRPC / HTTP / GORM），可对接 OTLP collector。
- **版本化 Schema 迁移** — 基于 [goose](https://github.com/pressly/goose) 的迁移内嵌进二进制，启动时可自动建表。
- **运维后台** — 任务列表（状态筛选 + 详情 Dialog）、worker 健康、namespace / API Key 管理。

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

客户端使用示例见 [`client/example`](client/example)（`echo` / `raw` worker）。

## License

[Apache-2.0](LICENSE)

<p align="right">(<a href="#readme-top">back to top</a>)</p>
