package worker

import (
	"context"
	"testing"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/server/internal/auth"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 假实现用「嵌入接口 + 只覆盖用到的方法」：没覆盖的方法一被调用就 nil panic。

type fakeNamespaceStore struct {
	store.Namespace

	get func(ctx context.Context, name string) (*reposchema.Namespace, error)
}

func (f *fakeNamespaceStore) Get(ctx context.Context, name string) (*reposchema.Namespace, error) {
	if f.get == nil {
		panic("fakeNamespaceStore.Get: not stubbed")
	}
	return f.get(ctx, name)
}

type fakeTaskWorkerStore struct {
	store.TaskWorker

	get             func(ctx context.Context, id int64) (*reposchema.TaskWorker, error)
	delete          func(ctx context.Context, id int64) error
	updateHeartbeat func(ctx context.Context, id int64, heartbeatTime int64) (bool, error)
}

func (f *fakeTaskWorkerStore) Get(ctx context.Context, id int64) (*reposchema.TaskWorker, error) {
	if f.get == nil {
		panic("fakeTaskWorkerStore.Get: not stubbed")
	}
	return f.get(ctx, id)
}

func (f *fakeTaskWorkerStore) Delete(ctx context.Context, id int64) error {
	if f.delete == nil {
		panic("fakeTaskWorkerStore.Delete: not stubbed")
	}
	return f.delete(ctx, id)
}

func (f *fakeTaskWorkerStore) UpdateHeartbeat(
	ctx context.Context, id int64, heartbeatTime int64,
) (bool, error) {
	if f.updateHeartbeat == nil {
		panic("fakeTaskWorkerStore.UpdateHeartbeat: not stubbed")
	}
	return f.updateHeartbeat(ctx, id, heartbeatTime)
}

type fakeTaskStore struct {
	store.Task

	get           func(ctx context.Context, id uuid.UUID) (*reposchema.Task, error)
	claim         func(ctx context.Context, namespace, taskType string, states []string) (*reposchema.Task, error)
	getCancelled  func(ctx context.Context, workerId int64) ([]uuid.UUID, error)
	updateOutcome func(
		ctx context.Context, id uuid.UUID, success bool, workerId int64,
		oldState, newState string, params *store.TaskUpdateOutcomeParams,
	) (bool, error)
}

func (f *fakeTaskStore) Get(ctx context.Context, id uuid.UUID) (*reposchema.Task, error) {
	if f.get == nil {
		panic("fakeTaskStore.Get: not stubbed")
	}
	return f.get(ctx, id)
}

func (f *fakeTaskStore) Claim(
	ctx context.Context, namespace, taskType string, states []string,
) (*reposchema.Task, error) {
	if f.claim == nil {
		panic("fakeTaskStore.Claim: not stubbed")
	}
	return f.claim(ctx, namespace, taskType, states)
}

func (f *fakeTaskStore) GetCancelledTasks(ctx context.Context, workerId int64) ([]uuid.UUID, error) {
	if f.getCancelled == nil {
		panic("fakeTaskStore.GetCancelledTasks: not stubbed")
	}
	return f.getCancelled(ctx, workerId)
}

func (f *fakeTaskStore) UpdateOutcome(
	ctx context.Context, id uuid.UUID, success bool, workerId int64,
	oldState, newState string, params *store.TaskUpdateOutcomeParams,
) (bool, error) {
	if f.updateOutcome == nil {
		panic("fakeTaskStore.UpdateOutcome: not stubbed")
	}
	return f.updateOutcome(ctx, id, success, workerId, oldState, newState, params)
}

type fakeTaskEventStore struct {
	store.TaskEvent
}

func (f *fakeTaskEventStore) Append(context.Context, *reposchema.TaskEvent) error {
	return nil
}

func newService(repo *repository.Store) workerv1.WorkerServiceServer {
	return NewService(repo, ServiceConfig{})
}

func assertKey(t *testing.T, err error, key pkgerr.DomainErrorKey) {
	t.Helper()

	domainErr, ok := pkgerr.As(err)
	require.True(t, ok, "expected a domain error, got %v", err)
	assert.Equal(t, key, domainErr.Key())
}

func TestRegister_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	var namespaceLookups int
	svc := newService(&repository.Store{
		Namespace: &fakeNamespaceStore{get: func(_ context.Context, name string) (*reposchema.Namespace, error) {
			namespaceLookups++
			return &reposchema.Namespace{Name: name}, nil
		}},
	})

	ctx := auth.WithNamespace(context.Background(), "demo")

	_, err := svc.Register(ctx, &schemav1.Worker{Namespace: "other", TaskType: "echo"})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyPermissionDenied)
	assert.Zero(t, namespaceLookups, "must not look up another namespace")
}

func TestPoll_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	var claims int
	svc := newService(&repository.Store{
		Task: &fakeTaskStore{claim: func(
			_ context.Context, namespace, taskType string, _ []string,
		) (*reposchema.Task, error) {
			claims++
			return nil, pkgerr.NoRecord
		}},
	})

	ctx := auth.WithNamespace(context.Background(), "demo")

	_, err := svc.Poll(ctx, &workerv1.PollRequest{Id: 1, Namespace: "other", TaskType: "echo"})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyPermissionDenied)
	assert.Zero(t, claims, "must not claim a task across namespaces")
}

// Unregister / Heartbeat 只有 id，做归属判断要多读一次行。
func TestUnregisterAndHeartbeat_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	const workerID = 7
	var (
		deletes          int
		heartbeatUpdates int
	)
	workerStore := &fakeTaskWorkerStore{
		get: func(_ context.Context, id int64) (*reposchema.TaskWorker, error) {
			require.Equal(t, int64(workerID), id)
			return &reposchema.TaskWorker{Id: id, Namespace: "demo"}, nil
		},
		delete: func(context.Context, int64) error {
			deletes++
			return nil
		},
		updateHeartbeat: func(context.Context, int64, int64) (bool, error) {
			heartbeatUpdates++
			return true, nil
		},
	}
	svc := newService(&repository.Store{
		TaskWorker: workerStore,
		Task: &fakeTaskStore{
			getCancelled: func(context.Context, int64) ([]uuid.UUID, error) {
				return nil, nil
			},
		},
	})

	other := auth.WithNamespace(context.Background(), "other")

	_, err := svc.Unregister(other, &workerv1.UnregisterRequest{Id: workerID})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyNoRecord)
	assert.Zero(t, deletes, "must not delete another namespace's worker")

	_, err = svc.Heartbeat(other, &workerv1.HeartbeatRequest{Id: workerID})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyNoRecord)
	assert.Zero(t, heartbeatUpdates, "must not heartbeat another namespace's worker")

	// 自己的 namespace：两个方法都正常。
	own := auth.WithNamespace(context.Background(), "demo")

	_, err = svc.Unregister(own, &workerv1.UnregisterRequest{Id: workerID})
	require.NoError(t, err)
	assert.Equal(t, 1, deletes)

	_, err = svc.Heartbeat(own, &workerv1.HeartbeatRequest{Id: workerID})
	require.NoError(t, err)
	assert.Equal(t, 1, heartbeatUpdates)
}

// 行不存在时，保持加鉴权之前的静默成功。
func TestUnregister_MissingWorkerKeepsLegacyBehaviour(t *testing.T) {
	t.Parallel()

	var deletes int
	svc := newService(&repository.Store{
		TaskWorker: &fakeTaskWorkerStore{
			get: func(context.Context, int64) (*reposchema.TaskWorker, error) {
				return nil, pkgerr.NoRecord
			},
			delete: func(context.Context, int64) error {
				deletes++
				return nil
			},
		},
	})

	ctx := auth.WithNamespace(context.Background(), "demo")

	_, err := svc.Unregister(ctx, &workerv1.UnregisterRequest{Id: 404})
	require.NoError(t, err)
	assert.Equal(t, 1, deletes)
}

func TestReport_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	taskID := uuid.New()
	running := &reposchema.Task{
		Id:        taskID,
		Namespace: "demo",
		State:     schemav1.TaskState_RUNNING.String(),
	}

	var outcomes int
	svc := newService(&repository.Store{
		Task: &fakeTaskStore{
			get: func(_ context.Context, id uuid.UUID) (*reposchema.Task, error) {
				if id != taskID {
					return nil, pkgerr.NoRecord
				}
				return running, nil
			},
			updateOutcome: func(
				context.Context, uuid.UUID, bool, int64, string, string,
				*store.TaskUpdateOutcomeParams,
			) (bool, error) {
				outcomes++
				return true, nil
			},
		},
		TaskEvent: &fakeTaskEventStore{},
	})

	req := &workerv1.ReportRequest{
		WorkerId: 1,
		TaskId:   taskID.String(),
		Action:   workerv1.ReportAction_SUCCESS,
	}

	other := auth.WithNamespace(context.Background(), "other")
	_, err := svc.Report(other, req)
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyNoRecord)
	assert.Zero(t, outcomes, "must not report another namespace's task result")

	own := auth.WithNamespace(context.Background(), "demo")
	_, err = svc.Report(own, req)
	require.NoError(t, err)
	assert.Equal(t, 1, outcomes)
}
