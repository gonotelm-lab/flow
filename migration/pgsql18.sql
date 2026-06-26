CREATE DATABASE flowdb;

\c flowdb;

CREATE TABLE namespaces (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name VARCHAR(128) NOT NULL CONSTRAINT uk_name UNIQUE,
  description VARCHAR(255),
  api_key VARCHAR(128) NOT NULL CONSTRAINT uk_api_key UNIQUE,
  creator VARCHAR(255),
  create_time BIGINT NOT NULL DEFAULT 0,
  update_time BIGINT NOT NULL DEFAULT 0
);

-- 服务实例注册表
CREATE TABLE instances (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "group" VARCHAR(128) NOT NULL DEFAULT '',
  "key" VARCHAR(128) NOT NULL DEFAULT '' CONSTRAINT uk_key UNIQUE,
  value VARCHAR(1024),
  start_time BIGINT NOT NULL DEFAULT 0,
  expire_time BIGINT NOT NULL DEFAULT 0,
  fencing_token BIGINT NOT NULL,
  create_revision BIGINT NOT NULL,
  extras BYTEA
);
CREATE INDEX idx_instances_expire_time_id ON instances (expire_time ASC, id ASC);

-- 服务实例状态变化事件表
CREATE TABLE instance_events (
  revision BIGINT NOT NULL PRIMARY KEY,
  "group" VARCHAR(128) NOT NULL DEFAULT '',
  "key" VARCHAR(128) NOT NULL DEFAULT '',
  value VARCHAR(1024),
  "type" VARCHAR(16),
  create_time BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_instance_events_group_revision ON instance_events ("group" ASC, revision ASC);

-- 全局revision表
CREATE TABLE global_revisions (
  name VARCHAR(128) NOT NULL PRIMARY KEY,
  current_revision BIGINT NOT NULL DEFAULT 0,
  update_time BIGINT NOT NULL DEFAULT 0
);

-- task任务表
CREATE TABLE tasks (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  namespace VARCHAR(128) NOT NULL,
  task_type VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  payload BYTEA,
  result BYTEA,
  error BYTEA,
  create_time BIGINT NOT NULL DEFAULT 0,
  next_run_time BIGINT NOT NULL DEFAULT 0,
  update_time BIGINT NOT NULL DEFAULT 0,
  worker_id BIGINT NOT NULL,
  max_retry SMALLINT NOT NULL DEFAULT 0,
  attempt_no SMALLINT NOT NULL DEFAULT 0
);

ALTER TABLE tasks ADD COLUMN last_heartbeat_time BIGINT NOT NULL DEFAULT 0;

CREATE INDEX idx_tasks_namespace_task_type
ON tasks (namespace, task_type);

CREATE INDEX idx_tasks_worker_id
ON tasks (worker_id);

COMMENT ON TABLE tasks IS 'task table';
COMMENT ON COLUMN tasks.id IS 'task id, primary key';
COMMENT ON COLUMN tasks.namespace IS 'namespace';
COMMENT ON COLUMN tasks.task_type IS 'task type';
COMMENT ON COLUMN tasks.state IS 'task state';
COMMENT ON COLUMN tasks.payload IS 'task payload';
COMMENT ON COLUMN tasks.result IS 'task result';
COMMENT ON COLUMN tasks.error IS 'task error';
COMMENT ON COLUMN tasks.create_time IS 'task create time';
COMMENT ON COLUMN tasks.next_run_time IS 'task next run time';
COMMENT ON COLUMN tasks.update_time IS 'task update time';
COMMENT ON COLUMN tasks.worker_id IS 'task worker id';
COMMENT ON COLUMN tasks.max_retry IS 'task max retry';
COMMENT ON COLUMN tasks.attempt_no IS 'current attempt number';
COMMENT ON COLUMN tasks.last_heartbeat_time IS 'task last heartbeat time from worker';

CREATE TABLE task_workers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name VARCHAR(128),
  namespace VARCHAR(128) NOT NULL,
  task_type VARCHAR(64) NOT NULL,
  create_time BIGINT NOT NULL DEFAULT 0,
  heartbeat_time BIGINT NOT NULL DEFAULT 0,
  last_work_time BIGINT NOT NULL DEFAULT 0,
  total_dealt BIGINT NOT NULL DEFAULT 0,
  success_dealt BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_task_workers_heartbeat_time
ON task_workers (heartbeat_time);

COMMENT ON TABLE task_workers IS 'task worker table';
COMMENT ON COLUMN task_workers.id IS 'task worker id, primary key';
COMMENT ON COLUMN task_workers.name IS 'task worker name';
COMMENT ON COLUMN task_workers.namespace IS 'task worker namespace';
COMMENT ON COLUMN task_workers.task_type IS 'task worker task type';
COMMENT ON COLUMN task_workers.create_time IS 'task worker create time';
COMMENT ON COLUMN task_workers.heartbeat_time IS 'task worker last heartbeat time';
COMMENT ON COLUMN task_workers.last_work_time IS 'task worker last work handling time';
COMMENT ON COLUMN task_workers.total_dealt IS 'task worker total dealt count';
COMMENT ON COLUMN task_workers.success_dealt IS 'task worker success dealt count';

CREATE TABLE task_events (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  task_id UUID NOT NULL,
  event_type VARCHAR(16) NOT NULL,
  create_time BIGINT NOT NULL DEFAULT 0,
  payload BYTEA
);

CREATE INDEX idx_task_events_task_id
ON task_events (task_id);

COMMENT ON TABLE task_events IS 'task event table';
COMMENT ON COLUMN task_events.id IS 'task event id, primary key';
COMMENT ON COLUMN task_events.task_id IS 'task id';
COMMENT ON COLUMN task_events.event_type IS 'task event type';
COMMENT ON COLUMN task_events.create_time IS 'task event create time';
COMMENT ON COLUMN task_events.payload IS 'task event payload according to event type';
