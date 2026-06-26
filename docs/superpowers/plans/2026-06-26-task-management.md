# Task Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add full task management: submit/get/cancel APIs, task-submitter client, heartbeat-based task liveness, configurable retry and stale detection.

**Architecture:** Incremental changes across api (protobuf), server (service/store/background), and client (new task submitter + worker heartbeat changes). Follows existing layered patterns: proto → store interface → PG impl → service → app registration.

**Tech Stack:** Go 1.25, protobuf + buf, gRPC, GORM + PostgreSQL, golang.org/x/sync/semaphore.

## Global Constraints

- Go version: 1.25.4
- Server module: `github.com/gonotelm-lab/flow/server`
- Client module: `github.com/gonotelm-lab/flow/client`
- API module: `github.com/gonotelm-lab/flow/api`
- Proto generation: `buf generate` via buf.gen.yaml
- DB: PostgreSQL, migrations in `migration/pgsql18.sql`
- Store layer pattern: interface in `store/`, impl in `impl/postgres/`, factory in `impl/new.go`
- Error pattern: `pkg/errors` DomainError, service-level in `service/errors/`
- No new dependencies unless in existing go.mod

---

## File Structure Map

```
api/
├── task/v1/rpc.proto          [CREATE]  TaskService RPC definitions
├── schema/v1/task.proto       [MODIFY]  add last_heartbeat_time
└── worker/v1/rpc.proto        [MODIFY]  extend heartbeat request/response

migration/
└── pgsql18.sql                [MODIFY]  ALTER TABLE tasks ADD last_heartbeat_time

server/internal/
├── config/config.go           [MODIFY]  add stale/retry config fields
├── repository/
│   ├── store/task.go          [MODIFY]  add new store methods
│   ├── schema/task.go         [MODIFY]  add LastHeartbeatTime field
│   └── impl/postgres/task.go  [MODIFY]  implement new store methods
├── service/
│   ├── errors/errors.go       [CREATE]  task service error types
│   ├── task/
│   │   ├── service.go         [CREATE]  TaskService struct
│   │   └── impl.go            [CREATE]  submit/get/cancel logic
│   └── worker/service.go      [MODIFY]  heartbeat signatures
│       └── impl.go            [MODIFY]  heartbeat/report logic
├── app/
│   ├── app.go                 [MODIFY]  start RetryMender, StaleDetector
│   ├── retrymender.go         [CREATE]  RetryMender background goroutine
│   └── staledetector.go       [CREATE]  StaleDetector background goroutine
└── endpoint/apiserver.go      [MODIFY]  register TaskService

client/
├── task/
│   ├── task.go                [CREATE]  Client + Submit/Get/Cancel
│   └── config.go              [CREATE]  SubmitOption types
└── worker/internal/runtime/
    ├── poll.go                [MODIFY]  TryAcquire, pause when full, running IDs, cancel ctx
    ├── heartbeat.go           [MODIFY]  send running IDs, handle cancelled
    └── runtime.go             [MODIFY]  wire shared RunningTasks state
```

---

### Task 1: Proto API Definitions

**Files:**
- Create: `api/task/v1/rpc.proto`
- Modify: `api/schema/v1/task.proto`
- Modify: `api/worker/v1/rpc.proto`

**Produces:** Generated Go code via `buf generate`:
- `api/task/v1/rpc.pb.go`, `api/task/v1/rpc_grpc.pb.go`
- Updated `api/schema/v1/task.pb.go`
- Updated `api/worker/v1/rpc.pb.go`, `api/worker/v1/rpc_grpc.pb.go`

- [ ] **Step 1: Write `api/task/v1/rpc.proto`**

```protobuf
syntax = "proto3";

package api.task.v1;

import "api/schema/v1/task.proto";
import "buf/validate/validate.proto";
import "google/api/field_behavior.proto";

option go_package = "github.com/gonotelm-lab/flow/api/task/v1";

message SubmitRequest {
  string namespace = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.min_len = 1
  ];
  string task_type = 2 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.min_len = 1
  ];
  bytes payload = 3;
  int64 max_retry = 4 [
    (buf.validate.field).int64.gte = 0
  ];
}

message SubmitResponse {
  api.schema.v1.Task task = 1 [(google.api.field_behavior) = OUTPUT_ONLY];
}

message GetRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}

message GetResponse {
  api.schema.v1.Task task = 1 [(google.api.field_behavior) = OUTPUT_ONLY];
}

message CancelRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}

message CancelResponse {}

service TaskService {
  rpc Submit(SubmitRequest) returns (SubmitResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Cancel(CancelRequest) returns (CancelResponse);
}
```

- [ ] **Step 2: Modify `api/schema/v1/task.proto` — add `last_heartbeat_time` field**

Add after line 47 (after `worker_id` field, before closing `}`):

```protobuf
  google.protobuf.Timestamp last_heartbeat_time = 14 [(google.api.field_behavior) = OUTPUT_ONLY];
```

- [ ] **Step 3: Modify `api/worker/v1/rpc.proto` — extend HeartbeatRequest/Response**

Replace `HeartbeatRequest` (lines 22-27):

```protobuf
message HeartbeatRequest {
  int64 id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).int64.gt = 0
  ];
  repeated string running_task_ids = 2;
}
```

Replace `HeartbeatResponse` (lines 29-31):

```protobuf
message HeartbeatResponse {
  google.protobuf.Timestamp heartbeat_time = 1 [(google.api.field_behavior) = OUTPUT_ONLY];
  repeated string cancelled_task_ids = 2 [(google.api.field_behavior) = OUTPUT_ONLY];
}
```

- [ ] **Step 4: Run protobuf code generation**

```bash
cd api && buf generate ../
```

Expected: generates/updates `.pb.go` and `_grpc.pb.go` files without errors.

- [ ] **Step 5: Commit**

```bash
git add api/task/v1/rpc.proto api/schema/v1/task.proto api/worker/v1/rpc.proto api/task/v1/ api/schema/v1/task.pb.go api/worker/v1/rpc.pb.go api/worker/v1/rpc_grpc.pb.go
git commit -m "feat(api): add TaskService RPC, heartbeat task reporting, task last_heartbeat_time"
```

---

### Task 2: Database Migration + Schema

**Files:**
- Modify: `migration/pgsql18.sql`
- Modify: `server/internal/repository/schema/task.go`

**Produces:** Updated task table with `last_heartbeat_time` column, updated GORM model.

- [ ] **Step 1: Add migration SQL**

In `migration/pgsql18.sql`, add after line 62 (after the tasks table CREATE TABLE block, before the first index):

```sql
ALTER TABLE tasks ADD COLUMN last_heartbeat_time BIGINT NOT NULL DEFAULT 0;
```

Add comment after the ALTER:

```sql
COMMENT ON COLUMN tasks.last_heartbeat_time IS 'task last heartbeat time from worker';
```

- [ ] **Step 2: Update GORM schema model**

In `server/internal/repository/schema/task.go`, add after line 18 (after `AttemptNo` field):

```go
	LastHeartbeatTime int64 `gorm:"column:last_heartbeat_time"`
```

- [ ] **Step 3: Verify existing tests still pass**

```bash
cd server && go test ./internal/repository/impl/postgres/... -run TestTask -v -count=1
```

Expected: existing Task tests pass.

- [ ] **Step 4: Commit**

```bash
git add migration/pgsql18.sql server/internal/repository/schema/task.go
git commit -m "feat: add last_heartbeat_time column to tasks table"
```

---

### Task 3: Store Layer — New Methods

**Files:**
- Modify: `server/internal/repository/store/task.go`
- Modify: `server/internal/repository/impl/postgres/task.go`

**Interfaces:**
- Produces: `TaskStoreImpl` methods: `UpdateHeartbeat`, `GetCancelledTasks`, `GetRetriableTasks`, `GetStaleTasks`, `BatchUpdate`

- [ ] **Step 1: Add new interfaces to `store/task.go`**

Add after the existing `TaskUpdateOutcomeParams` block (after line 19):

```go
type TaskBatchUpdateParams struct {
	Id            uuid.UUID
	State         string
	AttemptNo     int
	NextRunTime   int64
	WorkerId      int64
	UpdateTime    int64
	Error         []byte
}
```

Add a `BatchUpdateParams` helper:

```go
type TaskBatchUpdateParams struct {
	State       string
	AttemptNo   int
	NextRunTime int64
	WorkerId    int64
	UpdateTime  int64
	Error       []byte
}
```

Add new methods to the `Task` interface (after line 39):

```go
	// UpdateHeartbeat 批量更新任务心跳时间
	UpdateHeartbeat(ctx context.Context, ids []uuid.UUID, workerId int64, heartTime int64) error

	// GetCancelledTasks 查询已被取消的任务（worker 正在 RUNNING 但 state 已是 CANCELLED）
	GetCancelledTasks(ctx context.Context, workerId int64) ([]uuid.UUID, error)

	// GetRetriableTasks 查询可重试的 FAILED 任务（FOR UPDATE SKIP LOCKED）
	GetRetriableTasks(ctx context.Context, batchSize int) ([]*schema.Task, error)

	// GetStaleTasks 查询失联的 RUNNING 任务（FOR UPDATE SKIP LOCKED）
	GetStaleTasks(ctx context.Context, timeout int64, batchSize int) ([]*schema.Task, error)

	// BatchUpdate 批量更新任务字段
	BatchUpdate(ctx context.Context, tasks []*schema.Task) error
```

Also modify the existing `Task` interface signature for `UpdateHeartbeat` — actually let me reconsider. We need `BatchUpdate` as a general-purpose batch update. Let me simplify:

```go
	// UpdateHeartbeat 批量更新任务心跳时间（仅限 state=RUNNING 且 worker_id 匹配）
	UpdateHeartbeat(ctx context.Context, ids []uuid.UUID, workerId int64, heartTime int64) error

	// GetCancelledTasks 查询指定 worker 的已取消任务
	GetCancelledTasks(ctx context.Context, workerId int64) ([]uuid.UUID, error)

	// GetRetriableTasks 查询可重试的失败任务
	GetRetriableTasks(ctx context.Context, batchSize int) ([]*schema.Task, error)

	// GetStaleTasks 查询失联任务（last_heartbeat_time < now - timeout）
	GetStaleTasks(ctx context.Context, timeout int64, batchSize int) ([]*schema.Task, error)

	// BatchUpdate 批量保存任务变更
	BatchUpdate(ctx context.Context, tasks []*schema.Task) error
```

- [ ] **Step 2: Implement new methods in `impl/postgres/task.go`**

Add after the existing `UpdateOutcome` method (after line 125):

```go
func (s *TaskStoreImpl) UpdateHeartbeat(
	ctx context.Context,
	ids []uuid.UUID,
	workerId int64,
	heartTime int64,
) error {
	db := util.GetDB(ctx, s.db)
	result := db.Model(&schema.Task{}).
		Where("id IN ? AND worker_id = ? AND state = ?", ids, workerId, "RUNNING").
		Update("last_heartbeat_time", heartTime)
	if result.Error != nil {
		return sql.WrapError(result.Error)
	}
	return nil
}

func (s *TaskStoreImpl) GetCancelledTasks(
	ctx context.Context,
	workerId int64,
) ([]uuid.UUID, error) {
	db := util.GetDB(ctx, s.db)
	var ids []uuid.UUID
	if err := db.Model(&schema.Task{}).
		Where("worker_id = ? AND state = ?", workerId, "CANCELLED").
		Pluck("id", &ids).Error; err != nil {
		return nil, sql.WrapError(err)
	}
	return ids, nil
}

func (s *TaskStoreImpl) GetRetriableTasks(
	ctx context.Context,
	batchSize int,
) ([]*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	var tasks []*schema.Task
	err := db.Clauses(clause.Locking{
		Strength: clause.LockingStrengthUpdate,
		Options:  clause.LockingOptionsSkipLocked,
	}).Where("state = ? AND attempt_no < max_retry", "FAILED").
		Order("id ASC").
		Limit(batchSize).
		Find(&tasks).Error
	if err != nil {
		return nil, sql.WrapError(err)
	}
	return tasks, nil
}

func (s *TaskStoreImpl) GetStaleTasks(
	ctx context.Context,
	timeout int64,
	batchSize int,
) ([]*schema.Task, error) {
	db := util.GetDB(ctx, s.db)
	cutoff := time.Now().UnixMilli() - timeout
	var tasks []*schema.Task
	err := db.Clauses(clause.Locking{
		Strength: clause.LockingStrengthUpdate,
		Options:  clause.LockingOptionsSkipLocked,
	}).Where("state = ? AND last_heartbeat_time < ?", "RUNNING", cutoff).
		Order("last_heartbeat_time ASC").
		Limit(batchSize).
		Find(&tasks).Error
	if err != nil {
		return nil, sql.WrapError(err)
	}
	return tasks, nil
}

func (s *TaskStoreImpl) BatchUpdate(
	ctx context.Context,
	tasks []*schema.Task,
) error {
	if len(tasks) == 0 {
		return nil
	}
	db := util.GetDB(ctx, s.db)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, task := range tasks {
			if err := tx.Model(&schema.Task{}).
				Where("id = ?", task.Id).
				Updates(map[string]any{
					"state":               task.State,
					"attempt_no":          task.AttemptNo,
					"next_run_time":       task.NextRunTime,
					"worker_id":           task.WorkerId,
					"update_time":         task.UpdateTime,
					"error":               task.Error,
					"last_heartbeat_time": task.LastHeartbeatTime,
				}).Error; err != nil {
				return sql.WrapError(err)
			}
		}
		return nil
	})
}
```

Add `"time"` to the postgres task.go imports. The file does not currently import `"time"`, but `GetStaleTasks` uses `time.Now().UnixMilli()` which requires it.

- [ ] **Step 3: Compile check**

```bash
cd server && go build ./internal/repository/impl/postgres/...
```

Expected: no compilation errors.

- [ ] **Step 4: Commit**

```bash
git add server/internal/repository/store/task.go server/internal/repository/impl/postgres/task.go
git commit -m "feat(store): add task heartbeat, retry, stale query, batch update methods"
```

---

### Task 4: Server Configuration

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/etc/conf.toml.tpl`

- [ ] **Step 1: Extend `WorkerConfig` in `config.go`**

Replace the existing `WorkerConfig` struct (lines 43-46):

```go
type WorkerConfig struct {
	PollWait           time.Duration `toml:"pollWait"`
	PollCheckInterval  time.Duration `toml:"pollCheckInterval"`
	StaleTaskTimeout   time.Duration `toml:"staleTaskTimeout"`
	StaleScanInterval  time.Duration `toml:"staleScanInterval"`
	RetryScanInterval  time.Duration `toml:"retryScanInterval"`
	RetryScanBatch     int           `toml:"retryScanBatch"`
	StaleScanBatch     int           `toml:"staleScanBatch"`
}
```

- [ ] **Step 2: Update config defaults in the service layer**

Note: defaults will be set in App bootstrap (Task 5/7) not in config validation. The config struct just holds values; zero values get sensible defaults at usage site.

- [ ] **Step 3: Update `conf.toml.tpl`**

Add after line 22 (`pollCheckInterval` line):

```toml
staleTaskTimeout = "${FLOW_WORKER_STALE_TASK_TIMEOUT:-30s}"
staleScanInterval = "${FLOW_WORKER_STALE_SCAN_INTERVAL:-10s}"
retryScanInterval = "${FLOW_WORKER_RETRY_SCAN_INTERVAL:-10s}"
retryScanBatch = ${FLOW_WORKER_RETRY_SCAN_BATCH:-100}
staleScanBatch = ${FLOW_WORKER_STALE_SCAN_BATCH:-100}
```

- [ ] **Step 4: Compile check**

```bash
cd server && go build ./internal/config/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add server/internal/config/config.go server/etc/conf.toml.tpl
git commit -m "feat(config): add stale/retry background task config fields"
```

---

### Task 5: Background Processes — RetryMender + StaleDetector

**Files:**
- Create: `server/internal/app/retrymender.go`
- Create: `server/internal/app/staledetector.go`

**Interfaces:**
- Consumes: `repository.Store` (Task, TaskEvent stores), `config.WorkerConfig`
- Produces: `RetryMender` and `StaleDetector` types with `Run(ctx, store, cfg)` / `Close()` methods

- [ ] **Step 1: Create `retrymender.go`**

```go
package app

import (
	"context"
	"log/slog"
	"min"
	"runtime/debug"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	taskstate "github.com/gonotelm-lab/flow/api/schema/v1"
)

type RetryMender struct {
	store    *repository.Store
	interval time.Duration
	batchSize int
}

func NewRetryMender(store *repository.Store, cfg *config.WorkerConfig) *RetryMender {
	interval := cfg.RetryScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	batchSize := cfg.RetryScanBatch
	if batchSize <= 0 {
		batchSize = 100
	}
	return &RetryMender{
		store:     store,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (r *RetryMender) Run(ctx context.Context) {
	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(ctx, "retry mender panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mend(ctx)
		}
	}
}

func (r *RetryMender) mend(ctx context.Context) {
	tasks, err := r.store.Task.GetRetriableTasks(ctx, r.batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "retry mender get tasks failed", slog.Any("err", err))
		return
	}

	nowMilli := time.Now().UnixMilli()
	for _, task := range tasks {
		task.AttemptNo++
		task.State = taskstate.TaskState_INITED.String()
		task.WorkerId = 0
		task.UpdateTime = nowMilli

		backoff := time.Duration(30*time.Second) * time.Duration(1<<(task.AttemptNo-1))
		if backoff > 10*time.Minute {
			backoff = 10 * time.Minute
		}
		task.NextRunTime = nowMilli + int64(backoff/time.Millisecond)
	}

	if len(tasks) == 0 {
		return
	}

	if err := r.store.Task.BatchUpdate(ctx, tasks); err != nil {
		slog.ErrorContext(ctx, "retry mender batch update failed", slog.Any("err", err))
		return
	}

	for _, task := range tasks {
		_ = r.store.TaskEvent.Append(ctx, &reposchema.TaskEvent{
			TaskId:     task.Id,
			EventType:  "RETRIED",
			CreateTime: nowMilli,
			Payload:    []byte(task.State),
		})
	}
}
```

Note: imports need adjustment. Let me fix the retry mender to not import `min` package — go doesn't have a `min` package. Wait, Go 1.21+ has built-in `min` function. No import needed.

Also the import path for taskstate should be `schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"`. Let me fix:

```go
package app

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type RetryMender struct {
	store     *repository.Store
	interval  time.Duration
	batchSize int
}

func NewRetryMender(store *repository.Store, cfg *config.WorkerConfig) *RetryMender {
	interval := cfg.RetryScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	batchSize := cfg.RetryScanBatch
	if batchSize <= 0 {
		batchSize = 100
	}
	return &RetryMender{
		store:     store,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (r *RetryMender) Run(ctx context.Context) {
	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(ctx, "retry mender panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mend(ctx)
		}
	}
}

func (r *RetryMender) mend(ctx context.Context) {
	tasks, err := r.store.Task.GetRetriableTasks(ctx, r.batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "retry mender get tasks failed", slog.Any("err", err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	nowMilli := time.Now().UnixMilli()
	for _, task := range tasks {
		task.AttemptNo++
		task.State = schemav1.TaskState_INITED.String()
		task.WorkerId = 0
		task.UpdateTime = nowMilli

		shift := task.AttemptNo - 1
		if shift > 10 {
			shift = 10
		}
		backoffMs := 30000 * (1 << shift)
		if backoffMs > 600000 {
			backoffMs = 600000
		}
		task.NextRunTime = nowMilli + int64(backoffMs)
	}

	if err := r.store.Task.BatchUpdate(ctx, tasks); err != nil {
		slog.ErrorContext(ctx, "retry mender batch update failed", slog.Any("err", err))
		return
	}

	for _, task := range tasks {
		_ = r.store.TaskEvent.Append(ctx, &reposchema.TaskEvent{
			TaskId:     task.Id,
			EventType:  "RETRIED",
			CreateTime: nowMilli,
			Payload:    []byte(task.State),
		})
	}
}
```

- [ ] **Step 2: Create `staledetector.go`**

```go
package app

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type StaleDetector struct {
	store     *repository.Store
	interval  time.Duration
	timeout   time.Duration
	batchSize int
}

func NewStaleDetector(store *repository.Store, cfg *config.WorkerConfig) *StaleDetector {
	interval := cfg.StaleScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	timeout := cfg.StaleTaskTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	batchSize := cfg.StaleScanBatch
	if batchSize <= 0 {
		batchSize = 100
	}
	return &StaleDetector{
		store:     store,
		interval:  interval,
		timeout:   timeout,
		batchSize: batchSize,
	}
}

func (d *StaleDetector) Run(ctx context.Context) {
	defer func() {
		if e := recover(); e != nil {
			slog.ErrorContext(ctx, "stale detector panic",
				slog.Any("err", e),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.detect(ctx)
		}
	}
}

func (d *StaleDetector) detect(ctx context.Context) {
	timeoutMs := int64(d.timeout / time.Millisecond)
	tasks, err := d.store.Task.GetStaleTasks(ctx, timeoutMs, d.batchSize)
	if err != nil {
		slog.ErrorContext(ctx, "stale detector get tasks failed", slog.Any("err", err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	nowMilli := time.Now().UnixMilli()
	errMsg := []byte("last heartbeat time exceeded timeout")
	for _, task := range tasks {
		task.State = schemav1.TaskState_FAILED.String()
		task.WorkerId = 0
		task.UpdateTime = nowMilli
		task.Error = errMsg
	}

	if err := d.store.Task.BatchUpdate(ctx, tasks); err != nil {
		slog.ErrorContext(ctx, "stale detector batch update failed", slog.Any("err", err))
		return
	}

	for _, task := range tasks {
		_ = d.store.TaskEvent.Append(ctx, &reposchema.TaskEvent{
			TaskId:     task.Id,
			EventType:  "STALE_DETECTED",
			CreateTime: nowMilli,
			Payload:    errMsg,
		})
	}
}
```

- [ ] **Step 3: Compile check**

```bash
cd server && go build ./internal/app/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add server/internal/app/retrymender.go server/internal/app/staledetector.go
git commit -m "feat(app): add RetryMender and StaleDetector background processes"
```

---

### Task 6: Worker Service Changes (Heartbeat + Report)

**Files:**
- Modify: `server/internal/service/worker/service.go`
- Modify: `server/internal/service/worker/impl.go`
- Create: `server/internal/service/errors/task.go`

**Interfaces:**
- Consumes: updated proto HeartbeatRequest/Response, `repository.Store`
- Produces: enhanced heartbeat with task ID tracking, guarded report

- [ ] **Step 1: Create service error types**

Create `server/internal/service/errors/task.go`:

```go
package errors

import (
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyTaskNotFound       pkgerr.DomainErrorKey = "TASK_NOT_FOUND"
	KeyTaskAlreadyEnded   pkgerr.DomainErrorKey = "TASK_ALREADY_ENDED"
)

var (
	TaskNotFound     = pkgerr.New(codes.NotFound, KeyTaskNotFound)
	TaskAlreadyEnded = pkgerr.New(codes.FailedPrecondition, KeyTaskAlreadyEnded)
)
```

- [ ] **Step 2: Modify heartbeat impl in `impl.go`**

Replace the existing `heartbeat` method (lines 57-65):

```go
func (s *Service) heartbeat(
	ctx context.Context,
	workerId int64,
	runningTaskIds []string,
) (int64, []string, error) {
	heartbeatTime := time.Now().UnixMilli()
	_, err := s.repo.TaskWorker.UpdateHeartbeat(ctx, workerId, heartbeatTime)
	if err != nil {
		return 0, nil, errors.WithMessagef(err, "failed to update task worker heartbeat %d", workerId)
	}

	// Update task heartbeat for running tasks
	if len(runningTaskIds) > 0 {
		ids := make([]uuid.UUID, 0, len(runningTaskIds))
		for _, sid := range runningTaskIds {
			id, err := uuid.Parse(sid)
			if err != nil {
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			if err := s.repo.Task.UpdateHeartbeat(ctx, ids, workerId, heartbeatTime); err != nil {
				slog.ErrorContext(ctx, "update task heartbeat failed",
					"worker_id", workerId,
					slog.Any("err", err),
				)
			}
		}
	}

	// Check for cancelled tasks
	cancelledIds, err := s.repo.Task.GetCancelledTasks(ctx, workerId)
	if err != nil {
		return 0, nil, errors.WithMessagef(err, "failed to get cancelled tasks for worker %d", workerId)
	}

	cancelledStrs := make([]string, 0, len(cancelledIds))
	for _, id := range cancelledIds {
		cancelledStrs = append(cancelledStrs, id.String())
	}

	return heartbeatTime, cancelledStrs, nil
}
```

Add `"log/slog"` to imports if not already present.

- [ ] **Step 3: Modify `Heartbeat` handler in `service.go`**

Replace the `Heartbeat` method (lines 85-101):

```go
func (s *Service) Heartbeat(
	ctx context.Context,
	req *workerv1.HeartbeatRequest,
) (*workerv1.HeartbeatResponse, error) {
	if req.GetId() == 0 {
		return nil, pkgerr.InvalidArgument.WithDetail("id is required")
	}

	heartbeatMs, cancelledIds, err := s.heartbeat(ctx, req.GetId(), req.GetRunningTaskIds())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to heartbeat worker %d", req.GetId())
	}

	return &workerv1.HeartbeatResponse{
		HeartbeatTime:    timestamppb.New(time.UnixMilli(heartbeatMs)),
		CancelledTaskIds: cancelledIds,
	}, nil
}
```

- [ ] **Step 4: Modify `report` impl in `impl.go` — add state guard**

Replace the `report` method (lines 114-155):

```go
func (s *Service) report(ctx context.Context, req *workerv1.ReportRequest) error {
	taskId, err := uuid.Parse(req.GetTaskId())
	if err != nil {
		return pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	// Fetch current task state to guard against stale reports
	task, err := s.repo.Task.Get(ctx, taskId)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil // task deleted, silently ignore
		}
		return errors.WithMessagef(err, "failed to get task %s", taskId)
	}

	// Only accept reports for RUNNING tasks (ignore cancelled/stale/reset)
	if task.State != schemav1.TaskState_RUNNING.String() {
		return nil
	}

	var (
		success  bool
		newState string
		oldState = schemav1.TaskState_RUNNING.String()
		nowMilli = time.Now().UnixMilli()
	)

	switch req.GetAction() {
	case workerv1.ReportAction_SUCCESS:
		success = true
		newState = schemav1.TaskState_DONE.String()
	case workerv1.ReportAction_FAIL:
		success = false
		newState = schemav1.TaskState_FAILED.String()
	default:
		return pkgerr.InvalidArgument.WithDetail("action is invalid")
	}

	_, err = s.repo.Task.UpdateOutcome(
		ctx,
		taskId,
		success,
		req.GetWorkerId(),
		oldState,
		newState,
		&store.TaskUpdateOutcomeParams{
			Payload:    req.GetPayload(),
			UpdateTime: nowMilli,
		},
	)
	if err != nil {
		return errors.WithMessagef(err, "failed to update task outcome %s", taskId)
	}

	return nil
}
```

- [ ] **Step 5: Update `toProtoTask` in `service.go`**

Add `LastHeartbeatTime` to the proto conversion (after line 215):

```go
		LastHeartbeatTime: timestamppb.New(time.UnixMilli(task.LastHeartbeatTime)),
```

- [ ] **Step 6: Compile check**

```bash
cd server && go build ./internal/service/worker/... ./internal/service/errors/...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add server/internal/service/worker/service.go server/internal/service/worker/impl.go server/internal/service/errors/task.go
git commit -m "feat(worker): enhance heartbeat with task IDs, add report state guard"
```

---

### Task 7: Task Service + Registration

**Files:**
- Create: `server/internal/service/task/service.go`
- Create: `server/internal/service/task/impl.go`
- Modify: `server/internal/endpoint/apiserver.go`
- Modify: `server/internal/app/app.go`

**Interfaces:**
- Consumes: `*repository.Store`, `*config.WorkerConfig`, generated `taskv1.TaskServiceServer`
- Produces: Registered TaskService on gRPC, background processes started

- [ ] **Step 1: Create `service/task/service.go`**

```go
package task

import (
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

type Service struct {
	taskv1.UnimplementedTaskServiceServer

	repo *repository.Store
}

func NewService(repo *repository.Store) taskv1.TaskServiceServer {
	return &Service{repo: repo}
}
```

- [ ] **Step 2: Create `service/task/impl.go`**

```go
package task

import (
	"context"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) Submit(
	ctx context.Context,
	req *taskv1.SubmitRequest,
) (*taskv1.SubmitResponse, error) {
	namespace := req.GetNamespace()
	_, err := s.repo.Namespace.Get(ctx, namespace)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.NamespaceNotFound
		}
		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate task id")
	}

	nowMilli := time.Now().UnixMilli()
	task := &reposchema.Task{
		Id:          id,
		Namespace:   namespace,
		TaskType:    req.GetTaskType(),
		Payload:     req.GetPayload(),
		State:       schemav1.TaskState_INITED.String(),
		CreateTime:  nowMilli,
		NextRunTime: nowMilli,
		UpdateTime:  nowMilli,
		MaxRetry:    int(req.GetMaxRetry()),
		AttemptNo:   0,
		WorkerId:    0,
	}

	created, err := s.repo.Task.Create(ctx, task)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create task")
	}

	_ = s.repo.TaskEvent.Append(ctx, &reposchema.TaskEvent{
		TaskId:     created.Id,
		EventType:  schemav1.TaskState_INITED.String(),
		CreateTime: nowMilli,
	})

	return &taskv1.SubmitResponse{Task: toProtoTask(created)}, nil
}

func (s *Service) Get(
	ctx context.Context,
	req *taskv1.GetRequest,
) (*taskv1.GetResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.repo.Task.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.TaskNotFound
		}
		return nil, errors.WithMessage(err, "failed to get task")
	}

	return &taskv1.GetResponse{Task: toProtoTask(task)}, nil
}

func (s *Service) Cancel(
	ctx context.Context,
	req *taskv1.CancelRequest,
) (*taskv1.CancelResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerr.InvalidArgument.WithDetail("task_id is invalid")
	}

	task, err := s.repo.Task.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pkgerr.NoRecord) {
			return nil, srverr.TaskNotFound
		}
		return nil, errors.WithMessage(err, "failed to get task")
	}

	// Only INITED or RUNNING can be cancelled
	if task.State != schemav1.TaskState_INITED.String() &&
		task.State != schemav1.TaskState_RUNNING.String() {
		return nil, srverr.TaskAlreadyEnded.WithDetail(
			"task state: " + task.State,
		)
	}

	nowMilli := time.Now().UnixMilli()
	task.State = schemav1.TaskState_CANCELLED.String()
	task.UpdateTime = nowMilli

	ok, err := s.repo.Task.Update(ctx, task)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to cancel task")
	}
	if !ok {
		return nil, srverr.TaskAlreadyEnded
	}

	_ = s.repo.TaskEvent.Append(ctx, &reposchema.TaskEvent{
		TaskId:     task.Id,
		EventType:  schemav1.TaskState_CANCELLED.String(),
		CreateTime: nowMilli,
	})

	return &taskv1.CancelResponse{}, nil
}

func toProtoTask(task *reposchema.Task) *schemav1.Task {
	if task == nil {
		return nil
	}

	state := schemav1.TaskState_TASK_STATE_UNSPECIFIED
	if rawState, ok := schemav1.TaskState_value[task.State]; ok {
		state = schemav1.TaskState(rawState)
	}

	return &schemav1.Task{
		Id:                 task.Id.String(),
		Namespace:          task.Namespace,
		TaskType:           task.TaskType,
		Payload:            task.Payload,
		Result:             task.Result,
		Error:              task.Error,
		State:              state,
		CreateTime:         timestamppb.New(time.UnixMilli(task.CreateTime)),
		UpdateTime:         timestamppb.New(time.UnixMilli(task.UpdateTime)),
		NextRunTime:        task.NextRunTime,
		MaxRetry:           int64(task.MaxRetry),
		AttemptNo:          int32(task.AttemptNo),
		WorkerId:           task.WorkerId,
		LastHeartbeatTime:  timestamppb.New(time.UnixMilli(task.LastHeartbeatTime)),
	}
}
```

- [ ] **Step 3: Register TaskService in `endpoint/apiserver.go`**

Modify `registerGrpcServices` to add TaskService registration. After the WorkerService registration block (after line 90):

```go
	taskService := task.NewService(repoStore)
	taskv1.RegisterTaskServiceServer(s.grpcServer, taskService)
```

Add import for `taskv1 "github.com/gonotelm-lab/flow/api/task/v1"` and `"github.com/gonotelm-lab/flow/server/internal/service/task"`.

- [ ] **Step 4: Start background processes in `app/app.go`**

In `bootstrap()` method (after `a.startInstanceWatch()`, around line 88), add:

```go
	if config.Conf.Worker != nil {
		retryMender := NewRetryMender(repoStore, config.Conf.Worker)
		go retryMender.Run(a.rootCtx)

		staleDetector := NewStaleDetector(repoStore, config.Conf.Worker)
		go staleDetector.Run(a.rootCtx)
	}
```

`repoStore` is a `*repository.Store` — check how it's available in `bootstrap()`. Looking at `app.go`, `New()` receives `*repository.Impl` which has a `.Store()` method. Let me check...

Actually, `New` receives `*repository.Impl`. Let me look at what `repository.Impl` exposes. From the exploration, `repository.Impl` wraps a `*repository.Store`. Let me check.

Let me look at the existing `New` signature: `func New(repo *repository.Impl) (*App, error)`. And it accesses `repo.TxManager()` and `repo.Store()`. So `bootstrap()` uses... wait, `bootstrap()` doesn't have access to `repo`. The bootstrap/hooks pattern uses `repo *repository.Impl` available from `New`.

Actually looking at `app.go` again: `New(repo *repository.Impl)` — and it passes `repo.Store()` into `endpoint.NewApiServer(...)`. In `bootstrap()`, we need the store. `bootstrap()` doesn't have access to `repo` directly. We need to store it in the `App` struct.

Add `repo *repository.Impl` to `App` struct and save it in `New`:

```go
type App struct {
	// ... existing fields ...
	repo *repository.Impl   // NEW
}
```

In `New`, after `a.ready.Store(false)` (or after initializing `a`):

```go
	a.repo = repo
```

Then in `bootstrap()`:

```go
	store := a.repo.Store()
	if config.Conf.Worker != nil {
		retryMender := NewRetryMender(store, config.Conf.Worker)
		go retryMender.Run(a.rootCtx)

		staleDetector := NewStaleDetector(store, config.Conf.Worker)
		go staleDetector.Run(a.rootCtx)
	}
```

Actually wait, let me re-read app.go. The `New` function gets `*repository.Impl`, not `*repository.Store`. And `endpoint.NewApiServer` receives `repo.Store()` (the `*repository.Store`). So we need to store `repo *repository.Impl` on App to access `.Store()` in bootstrap. The background processes take `*repository.Store`.

But actually, the `RetryMender` and `StaleDetector` don't need the full `*repository.Impl`, just `*repository.Store`. So we could pass the store directly. Either way works. Let me store `repo` for simplicity.

Wait, I don't want to change the App struct signature too much. Let me instead have `NewRetryMender` and `NewStaleDetector` receive `*repository.Store` and call `.Store()` when passing.

Let me re-read App to make sure the store is accessible.

In `app.go` line 38: `func New(repo *repository.Impl) (*App, error)` 
Line 48: `repo.TxManager()`
Line 49: `repo.Store()`
Line 76: `repo.Store()` passed to `endpoint.NewApiServer`

But in `bootstrap()`, we don't have `repo` accessible. So we need to add it to `App`:

```go
type App struct {
	rootCtx    context.Context
	rootCancel context.CancelFunc

	registry *instance.Registry
	sweeper  *instance.Sweeper
	watcher  *instance.Watcher

	self      *instance.Instance
	selfShard atomic.Pointer[sharding.Shard]
	shardCalc sharding.Calculator
	ready     atomic.Bool

	apiServer *endpoint.ApiServer

	store *repository.Store  // NEW
}
```

In `New`:

```go
a := &App{
	registry:  registry,
	sweeper:   sweeper,
	watcher:   watcher,
	shardCalc: &sharding.SequentialCalculator{},
	store:     repo.Store(),  // NEW
}
```

In `bootstrap()`:

```go
if config.Conf.Worker != nil {
	retryMender := NewRetryMender(a.store, config.Conf.Worker)
	go retryMender.Run(a.rootCtx)

	staleDetector := NewStaleDetector(a.store, config.Conf.Worker)
	go staleDetector.Run(a.rootCtx)
}
```

This is clean. Let me now re-do the plan step with all these corrections.

- [ ] **Step 4: Add `store` field to `App` and wire background processes**

In `app.go`, modify the `App` struct to add `store` field. After line 34 (after `apiServer`):

```go
	store     *repository.Store
```

In the `New` function, after `a := &App{` (around line 65+), add the store field:

```go
		store:     repo.Store(),
```

In `bootstrap()`, after `a.startInstanceWatch()` (after line 88), add:

```go
	if config.Conf.Worker != nil {
		retryMender := NewRetryMender(a.store, config.Conf.Worker)
		go retryMender.Run(a.rootCtx)

		staleDetector := NewStaleDetector(a.store, config.Conf.Worker)
		go staleDetector.Run(a.rootCtx)
	}
```

- [ ] **Step 5: Compile check**

```bash
cd server && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add server/internal/service/task/ server/internal/endpoint/apiserver.go server/internal/app/app.go
git commit -m "feat(server): add TaskService, register on gRPC, start retry/stale detectors"
```

---

### Task 8: Client — Task Submitter

**Files:**
- Create: `client/task/task.go`
- Create: `client/task/config.go`

**Interfaces:**
- Consumes: `taskv1.TaskServiceClient` (gRPC generated)
- Produces: `Client` with `Submit`, `Get`, `Cancel`, `Close`

- [ ] **Step 1: Create `client/task/config.go`**

```go
package task

type submitOptions struct {
	maxRetry int
}

type SubmitOption func(*submitOptions)

func WithMaxRetry(n int) SubmitOption {
	return func(o *submitOptions) {
		o.maxRetry = n
	}
}
```

- [ ] **Step 2: Create `client/task/task.go`**

```go
package task

import (
	"context"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn     grpc.ClientConnInterface
	client   taskv1.TaskServiceClient
	ownsConn bool
}

func New(addr string, opts ...grpc.DialOption) (*Client, error) {
	baseOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	baseOpts = append(baseOpts, opts...)

	conn, err := grpc.NewClient(addr, baseOpts...)
	if err != nil {
		return nil, err
	}

	c := NewWithConn(conn)
	c.ownsConn = true
	return c, nil
}

func NewWithConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		conn:   conn,
		client: taskv1.NewTaskServiceClient(conn),
	}
}

func (c *Client) Submit(
	ctx context.Context,
	namespace, taskType string,
	payload []byte,
	opts ...SubmitOption,
) (*schemav1.Task, error) {
	o := &submitOptions{}
	for _, opt := range opts {
		opt(o)
	}

	resp, err := c.client.Submit(ctx, &taskv1.SubmitRequest{
		Namespace: namespace,
		TaskType:  taskType,
		Payload:   payload,
		MaxRetry:  int64(o.maxRetry),
	})
	if err != nil {
		return nil, err
	}
	return resp.GetTask(), nil
}

func (c *Client) Get(ctx context.Context, taskID string) (*schemav1.Task, error) {
	resp, err := c.client.Get(ctx, &taskv1.GetRequest{Id: taskID})
	if err != nil {
		return nil, err
	}
	return resp.GetTask(), nil
}

func (c *Client) Cancel(ctx context.Context, taskID string) error {
	_, err := c.client.Cancel(ctx, &taskv1.CancelRequest{Id: taskID})
	return err
}

func (c *Client) Close() error {
	if c.ownsConn {
		if cc, ok := c.conn.(*grpc.ClientConn); ok {
			return cc.Close()
		}
	}
	return nil
}
```

- [ ] **Step 3: Compile check**

```bash
cd client && go build ./task/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add client/task/
git commit -m "feat(client): add task submitter client with Submit/Get/Cancel"
```

---

### Task 9: Client — Worker Changes (Poll Pause + Heartbeat Tasks)

**Files:**
- Modify: `client/worker/internal/runtime/poll.go`
- Modify: `client/worker/internal/runtime/heartbeat.go`
- Modify: `client/worker/internal/runtime/runtime.go`

**Interfaces:**
- Consumes: updated `workerv1.WorkerServiceClient` with new heartbeat signatures
- Produces: `Semaphore.TryAcquire`, `PollLoop.RunningTaskIDs`, cancelled task abortion via context cancel

- [ ] **Step 1: Add `TryAcquire` to Semaphore in `poll.go`**

After the existing `Acquire` method (after line 33), add:

```go
func (s *Semaphore) TryAcquire() bool {
	return s.sem.TryAcquire(1)
}

func (s *Semaphore) HasCapacity() bool {
	return s.sem.TryAcquire(1)
}
```

Wait, `TryAcquire` would consume a slot if it succeeds. We need a non-blocking check and a way to release without the WaitGroup being affected. Let me think...

The issue: before calling Poll, we want to check if there's capacity. If `TryAcquire()` succeeds, we've taken a slot — but we haven't started a task yet. We need to `Release()` if we change our mind, but `Release()` calls `wg.Done()` which would fail because we never called `wg.Add(1)`.

So I need to separate the semaphore check from the WaitGroup tracking:

```go
func (s *Semaphore) TryAcquire() bool {
	return s.sem.TryAcquire(1)
}
```

And modify `Acquire` to:
```go
func (s *Semaphore) Acquire(ctx context.Context) error {
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	s.wg.Add(1)
	return nil
}
```

Wait, the current code already does this. `TryAcquire` just calls `s.sem.TryAcquire(1)` without `wg.Add(1)`. But `Release()` calls both `s.sem.Release(1)` and `s.wg.Done()`.

If someone calls `TryAcquire()` → succeeds → then calls `Release()`, `wg.Done()` would panic.

Better approach: `TryAcquire` is only used as a capacity check before calling Poll, and we never `Release` after a pure capacity check. In the poll loop, the flow is:

```
if !sem.TryAcquire() { sleep; continue }  // Just check, don't hold
sem.Release()  // We wouldn't call this!
sem.Acquire(ctx)  // Actually acquire + wg.Add(1)
```

Wait no — the flow should be cleaner. Let me re-think:

Option A: Separate `Available() bool` and `TryAcquire() (bool)` — keep things simple.

Actually, the simplest approach: use a count check. The semaphore has a `TryAcquire` method in the `x/sync/semaphore` package already. If we call it and get `true`, we've taken a slot. We can just return it immediately after — the point is to not call Poll if there's no capacity. We shouldn't hold the slot across the Poll call.

Let me do this properly:

```go
func (s *Semaphore) IsFull() bool {
	return !s.sem.TryAcquire(1)
}
```

If `TryAcquire` succeeds, we release the unused slot immediately:
```go
func (s *Semaphore) IsFull() bool {
	if s.sem.TryAcquire(1) {
		s.sem.Release(1)
		return false
	}
	return true
}
```

And `Acquire` stays as is with `wg.Add(1)`.

In the poll loop:
```go
if p.cfg.Semaphore.IsFull() {
	select {
	case <-ctx.Done(): return
	case <-time.After(100 * time.Millisecond):
	}
	continue
}
```

This is clean. `IsFull` atomically checks and releases the temp slot.

- [ ] **Step 2: Add `RunningTaskIDs` + cancel map to `PollLoop`**

Add to `PollLoopConfig` struct (after `Logger` field):

```go
	CancelFuncs map[string]context.CancelFunc
	RunningIDs  *sync.Map  // taskID -> struct{}
```

Actually, `PollLoop` already has access to the config. Let me add a mutex-protected map:

Modify `PollLoop` struct:

```go
type PollLoop struct {
	cfg    PollLoopConfig
	client workerv1.WorkerServiceClient

	mu           sync.Mutex
	runningIDs   map[string]struct{}
	cancelFuncs  map[string]context.CancelFunc
}
```

Add `RunningTaskIDs()` and `CancelTask()` methods:

```go
func (p *PollLoop) RunningTaskIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.runningIDs))
	for id := range p.runningIDs {
		ids = append(ids, id)
	}
	return ids
}

func (p *PollLoop) CancelTask(taskID string) {
	p.mu.Lock()
	cancel, ok := p.cancelFuncs[taskID]
	p.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}
```

Initialize in `NewPollLoop`:

```go
func NewPollLoop(cfg PollLoopConfig) *PollLoop {
	return &PollLoop{
		cfg:         cfg,
		client:      workerv1.NewWorkerServiceClient(cfg.Conn),
		runningIDs:  make(map[string]struct{}),
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}
```

- [ ] **Step 3: Modify `Run` in `PollLoop` to pause when full**

Replace the `Run` method:

```go
func (p *PollLoop) Run(ctx context.Context) {
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		if p.cfg.Semaphore.IsFull() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		resp, err := p.client.Poll(ctx, &workerv1.PollRequest{
			Id:        p.cfg.WorkerID,
			Namespace: p.cfg.Namespace,
			TaskType:  p.cfg.TaskType,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.cfg.Logger.Error("poll failed", "err", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		task := resp.GetTask()
		if task == nil || task.GetId() == "" {
			continue
		}

		if err := p.cfg.Semaphore.Acquire(ctx); err != nil {
			return
		}

		taskCopy := task
		go p.runTask(ctx, taskCopy)
	}
}
```

- [ ] **Step 4: Modify `runTask` to track running IDs and support cancellation**

Replace `runTask`:

```go
func (p *PollLoop) runTask(ctx context.Context, task *schemav1.Task) {
	defer p.cfg.Semaphore.Release()

	taskID := task.GetId()
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	p.mu.Lock()
	p.runningIDs[taskID] = struct{}{}
	p.cancelFuncs[taskID] = cancel
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.runningIDs, taskID)
		delete(p.cancelFuncs, taskID)
		p.mu.Unlock()
	}()

	defer func() {
		if r := recover(); r != nil {
			p.cfg.Logger.Error("task handler panic",
				"task_id", taskID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
			_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, workerv1.ReportAction_FAIL, []byte("panic"))
		}
	}()

	p.cfg.Logger.Info("task started", "task_id", taskID)
	action, payload := p.cfg.Handler(taskCtx, task)
	if taskCtx.Err() == nil {
		p.cfg.Logger.Info("task finished", "task_id", taskID, "action", action.String())
		_ = p.cfg.Reporter.ReportTask(ctx, p.cfg.WorkerID, task, action, payload)
	}
}
```

Note: pass `taskCtx` to the handler — when cancelled via `CancelTask`, the handler gets `ctx.Done()`. When context is cancelled, skip reporting (the task was cancelled, result doesn't matter).

- [ ] **Step 5: Modify `HeartbeatLoop` to send running IDs and handle cancelled**

Modify `HeartbeatLoop` struct:

```go
type HeartbeatLoop struct {
	client       workerv1.WorkerServiceClient
	workerID     int64
	interval     time.Duration
	logger       *slog.Logger
	runningIDs   func() []string    // NEW
	onCancelled  func([]string)     // NEW
}
```

Update `NewHeartbeatLoop`:

```go
func NewHeartbeatLoop(
	conn grpc.ClientConnInterface,
	workerID int64,
	interval time.Duration,
	logger *slog.Logger,
	runningIDs func() []string,
	onCancelled func([]string),
) *HeartbeatLoop {
	return &HeartbeatLoop{
		client:      workerv1.NewWorkerServiceClient(conn),
		workerID:    workerID,
		interval:    interval,
		logger:      logger,
		runningIDs:  runningIDs,
		onCancelled: onCancelled,
	}
}
```

Update `Run`:

```go
func (h *HeartbeatLoop) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runningIDs := h.runningIDs()
			resp, err := h.client.Heartbeat(ctx, &workerv1.HeartbeatRequest{
				Id:             h.workerID,
				RunningTaskIds: runningIDs,
			})
			if err != nil {
				h.logger.Error("heartbeat failed", "worker_id", h.workerID, "err", err)
				continue
			}

			if cancelled := resp.GetCancelledTaskIds(); len(cancelled) > 0 {
				h.logger.Info("received cancelled tasks", "task_ids", cancelled)
				h.onCancelled(cancelled)
			}
		}
	}
}
```

- [ ] **Step 6: Update `Runtime` to wire heartbeat and poll together**

Modify `runtime.go` — update `Start` to pass `runningIDs` and `onCancelled` callbacks:

Replace the heartbeat/poll creation block (lines 75-85):

```go
	r.hb = NewHeartbeatLoop(r.cfg.Conn, r.workerID, r.cfg.HeartbeatInterval, r.cfg.Logger,
		func() []string { return r.poll.RunningTaskIDs() },
		func(ids []string) {
			for _, id := range ids {
				r.poll.CancelTask(id)
			}
		},
	)
	r.poll = NewPollLoop(PollLoopConfig{
		Conn:      r.cfg.Conn,
		WorkerID:  r.workerID,
		Namespace: r.cfg.Namespace,
		TaskType:  r.cfg.TaskType,
		Handler:   r.cfg.Handler,
		Reporter:  reporter,
		Semaphore: r.sem,
		Logger:    r.cfg.Logger,
	})
```

Wait — `r.poll` is created after `r.hb`, but `r.hb` references `r.poll.RunningTaskIDs()` and `r.poll.CancelTask()`. This is a chicken-and-egg problem. Create `r.poll` first, then `r.hb`.

```go
	r.poll = NewPollLoop(PollLoopConfig{
		Conn:      r.cfg.Conn,
		WorkerID:  r.workerID,
		Namespace: r.cfg.Namespace,
		TaskType:  r.cfg.TaskType,
		Handler:   r.cfg.Handler,
		Reporter:  reporter,
		Semaphore: r.sem,
		Logger:    r.cfg.Logger,
	})
	r.hb = NewHeartbeatLoop(r.cfg.Conn, r.workerID, r.cfg.HeartbeatInterval, r.cfg.Logger,
		func() []string { return r.poll.RunningTaskIDs() },
		func(ids []string) {
			for _, id := range ids {
				r.poll.CancelTask(id)
			}
		},
	)
```

- [ ] **Step 7: Compile check**

```bash
cd client && go build ./worker/...
```

Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add client/worker/internal/runtime/poll.go client/worker/internal/runtime/heartbeat.go client/worker/internal/runtime/runtime.go
git commit -m "feat(worker): pause poll when full, heartbeat with task IDs, cancel support"
```

---

## Plan Completion Checklist

- [x] Task 1: Proto API Definitions
- [x] Task 2: Database Migration + Schema
- [x] Task 3: Store Layer — New Methods
- [x] Task 4: Server Configuration
- [x] Task 5: Background Processes
- [x] Task 6: Worker Service Changes
- [x] Task 7: Task Service + Registration
- [x] Task 8: Client — Task Submitter
- [x] Task 9: Client — Worker Changes
