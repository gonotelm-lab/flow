[db]
driver = "${FLOW_DB_DRIVER:-pgsql}"

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

[apiServer]
[apiServer.http]
listen = "${FLOW_API_SERVER_HTTP_LISTEN:-0.0.0.0}"
port = ${FLOW_API_SERVER_HTTP_PORT:-7090}

[apiServer.grpc]
listen = "${FLOW_API_SERVER_GRPC_LISTEN:-0.0.0.0}"
port = ${FLOW_API_SERVER_GRPC_PORT:-7091}
