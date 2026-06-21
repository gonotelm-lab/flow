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
