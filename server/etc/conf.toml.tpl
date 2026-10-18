[db]
driver = "${FLOW_DB_DRIVER:-pgsql}"
autoInit = ${FLOW_DB_AUTO_INIT:-true}

[db.config]
host = "${FLOW_DB_HOST:-127.0.0.1}"
port = ${FLOW_DB_PORT:-5432}
user = "${FLOW_DB_USER:-postgres}"
password = "${FLOW_DB_PASS:-postgres}"
dbName = "${FLOW_DB_NAME:-flowdb}"

[registry]
expiry = "${FLOW_REGISTRY_EXPIRY:-12s}"
keepaliveInterval = "${FLOW_REGISTRY_KEEPALIVE_INTERVAL:-10s}"
sweepInterval = "${FLOW_REGISTRY_SWEEP_INTERVAL:-1s}"
sweepBatch = ${FLOW_REGISTRY_SWEEP_BATCH:-200}
watchInterval = "${FLOW_REGISTRY_WATCH_INTERVAL:-500ms}"
watchBatchSize = ${FLOW_REGISTRY_WATCH_BATCH_SIZE:-200}
watchMaxRetryBackoff = "${FLOW_REGISTRY_WATCH_MAX_RETRY_BACKOFF:-10s}"

[worker]
pollWait = "${FLOW_WORKER_POLL_WAIT:-10s}"
pollCheckInterval = "${FLOW_WORKER_POLL_CHECK_INTERVAL:-500ms}"
staleTaskTimeout = "${FLOW_WORKER_STALE_TASK_TIMEOUT:-30s}"
staleScanInterval = "${FLOW_WORKER_STALE_SCAN_INTERVAL:-10s}"
retryScanInterval = "${FLOW_WORKER_RETRY_SCAN_INTERVAL:-10s}"
retryScanBatch = ${FLOW_WORKER_RETRY_SCAN_BATCH:-100}
staleScanBatch = ${FLOW_WORKER_STALE_SCAN_BATCH:-100}
workerTtl = "${FLOW_WORKER_TTL:-60s}"
workerRetention = "${FLOW_WORKER_RETENTION:-168h}"
workerReapInterval = "${FLOW_WORKER_REAP_INTERVAL:-1h}"
workerReapBatch = ${FLOW_WORKER_REAP_BATCH:-500}

[scheduler]
# 到期判定粒度。一次性任务要求秒级精度，因此 tick 必须先于或等于最细粒度。
tickInterval = "${FLOW_SCHEDULER_TICK_INTERVAL:-1s}"
scanBatch = ${FLOW_SCHEDULER_SCAN_BATCH:-100}
deliverConcurrency = ${FLOW_SCHEDULER_DELIVER_CONCURRENCY:-64}
httpTimeout = "${FLOW_SCHEDULER_HTTP_TIMEOUT:-10s}"
# 必须显著大于 httpTimeout，否则正常投递会被自己的租约过期判成可接管，制造重复投递。
leaseDuration = "${FLOW_SCHEDULER_LEASE_DURATION:-30s}"
# 迟到容差：超过它视为「停机期间错过」，不触发而是跳过并重算。
latenessTolerance = "${FLOW_SCHEDULER_LATENESS_TOLERANCE:-30s}"
retryJitterRatio = ${FLOW_SCHEDULER_RETRY_JITTER_RATIO:-0.2}
# 逗号分隔的快速重试间隔（含首次共 len+1 次尝试；默认 2 个间隔 = 首发 + 2 次重试 = 3 次快尝试）
fastRetryIntervals = "${FLOW_SCHEDULER_FAST_RETRY_INTERVALS:-3s,10s}"
# 逗号分隔的死信慢重试间隔
deadLetterIntervals = "${FLOW_SCHEDULER_DEAD_LETTER_INTERVALS:-1m,5m,30m}"

[apiServer]
[apiServer.http]
listen = "${FLOW_API_SERVER_HTTP_LISTEN:-0.0.0.0}"
port = ${FLOW_API_SERVER_HTTP_PORT:-7090}

[apiServer.grpc]
listen = "${FLOW_API_SERVER_GRPC_LISTEN:-0.0.0.0}"
port = ${FLOW_API_SERVER_GRPC_PORT:-7091}

[apiServer.auth]
enabled = ${FLOW_API_AUTH_ENABLED:-false}

[otel]
enabled = ${FLOW_OTEL_ENABLED:-false}
serviceName = "${FLOW_OTEL_SERVICE_NAME:-flow-server}"
endpoint = "${FLOW_OTEL_ENDPOINT:-}"
protocol = "${FLOW_OTEL_PROTOCOL:-}"
samplerRatio = ${FLOW_OTEL_SAMPLER_RATIO:-1.0}
