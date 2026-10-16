-- +goose Up

-- 定时回调（Schedule）四张表。
--
-- 设计要点：
--   1. schedules.shard 是静态分区键（0..1023），注册时按 id 哈希算一次、永不重算；
--      「分片 -> 实例」的映射由实例注册表动态推导，扩缩容不需要重写数据行。
--   2. 每条 schedule 同时最多一个未完成 fire，fire 状态直接压在 schedules 行上
--      （不单独建 fire 表），fire_state IS NOT NULL 即表示该 schedule 被阻塞。
--   3. 正确性来自「FOR UPDATE SKIP LOCKED 认领 + 租约」，分片只用于切分扫描范围。

CREATE TABLE IF NOT EXISTS schedules (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  namespace VARCHAR(128) NOT NULL,
  name VARCHAR(128),

  -- ONE_SHOT | CRON
  schedule_type VARCHAR(16) NOT NULL,
  -- CRON 专用：标准 5 字段（分 时 日 月 周）
  cron_expr VARCHAR(128),
  -- CRON 专用：IANA 时区名，缺省 UTC
  timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
  -- ONE_SHOT 专用：unix 毫秒
  fire_at BIGINT NOT NULL DEFAULT 0,

  -- ACTIVE | STOPPED | FINISHED | FAILED；只有 ACTIVE 会触发回调
  state VARCHAR(16) NOT NULL CONSTRAINT ck_schedules_state
    CHECK (state IN ('ACTIVE', 'STOPPED', 'FINISHED', 'FAILED')),
  callback_url VARCHAR(2048) NOT NULL,
  content_type VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream',
  payload BYTEA,
  idempotency_key VARCHAR(128),

  -- 调度：静态分片号 + 下次计划触发时刻（unix 毫秒）
  shard SMALLINT NOT NULL DEFAULT 0 CONSTRAINT ck_schedules_shard CHECK (shard >= 0 AND shard < 1024),
  next_fire_at BIGINT NOT NULL DEFAULT 0,

  -- fire 槽位：同时最多一个未完成 fire
  fire_id UUID,
  fire_scheduled_time BIGINT NOT NULL DEFAULT 0,
  -- NULL | PENDING | DELIVERING | DEAD_LETTER
  fire_state VARCHAR(16),
  attempt_no SMALLINT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL DEFAULT 0,
  lease_owner VARCHAR(128),
  lease_until BIGINT NOT NULL DEFAULT 0,

  -- 运行统计
  consecutive_failures INT NOT NULL DEFAULT 0,
  total_failures BIGINT NOT NULL DEFAULT 0,
  -- 因上一次 fire 未结束（或判定迟到）而跳过的 occurrence 数
  skipped_count BIGINT NOT NULL DEFAULT 0,
  last_error VARCHAR(1024),
  last_success_at BIGINT NOT NULL DEFAULT 0,

  create_time BIGINT NOT NULL DEFAULT 0,
  update_time BIGINT NOT NULL DEFAULT 0
);

-- 扫描待触发：只覆盖「空闲且 ACTIVE」的行，与重试扫描互不污染
CREATE INDEX IF NOT EXISTS idx_schedules_due
  ON schedules (shard ASC, next_fire_at ASC)
  WHERE state = 'ACTIVE' AND fire_state IS NULL;

-- 扫描待重试：快重试与死信慢重试共用
CREATE INDEX IF NOT EXISTS idx_schedules_retry
  ON schedules (shard ASC, next_attempt_at ASC)
  WHERE fire_state IS NOT NULL;

-- 租约过期接管
CREATE INDEX IF NOT EXISTS idx_schedules_lease
  ON schedules (lease_until ASC)
  WHERE fire_state IS NOT NULL;

-- Admin 列表
CREATE INDEX IF NOT EXISTS idx_schedules_namespace_create_time
  ON schedules (namespace ASC, create_time DESC);

-- 幂等注册：namespace 内唯一，NULL 不参与唯一性
CREATE UNIQUE INDEX IF NOT EXISTS uk_schedules_idempotency
  ON schedules (namespace, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

COMMENT ON TABLE schedules IS 'scheduled callback definition plus its single in-flight fire slot';
COMMENT ON COLUMN schedules.id IS 'schedule id, primary key';
COMMENT ON COLUMN schedules.namespace IS 'namespace';
COMMENT ON COLUMN schedules.name IS 'human readable name';
COMMENT ON COLUMN schedules.schedule_type IS 'ONE_SHOT | CRON';
COMMENT ON COLUMN schedules.cron_expr IS 'standard 5-field cron expression (min hour dom month dow)';
COMMENT ON COLUMN schedules.timezone IS 'IANA timezone name, default UTC';
COMMENT ON COLUMN schedules.fire_at IS 'one-shot fire time, unix millis';
COMMENT ON COLUMN schedules.state IS 'ACTIVE | STOPPED | FINISHED | FAILED';
COMMENT ON COLUMN schedules.callback_url IS 'callback URL, http/https only';
COMMENT ON COLUMN schedules.content_type IS 'content type used when delivering the callback';
COMMENT ON COLUMN schedules.payload IS 'raw callback body';
COMMENT ON COLUMN schedules.idempotency_key IS 'optional idempotency key, unique per namespace';
COMMENT ON COLUMN schedules.shard IS 'static shard id 0..1023, immutable after insert';
COMMENT ON COLUMN schedules.next_fire_at IS 'next planned fire time, unix millis';
COMMENT ON COLUMN schedules.fire_id IS 'current fire id, stable across retries';
COMMENT ON COLUMN schedules.fire_scheduled_time IS 'planned time of the current fire, unix millis';
COMMENT ON COLUMN schedules.fire_state IS 'NULL | PENDING | DELIVERING | DEAD_LETTER';
COMMENT ON COLUMN schedules.attempt_no IS 'attempt number of the current fire';
COMMENT ON COLUMN schedules.next_attempt_at IS 'next delivery attempt time of the current fire, unix millis';
COMMENT ON COLUMN schedules.lease_owner IS 'instance key holding the delivery lease';
COMMENT ON COLUMN schedules.lease_until IS 'lease expiry, unix millis';
COMMENT ON COLUMN schedules.consecutive_failures IS 'consecutive failed fires';
COMMENT ON COLUMN schedules.total_failures IS 'total failed attempts';
COMMENT ON COLUMN schedules.skipped_count IS 'occurrences skipped because the previous fire was still running or arrived too late';
COMMENT ON COLUMN schedules.last_error IS 'last delivery error summary';
COMMENT ON COLUMN schedules.last_success_at IS 'last successful delivery time, unix millis';
COMMENT ON COLUMN schedules.create_time IS 'create time, unix millis';
COMMENT ON COLUMN schedules.update_time IS 'update time, unix millis';

-- 每次失败的投递尝试落一行；成功不记录。
CREATE TABLE IF NOT EXISTS schedule_attempt_failures (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  schedule_id UUID NOT NULL,
  namespace VARCHAR(128) NOT NULL,
  fire_id UUID NOT NULL,
  attempt_no SMALLINT NOT NULL,
  -- FAST | DEAD_LETTER
  phase VARCHAR(16) NOT NULL,
  scheduled_time BIGINT NOT NULL DEFAULT 0,
  attempt_time BIGINT NOT NULL DEFAULT 0,
  -- 0 表示无响应（超时 / 连接失败）
  http_status INT NOT NULL DEFAULT 0,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  error VARCHAR(1024),
  create_time BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_schedule_attempt_failures_schedule
  ON schedule_attempt_failures (schedule_id ASC, create_time DESC);

-- 预留：后续 TTL 清理任务按此索引删除
CREATE INDEX IF NOT EXISTS idx_schedule_attempt_failures_create_time
  ON schedule_attempt_failures (create_time ASC);

COMMENT ON TABLE schedule_attempt_failures IS 'failed delivery attempts only; successes are not recorded';
COMMENT ON COLUMN schedule_attempt_failures.id IS 'attempt id, primary key';
COMMENT ON COLUMN schedule_attempt_failures.schedule_id IS 'schedule id';
COMMENT ON COLUMN schedule_attempt_failures.namespace IS 'namespace';
COMMENT ON COLUMN schedule_attempt_failures.fire_id IS 'fire id, stable across retries';
COMMENT ON COLUMN schedule_attempt_failures.attempt_no IS 'attempt number within the fire';
COMMENT ON COLUMN schedule_attempt_failures.phase IS 'FAST | DEAD_LETTER';
COMMENT ON COLUMN schedule_attempt_failures.scheduled_time IS 'planned fire time, unix millis';
COMMENT ON COLUMN schedule_attempt_failures.attempt_time IS 'attempt time, unix millis';
COMMENT ON COLUMN schedule_attempt_failures.http_status IS 'HTTP status, 0 when no response';
COMMENT ON COLUMN schedule_attempt_failures.duration_ms IS 'attempt duration in milliseconds';
COMMENT ON COLUMN schedule_attempt_failures.error IS 'error summary';
COMMENT ON COLUMN schedule_attempt_failures.create_time IS 'create time, unix millis';

-- 死信：快重试耗尽后进入。RETRYING 阻塞 schedule；EXHAUSTED 不阻塞，等人工重投；
-- 重投后是 REPLAYED，随 schedule 停掉的是 STOPPED。
CREATE TABLE IF NOT EXISTS schedule_dead_letters (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  schedule_id UUID NOT NULL,
  namespace VARCHAR(128) NOT NULL,
  fire_id UUID NOT NULL,
  scheduled_time BIGINT NOT NULL DEFAULT 0,
  attempt_count SMALLINT NOT NULL DEFAULT 0,
  -- RETRYING | EXHAUSTED | REPLAYED | STOPPED
  state VARCHAR(16) NOT NULL,
  next_attempt_at BIGINT NOT NULL DEFAULT 0,
  last_http_status INT NOT NULL DEFAULT 0,
  last_error VARCHAR(1024),
  -- 各次尝试的 JSON 摘要
  attempts BYTEA,
  create_time BIGINT NOT NULL DEFAULT 0,
  update_time BIGINT NOT NULL DEFAULT 0
);

-- 仅约束「重试中」：EXHAUSTED 可以随时间累积多条
CREATE UNIQUE INDEX IF NOT EXISTS uk_schedule_dead_letters_retrying
  ON schedule_dead_letters (schedule_id)
  WHERE state = 'RETRYING';

CREATE INDEX IF NOT EXISTS idx_schedule_dead_letters_retry
  ON schedule_dead_letters (next_attempt_at ASC)
  WHERE state = 'RETRYING';

CREATE INDEX IF NOT EXISTS idx_schedule_dead_letters_namespace
  ON schedule_dead_letters (namespace ASC, create_time DESC);

COMMENT ON TABLE schedule_dead_letters IS 'dead letters: fires whose retries were exhausted';
COMMENT ON COLUMN schedule_dead_letters.id IS 'dead letter id, primary key';
COMMENT ON COLUMN schedule_dead_letters.schedule_id IS 'schedule id';
COMMENT ON COLUMN schedule_dead_letters.namespace IS 'namespace';
COMMENT ON COLUMN schedule_dead_letters.fire_id IS 'fire id, reused by manual replay so receivers can dedupe';
COMMENT ON COLUMN schedule_dead_letters.scheduled_time IS 'planned fire time, unix millis';
COMMENT ON COLUMN schedule_dead_letters.attempt_count IS 'attempts made so far';
COMMENT ON COLUMN schedule_dead_letters.state IS 'RETRYING | EXHAUSTED | REPLAYED | STOPPED';
COMMENT ON COLUMN schedule_dead_letters.next_attempt_at IS 'next slow retry time, unix millis';
COMMENT ON COLUMN schedule_dead_letters.last_http_status IS 'last HTTP status, 0 when no response';
COMMENT ON COLUMN schedule_dead_letters.last_error IS 'last error summary';
COMMENT ON COLUMN schedule_dead_letters.attempts IS 'JSON summary of all attempts';
COMMENT ON COLUMN schedule_dead_letters.create_time IS 'create time, unix millis';
COMMENT ON COLUMN schedule_dead_letters.update_time IS 'update time, unix millis';

-- Admin 操作审计日志。设计为通用（target_type / target_id），当前只写 schedule 的
-- admin 管理接口操作；task / namespace / worker 后续接入，无需改表。
CREATE TABLE IF NOT EXISTS admin_audit_logs (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  -- CREATE_SCHEDULE / STOP_SCHEDULE /
  -- DELETE_SCHEDULE / TRIGGER_SCHEDULE / REPLAY_DEAD_LETTER
  action VARCHAR(64) NOT NULL,
  -- SCHEDULE（后续：TASK / NAMESPACE / WORKER）
  target_type VARCHAR(32) NOT NULL,
  target_id VARCHAR(128),
  namespace VARCHAR(128),
  -- 保留字段：当前无用户体系，恒为空，待接入后填 user id
  operator VARCHAR(128),
  source_ip VARCHAR(64),
  user_agent VARCHAR(512),
  -- ADMIN_HTTP（当前仅此来源）
  source VARCHAR(32) NOT NULL,
  -- 变更前/删除前关键字段的 JSON 快照
  snapshot BYTEA,
  -- OK | ERROR
  result VARCHAR(16) NOT NULL,
  error_message VARCHAR(1024),
  create_time BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_create_time
  ON admin_audit_logs (create_time DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_target
  ON admin_audit_logs (target_type ASC, target_id ASC, create_time DESC);

COMMENT ON TABLE admin_audit_logs IS 'admin operation audit log, generic over target_type';
COMMENT ON COLUMN admin_audit_logs.id IS 'audit log id, primary key';
COMMENT ON COLUMN admin_audit_logs.action IS 'action name, e.g. DELETE_SCHEDULE';
COMMENT ON COLUMN admin_audit_logs.target_type IS 'target resource type, e.g. SCHEDULE';
COMMENT ON COLUMN admin_audit_logs.target_id IS 'target resource id';
COMMENT ON COLUMN admin_audit_logs.namespace IS 'namespace';
COMMENT ON COLUMN admin_audit_logs.operator IS 'reserved, empty until a user system exists';
COMMENT ON COLUMN admin_audit_logs.source_ip IS 'caller source ip';
COMMENT ON COLUMN admin_audit_logs.user_agent IS 'caller user agent';
COMMENT ON COLUMN admin_audit_logs.source IS 'ADMIN_HTTP';
COMMENT ON COLUMN admin_audit_logs.snapshot IS 'JSON snapshot of key fields before the change';
COMMENT ON COLUMN admin_audit_logs.result IS 'OK | ERROR';
COMMENT ON COLUMN admin_audit_logs.error_message IS 'error message when result is ERROR';
COMMENT ON COLUMN admin_audit_logs.create_time IS 'create time, unix millis';

-- +goose Down
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS schedule_dead_letters;
DROP TABLE IF EXISTS schedule_attempt_failures;
DROP TABLE IF EXISTS schedules;
