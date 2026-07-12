# Admin API Design

> Date: 2026-07-12
> Status: draft

## Overview

Extend the AdminService to provide full operational management capabilities for
namespaces, tasks, workers, and task events. The admin API serves a
operations-facing dashboard/CLI, accessed via a separate Unix socket + HTTP
gRPC-gateway (existing isolation preserved).

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Deployment | Extend existing AdminService on Unix socket | Preserves security isolation from worker-facing APIs |
| Pagination | Offset-based (page/page_size) | UI-friendly for dashboard pagination |
| Namespace update | Name immutable, only description/creator mutable | Name is the unique identifier |
| Namespace delete | Not supported | Prevent cascading delete complexity |
| Task delete | Supported (hard delete) | Operator-driven data cleanup |
| Worker stats | Reuse per-worker fields in Worker schema | No separate aggregation endpoint |
| Filtering | Optional filters on namespace, task_type, state | Keep queries simple for admin dashboard |

---

## 1. Proto Layer

### 1.1 `api/admin/v1/rpc.proto` — Extend AdminService

New RPCs (9 added, 11 total):

```protobuf
service AdminService {
  // Existing
  rpc CreateNamespace(CreateNamespaceRequest) returns (api.schema.v1.Namespace) {
    option (google.api.http) = {
      post: "/api/admin/v1/namespaces"
      body: "*"
    };
  }
  rpc GetNamespace(GetNamespaceRequest) returns (api.schema.v1.Namespace) {
    option (google.api.http) = {get: "/api/admin/v1/namespaces/{name}"};
  }

  // Namespace (new)
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse) {
    option (google.api.http) = {get: "/api/admin/v1/namespaces"};
  }
  rpc UpdateNamespace(UpdateNamespaceRequest) returns (api.schema.v1.Namespace) {
    option (google.api.http) = {
      put: "/api/admin/v1/namespaces/{name}"
      body: "*"
    };
  }

  // Task (new)
  rpc ListTasks(ListTasksRequest) returns (ListTasksResponse) {
    option (google.api.http) = {get: "/api/admin/v1/tasks"};
  }
  rpc GetTask(GetTaskRequest) returns (api.schema.v1.Task) {
    option (google.api.http) = {get: "/api/admin/v1/tasks/{id}"};
  }
  rpc CancelTask(CancelTaskRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      post: "/api/admin/v1/tasks/{id}/cancel"
      body: "*"
    };
  }
  rpc DeleteTask(DeleteTaskRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/api/admin/v1/tasks/{id}"};
  }
  rpc ListTaskEvents(ListTaskEventsRequest) returns (ListTaskEventsResponse) {
    option (google.api.http) = {get: "/api/admin/v1/tasks/{task_id}/events"};
  }

  // Worker (new)
  rpc ListWorkers(ListWorkersRequest) returns (ListWorkersResponse) {
    option (google.api.http) = {get: "/api/admin/v1/workers"};
  }
  rpc GetWorker(GetWorkerRequest) returns (api.schema.v1.Worker) {
    option (google.api.http) = {get: "/api/admin/v1/workers/{id}"};
  }
}
```

#### 1.1.1 Shared pagination messages

```protobuf
message PageRequest {
  int32 page = 1 [(buf.validate.field).int32.gte = 1];
  int32 page_size = 2 [(buf.validate.field).int32 = {gte: 1, lte: 100}];
}

message PageResponse {
  int32 page = 1;
  int32 page_size = 2;
  int64 total_count = 3;
}
```

#### 1.1.2 Namespace messages

```protobuf
message ListNamespacesRequest {
  PageRequest page = 1;
}

message ListNamespacesResponse {
  PageResponse page = 1;
  repeated api.schema.v1.Namespace namespaces = 2;
}

message UpdateNamespaceRequest {
  string name = 1 [
    (buf.validate.field).required = true,
    (google.api.field_behavior) = IDENTIFIER
  ];
  string description = 2;
  string creator = 3;
}
```

UpdateNamespace: `name` is identity (immutable); only `description` and `creator` are updated.

#### 1.1.3 Task messages

```protobuf
message ListTasksRequest {
  PageRequest page = 1;
  string namespace = 2;
  string task_type = 3;
  api.schema.v1.TaskState state = 4;
}

message ListTasksResponse {
  PageResponse page = 1;
  repeated api.schema.v1.Task tasks = 2;
}

message GetTaskRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}

message CancelTaskRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}

message DeleteTaskRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}

message ListTaskEventsRequest {
  string task_id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
  PageRequest page = 2;
}

message ListTaskEventsResponse {
  PageResponse page = 1;
  repeated api.schema.v1.TaskEvent events = 2;
}
```

- `ListTasks`: optional filters `namespace`, `task_type`, `state`; default sort by `create_time DESC`.
- `CancelTask`: only allowed on INITED/RUNNING tasks (reuse existing Cancel logic from task service).
- `DeleteTask`: hard delete from the tasks table (does not cascade to task_events).

#### 1.1.4 Worker messages

```protobuf
message ListWorkersRequest {
  PageRequest page = 1;
  string namespace = 2;
  string task_type = 3;
}

message ListWorkersResponse {
  PageResponse page = 1;
  repeated api.schema.v1.Worker workers = 2;
}

message GetWorkerRequest {
  int64 id = 1 [(buf.validate.field).int64.gte = 1];
}
```

- `ListWorkers`: optional filters `namespace`, `task_type`; default sort by `create_time DESC`.

### 1.2 `api/schema/v1/taskevent.proto` — New schema

```protobuf
syntax = "proto3";

package api.schema.v1;

option go_package = "github.com/gonotelm-lab/flow/api/schema/v1";

message TaskEvent {
  int64 id = 1;
  string task_id = 2;
  string event_type = 3;
  int64 create_time = 4;
  bytes payload = 5;
}
```

---

## 2. Store Layer

### 2.1 `store/namespace.go` — Add 2 methods

```go
type Namespace interface {
    Create(ctx context.Context, namespace *schema.Namespace) (*schema.Namespace, error)
    Get(ctx context.Context, name string) (*schema.Namespace, error)
    List(ctx context.Context, offset, limit int) ([]*schema.Namespace, int64, error) // NEW
    Update(ctx context.Context, ns *schema.Namespace) error                          // NEW
}
```

- `List`: returns data + total_count, ordered by `create_time DESC`.
- `Update`: updates `description` and `creator` fields by `name` (name itself unchanged).

### 2.2 `store/task.go` — Add 2 methods + 1 params struct

```go
type TaskListParams struct {
    Namespace string
    TaskType  string
    State     string
    Offset    int
    Limit     int
}

type Task interface {
    // ... existing methods ...

    List(ctx context.Context, params *TaskListParams) ([]*schema.Task, int64, error) // NEW
    Delete(ctx context.Context, id uuid.UUID) error                                   // NEW
}
```

- `List`: GORM query with conditional `Where` clauses for non-empty filter fields; `Order("create_time DESC")`; `Offset`/`Limit`.
- `Delete`: `DELETE FROM tasks WHERE id = ?`.

### 2.3 `store/taskworker.go` — Add 1 method + 1 params struct

```go
type WorkerListParams struct {
    Namespace string
    TaskType  string
    Offset    int
    Limit     int
}

type TaskWorker interface {
    // ... existing methods ...

    List(ctx context.Context, params *WorkerListParams) ([]*schema.TaskWorker, int64, error) // NEW
}
```

- `List`: conditional filters + `Order("create_time DESC")` + offset/limit.

### 2.4 `store/taskevent.go` — Extend existing method

```go
type TaskEvent interface {
    Append(ctx context.Context, event *schema.TaskEvent) error
    ListByTaskID(ctx context.Context, taskID uuid.UUID, offset, limit int) ([]*schema.TaskEvent, int64, error) // MODIFIED
}
```

- Existing `limit` param split into `offset, limit`; added `total_count` return.
- Update callers: only `server/internal/repository/impl/postgres/taskevent_test.go` references the old signature.
- Ordered by `create_time ASC`.

---

## 3. Postgres Implementation

All new methods in `server/internal/repository/impl/postgres/` follow existing conventions:

- `util.GetDB(ctx, s.db)` for transaction support.
- `sql.WrapError(err)` for error mapping.
- GORM queries with conditional WHERE chains.
- Count query uses `db.Model(&schema.Xxx{}).Where(...).Count(&total)`.

Files to create/modify:
- `postgres/namespace.go` — add `List`, `Update`
- `postgres/task.go` — add `List`, `Delete`
- `postgres/taskworker.go` — add `List`
- `postgres/taskevent.go` — modify `ListByTaskID` signature

---

## 4. Service Layer

### 4.1 `server/internal/service/admin/service.go`

Add exported handler methods for each new RPC. Each handler does:
1. Nil/empty validation on request fields.
2. Calls private implementation method.
3. Wraps errors with `errors.WithMessage`.

Pattern:
```go
func (s *Service) ListNamespaces(
    ctx context.Context,
    req *adminv1.ListNamespacesRequest,
) (*adminv1.ListNamespacesResponse, error) {
    // validate page/page_size
    // call s.listNamespaces(ctx, ...)
    // return
}
```

### 4.2 `server/internal/service/admin/impl.go`

Add private methods for each handler. These handle:
- Type conversion between proto messages and `reposchema` structs.
- Calling `s.store.Xxx.Yyy(...)`.
- Converting repo errors to domain errors.

Namespace helpers: reuse existing `toApiNamespace(ns)`.
Task helpers: reuse existing `toProtoTask(task)` from `service/task/impl.go` (extract to shared location or duplicate; duplicate is acceptable given the thin conversion logic).
Worker helpers: new `toProtoWorker(w *reposchema.TaskWorker) *schemav1.Worker`.
TaskEvent helpers: new `toProtoTaskEvent(e *reposchema.TaskEvent) *schemav1.TaskEvent`.

### 4.3 `server/internal/service/errors/admin.go` — Add errors

```go
const (
    KeyNamespaceNotFound pkgerr.DomainErrorKey = "NAMESPACE_NOT_FOUND"  // existing
    KeyNamespaceExists   pkgerr.DomainErrorKey = "NAMESPACE_ALREADY_EXISTS" // existing
    KeyTaskNotFound      pkgerr.DomainErrorKey = "TASK_NOT_FOUND"       // NEW
    KeyWorkerNotFound    pkgerr.DomainErrorKey = "WORKER_NOT_FOUND"     // NEW
)

var (
    NamespaceNotFound = pkgerr.New(codes.NotFound, KeyNamespaceNotFound)
    NamespaceExists   = pkgerr.New(codes.AlreadyExists, KeyNamespaceExists)
    TaskNotFound      = pkgerr.New(codes.NotFound, KeyTaskNotFound)    // NEW
    WorkerNotFound    = pkgerr.New(codes.NotFound, KeyWorkerNotFound)  // NEW
)
```

---

## 5. Unchanged Components

- `server/internal/endpoint/adminserver.go` — no changes (auto-registers new RPCs via generated gRPC code).
- `server/internal/endpoint/apiserver.go` — no changes.
- `server/internal/service/task/` — no changes.
- `server/internal/service/worker/` — no changes.

---

## 6. Testing

### 6.1 Store tests
- `postgres/namespace_test.go` — add `TestListNamespaces`, `TestUpdateNamespace`.
- `postgres/task_test.go` — add `TestListTasks`, `TestDeleteTask`.
- `postgres/taskworker_test.go` — add `TestListWorkers`.
- `postgres/taskevent_test.go` — update `TestListByTaskID` for new signature.

### 6.2 Service tests
- `admin/service_test.go` — integration tests against a test DB for each new RPC.

Test database setup follows existing `main_test.go` pattern (`repository/init.go`'s `MustInit` for test DB).

---

## 7. HTTP API Summary

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/v1/namespaces` | List namespaces |
| POST | `/api/admin/v1/namespaces` | Create namespace (existing) |
| GET | `/api/admin/v1/namespaces/{name}` | Get namespace (existing) |
| PUT | `/api/admin/v1/namespaces/{name}` | Update namespace |
| GET | `/api/admin/v1/tasks` | List tasks |
| GET | `/api/admin/v1/tasks/{id}` | Get task |
| POST | `/api/admin/v1/tasks/{id}/cancel` | Cancel task |
| DELETE | `/api/admin/v1/tasks/{id}` | Delete task |
| GET | `/api/admin/v1/tasks/{id}/events` | List task events |
| GET | `/api/admin/v1/workers` | List workers |
| GET | `/api/admin/v1/workers/{id}` | Get worker |
