#!/usr/bin/env bash
#
# 端到端测试：起 flowserver（真 PostgreSQL），跑 client/e2e。
#
# 鉴权开、关两种模式各跑一遍：两种模式下 client/e2e 的断言不同。
#
# 用法：
#   scripts/e2e.sh                # 用已有的 PostgreSQL（FLOW_DB_* 或默认值）
#   scripts/e2e.sh --docker-db    # 自己起一个临时 postgres:18，退出时删掉
#
# CI 里由 .github/workflows/ci.yml 调用（数据库来自 service 容器，所以不带 --docker-db）。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GRPC_PORT="${FLOW_E2E_GRPC_PORT:-7091}"
HTTP_PORT="${FLOW_E2E_HTTP_PORT:-7090}"
ADMIN_URL="http://127.0.0.1:${HTTP_PORT}"

DB_HOST="${FLOW_DB_HOST:-127.0.0.1}"
DB_PORT="${FLOW_DB_PORT:-5432}"
DB_USER="${FLOW_DB_USER:-postgres}"
DB_PASS="${FLOW_DB_PASS:-postgres}"
DB_NAME="${FLOW_DB_NAME:-flowdb}"

USE_DOCKER_DB=0
for arg in "$@"; do
  case "$arg" in
    --docker-db) USE_DOCKER_DB=1 ;;
    *)
      echo "未知参数：$arg" >&2
      exit 2
      ;;
  esac
done

WORK_DIR="$(mktemp -d)"
# 日志留在仓库下的固定目录（已 gitignore），失败时 CI 会把它当 artifact 传上来。
LOG_DIR="${E2E_LOG_DIR:-$ROOT/.e2e-logs}"
CONTAINER="flow-e2e-pg-$$"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ "$USE_DOCKER_DB" == 1 ]]; then
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$LOG_DIR"

port_open() {
  (echo > "/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

if [[ "$USE_DOCKER_DB" == 1 ]]; then
  echo "== 启动临时 PostgreSQL =="
  docker run -d --name "$CONTAINER" \
    -e POSTGRES_PASSWORD="$DB_PASS" \
    -e POSTGRES_USER="$DB_USER" \
    -e POSTGRES_DB="$DB_NAME" \
    -p "${DB_PORT}:5432" \
    postgres:18 >/dev/null

  for _ in $(seq 1 60); do
    if docker exec "$CONTAINER" pg_isready -U "$DB_USER" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi

if port_open "$GRPC_PORT"; then
  echo "端口 ${GRPC_PORT} 已被占用：请先停掉本机已有的 flowserver" >&2
  exit 1
fi

echo "== 构建 flowserver =="
(cd "$ROOT/server" && go build -o "$WORK_DIR/flowserver" ./cmd/flowserver)

run_mode() {
  local mode="$1" auth="$2"
  local log="$LOG_DIR/flowserver-${mode}.log"

  echo "== 启动 flowserver（api_key 鉴权：${mode}）=="
  # 直接后台起进程，不要再包一层子 shell：那样 $! 拿到的是子 shell，kill 杀不到 server。
  FLOW_DB_HOST="$DB_HOST" \
    FLOW_DB_PORT="$DB_PORT" \
    FLOW_DB_USER="$DB_USER" \
    FLOW_DB_PASS="$DB_PASS" \
    FLOW_DB_NAME="$DB_NAME" \
    FLOW_DB_AUTO_INIT=true \
    FLOW_OTEL_ENABLED=false \
    FLOW_API_AUTH_ENABLED="$auth" \
    "$WORK_DIR/flowserver" -conf "$ROOT/server/etc/conf.toml.tpl" >"$log" 2>&1 &
  SERVER_PID=$!

  for _ in $(seq 1 60); do
    if port_open "$GRPC_PORT"; then
      break
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      echo "flowserver 启动失败：" >&2
      cat "$log" >&2
      exit 1
    fi
    sleep 1
  done

  echo "== 跑 e2e（FLOW_E2E_AUTH_MODE=${mode}）=="
  if ! (
    cd "$ROOT/client"
    FLOW_E2E_ADDR="127.0.0.1:${GRPC_PORT}" \
      FLOW_E2E_ADMIN_URL="$ADMIN_URL" \
      FLOW_E2E_AUTH_MODE="$mode" \
      go test ./e2e/... -count=1 -v
  ); then
    echo "e2e 失败，flowserver 日志：" >&2
    cat "$log" >&2
    exit 1
  fi

  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  SERVER_PID=""

  # 等端口真正释放，否则下一个模式会撞上「address already in use」。
  for _ in $(seq 1 30); do
    if ! port_open "$GRPC_PORT"; then
      return 0
    fi
    sleep 1
  done
  echo "flowserver 停止后端口 ${GRPC_PORT} 仍被占用" >&2
  exit 1
}

run_mode enabled true
run_mode disabled false

echo "== e2e 全部通过 =="
