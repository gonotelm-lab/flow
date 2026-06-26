# Task Management Design

> Date: 2026-06-26
> Status: approved

## Overview

Add full task management capabilities to the flow distributed task queue:
task submission, query, cancellation, heartbeat-based liveness tracking,
automatic retry, and stale task detection.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| TaskService deployment | TCP gRPC (same as WorkerService) | External submitters connect via TCP |
| Heartbeat task reporting | Full `running_task_ids` list | Direct, no extra server query |
| Retry behavior | Backoff delay via `next_run_time` | Configurable, no immediate re-claim |
| Stale timeout | Configurable via `WorkerConfig` | Operator control |
| Cancel semantics | Mark CANCELLED, worker discovers on heartbeat | Simple, no push notification needed |
| Background processes | App layer (like sweeper/watcher) | Consistent with existing architecture |
| Submit options | Functional options pattern | Extensible, Go idiomatic |

---

## 1. API Layer (Protobuf)

### 1.1 `api/task/v1/rpc.proto` — New TaskService

```protobuf
service TaskService {
  rpc Submit(SubmitRequest) returns (SubmitResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Cancel(CancelRequest) returns (CancelResponse);
}
```

- **Submit**: namespace, task_type, payload, max_retry (optional). Returns created Task.
- **Get**: task_id (UUID). Returns Task or NotFound.
- **Cancel**: task_id (UUID). Allowed only for INITED/RUNNING states; terminal states return FailedPrecondition.

### 1.2 `api/schema/v1/task.proto` — Task add field

```protobuf
google.protobuf.Timestamp last_heartbeat_time = 14 [(google.api.field_behavior) = OUTPUT_ONLY];
```

### 1.3 `api/worker/v1/rpc.proto` — Heartbeat extension

```protobuf
message HeartbeatRequest {
  int64 id = 1;
  repeated string running_task_ids = 2;   // NEW
}

message HeartbeatResponse {
  google.protobuf.Timestamp heartbeat_time = 1;
  repeated string cancelled_task_ids = 2; // NEW
}
```

---

## 2. Server — TaskService

### 2.1 `server/internal/service/task/`

New service package implementing `taskv1.TaskServiceServer`.

```
server/internal/service/task/
├── service.go       # Service struct
└── impl.go          # submit, get, cancel logic
```

#### Submit

1. Validate namespace exists (via Namespace store)
2. Generate task ID (UUID v7)
3. `next_run_time = now`, state = `INITED`, attempt_no = 0, worker_id = 0
4. Create Task in DB
5. Append TaskEvent (event_type: INITED)
6. Return Task proto

#### Get

1. Parse UUID, query Task store
2. Return Task or NotFound

#### Cancel

1. Parse UUID, query Task store
2. State check: only INITED or RUNNING can be cancelled
3. CAS update state to CANCELLED
4. Append TaskEvent (event_type: CANCELLED)

### 2.2 gRPC Registration

In `endpoint/apiserver.go`, register `TaskService` alongside `WorkerService` on the same TCP gRPC server.

---

## 3. Server — WorkerService Changes

### 3.1 Heartbeat Enhancement

```
heartbeat(ctx, workerId, runningTaskIds) -> (heartbeatTime, cancelledTaskIds, err)
```

1. Update `task_workers.heartbeat_time` (unchanged)
2. **NEW**: Batch update `tasks.last_heartbeat_time = now` for all `running_task_ids`
   WHERE worker_id matches and state = RUNNING
3. **NEW**: Query tasks WHERE worker_id = ? AND state = CANCELLED, return as `cancelled_task_ids`
   Clear `worker_id` on these tasks (detach from dead worker)
4. Return heartbeat time + cancelled list

### 3.2 Report Enhancement

Add pre-check: if `task.state != RUNNING`, silently ignore the report (task may have been
cancelled or reset by stale detector). Only proceed with CAS `RUNNING -> DONE/FAILED` if
task is still in RUNNING state.

### 3.3 Poll (No Change)

Poll uses `FOR UPDATE SKIP LOCKED` with CAS state transition. No changes needed.

---

## 4. Server — Background Processes (App Layer)

### 4.1 RetryMender

```
server/internal/app/retrymender.go
```

**Config:**
- `Interval`: scan interval (default 10s)
- `BatchSize`: max tasks per scan (default 100)

**Logic:**
1. Query: `SELECT * FROM tasks WHERE state = 'FAILED' AND attempt_no < max_retry ORDER BY id LIMIT ? FOR UPDATE SKIP LOCKED`
2. For each task:
   - `attempt_no++`
   - `next_run_time = now + backoff` where backoff = `min(30s * 2^(attempt_no-1), 10min)`
   - Reset state to `INITED`, `worker_id = 0`
   - Append TaskEvent (event_type: RETRIED)
3. Batch commit

### 4.2 StaleDetector

```
server/internal/app/staledetector.go
```

**Config:**
- `Interval`: scan interval (default 10s)
- `Timeout`: staleness threshold (default 30s = 6x heartbeat interval)
- `BatchSize`: max tasks per scan (default 100)

**Logic:**
1. Query: `SELECT * FROM tasks WHERE state = 'RUNNING' AND last_heartbeat_time < (now - timeout) ORDER BY last_heartbeat_time ASC LIMIT ? FOR UPDATE SKIP LOCKED`
2. For each task:
   - Set state to `FAILED`, record error info
   - Clear `worker_id`
   - Append TaskEvent (event_type: STALE_DETECTED)
3. Batch commit

> Design note: Stale tasks transition to FAILED (not INITED). The RetryMender will
> pick them up in the next cycle if `max_retry > 0`. If `max_retry = 0`, they stay FAILED.

### 4.3 Configuration

Add to `WorkerConfig`:
```go
StaleTaskTimeout  time.Duration  // staleness threshold
StaleScanInterval time.Duration  // stale scanner interval
RetryScanInterval time.Duration  // retry scanner interval
RetryScanBatch    int
StaleScanBatch    int
```

### 4.4 Startup

In `App.bootstrap()`, start both goroutines with root context cancellation support
and panic recovery (following the existing instance watcher pattern).

---

## 5. Client — Task Submitter

### 5.1 `client/task/`

```
client/task/
├── task.go      # Client + public API
└── config.go    # Config
```

### 5.2 API

```go
type Client struct { ... }

func New(addr string) (*Client, error)
func NewWithConn(conn grpc.ClientConnInterface) *Client

// Submit creates a task. Uses functional options for non-essential params.
func (c *Client) Submit(ctx context.Context, namespace, taskType string,
    payload []byte, opts ...SubmitOption) (*schemav1.Task, error)

// Get retrieves task by ID.
func (c *Client) Get(ctx context.Context, taskID string) (*schemav1.Task, error)

// Cancel cancels a non-terminal task.
func (c *Client) Cancel(ctx context.Context, taskID string) error

func (c *Client) Close() error

// Options
type SubmitOption func(*submitOptions)
func WithMaxRetry(n int) SubmitOption
```

### 5.3 Design Notes

- No typed generic helpers (caller handles serialization)
- `Submit` returns full Task proto — caller uses `taskID` for subsequent `Get` polling
- Connection management: `ownsConn` flag, `Close()` cleans up
- Independent of `client/worker/` — separate concerns (submitter vs executor)

---

## 6. Client — Worker Changes

### 6.1 Poll Pause When Concurrency Full

`Semaphore` gains `TryAcquire()` for non-blocking capacity check.
`PollLoop.Run` checks capacity before calling `Poll`:
- If `!TryAcquire()`, sleep briefly and retry (no server round-trip wasted)
- Heartbeat continues independently

### 6.2 Heartbeat with Task IDs

`PollLoop` tracks running task IDs via a mutex-protected map.
`HeartbeatLoop` reads running IDs on each tick, packages into `HeartbeatRequest`,
and handles `cancelled_task_ids` from the response.

### 6.3 Cancelled Task Abortion

- `PollLoop` maintains a `map[string]context.CancelFunc` for active tasks
- On heartbeat response with cancelled IDs, call corresponding `cancel()`
- The task handler goroutine receives `ctx.Done()` and exits
- Task result is ignored (server-side Report already handles non-RUNNING state)

### 6.4 Communication Between Heartbeat and Poll Loops

Shared `RunningTasks` structure (mutex-protected) bridges heartbeat and poll loops:
- Poll writes task IDs on start, removes on finish
- Heartbeat reads for request, writes cancelled signals

No new channels needed — reuse existing `sync.Mutex` patterns.

---

## 7. Database & Store Layer

### 7.1 Migration

```sql
ALTER TABLE tasks ADD COLUMN last_heartbeat_time BIGINT NOT NULL DEFAULT 0;
```

### 7.2 Schema

```go
// server/internal/repository/schema/task.go
type Task struct {
    // ... existing fields ...
    LastHeartbeatTime int64 `gorm:"column:last_heartbeat_time"`
}
```

### 7.3 Store Interface — New Methods

```go
// store/task.go
Task interface {
    // ... existing: Create, Get, Claim, ClaimUpdate, Update, UpdateOutcome ...

    // Batch update heartbeat time for running tasks owned by a worker
    UpdateHeartbeat(ctx context.Context, ids []uuid.UUID, workerId int64, heartTime int64) error

    // Query cancelled tasks for a given worker
    GetCancelledTasks(ctx context.Context, workerId int64) ([]uuid.UUID, error)

    // Query retriable failed tasks (FOR UPDATE SKIP LOCKED)
    GetRetriableTasks(ctx context.Context, batchSize int) ([]*schema.Task, error)

    // Query stale running tasks (FOR UPDATE SKIP LOCKED)
    GetStaleTasks(ctx context.Context, timeout int64, batchSize int) ([]*schema.Task, error)

    // Batch update task fields
    BatchUpdate(ctx context.Context, tasks []*schema.Task) error
}
```

### 7.4 Implementation Notes

- `UpdateHeartbeat`: `UPDATE tasks SET last_heartbeat_time = ? WHERE id IN (?) AND worker_id = ? AND state = 'RUNNING'`
- `GetCancelledTasks`: `SELECT id FROM tasks WHERE worker_id = ? AND state = 'CANCELLED'`
- `GetRetriableTasks`: uses `FOR UPDATE SKIP LOCKED` for concurrent safety across instances
- `GetStaleTasks`: uses `FOR UPDATE SKIP LOCKED` for concurrent safety across instances
- New task_events types: `RETRIED`, `STALE_DETECTED`

---

## 8. Error Handling & Robustness

### 8.1 Concurrency Safety

- Task claiming: `FOR UPDATE SKIP LOCKED` + CAS state transitions (existing pattern)
- Background processes: `FOR UPDATE SKIP LOCKED` prevents duplicate processing across instances
- Worker-side: semaphore controls goroutine count, mutex protects shared state

### 8.2 Graceful Degradation

- Report on non-RUNNING task: silently succeed (not an error)
- Heartbeat with stale task IDs: gracefully handle missing tasks
- Background process panic: per-goroutine recovery, one crash doesn't take down others
- Worker disconnect: Unregister on shutdown returns tasks to pool

### 8.3 Edge Cases

- Task cancelled while running: heartbeat returns cancelled IDs, worker aborts
- Worker crashes mid-task: StaleDetector resets to FAILED -> RetryMender handles
- Duplicate Cancel: idempotent (already CANCELLED returns success)
- Max retry exhausted: task stays FAILED permanently, logged via TaskEvent
- Worker re-registers: new worker_id, old tasks (still linked to old id) go stale and get recovered
