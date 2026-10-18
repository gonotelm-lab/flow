package task

import (
	"context"
	"testing"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/gonotelm-lab/flow/server/internal/auth"
	"github.com/gonotelm-lab/flow/server/internal/repository"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/errors"
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

type fakeTaskStore struct {
	store.Task

	get    func(ctx context.Context, id uuid.UUID) (*reposchema.Task, error)
	create func(ctx context.Context, task *reposchema.Task) (*reposchema.Task, error)
	update func(ctx context.Context, task *reposchema.Task) (bool, error)
}

func (f *fakeTaskStore) Get(ctx context.Context, id uuid.UUID) (*reposchema.Task, error) {
	if f.get == nil {
		panic("fakeTaskStore.Get: not stubbed")
	}
	return f.get(ctx, id)
}

func (f *fakeTaskStore) Create(ctx context.Context, task *reposchema.Task) (*reposchema.Task, error) {
	if f.create == nil {
		panic("fakeTaskStore.Create: not stubbed")
	}
	return f.create(ctx, task)
}

func (f *fakeTaskStore) Update(ctx context.Context, task *reposchema.Task) (bool, error) {
	if f.update == nil {
		panic("fakeTaskStore.Update: not stubbed")
	}
	return f.update(ctx, task)
}

type fakeTaskEventStore struct {
	store.TaskEvent
}

func (f *fakeTaskEventStore) Append(context.Context, *reposchema.TaskEvent) error {
	return nil
}

func newService(ns store.Namespace, tasks store.Task) taskv1.TaskServiceServer {
	return NewService(&repository.Store{
		Namespace: ns,
		Task:      tasks,
		TaskEvent: &fakeTaskEventStore{},
	})
}

func assertKey(t *testing.T, err error, key pkgerr.DomainErrorKey) {
	t.Helper()

	domainErr, ok := pkgerr.As(err)
	require.True(t, ok, "expected a domain error, got %v", err)
	assert.Equal(t, key, domainErr.Key())
}

// 归属判断必须在「查 namespace 是否存在」之前，否则错误码会泄露别人的 namespace 是否存在。
func TestSubmit_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	var (
		namespaceLookups int
		created          int
	)

	svc := newService(
		&fakeNamespaceStore{get: func(_ context.Context, name string) (*reposchema.Namespace, error) {
			namespaceLookups++
			return &reposchema.Namespace{Name: name}, nil
		}},
		&fakeTaskStore{create: func(_ context.Context, task *reposchema.Task) (*reposchema.Task, error) {
			created++
			return task, nil
		}},
	)

	ctx := auth.WithNamespace(context.Background(), "demo")

	// 自己的 namespace：照常提交。
	_, err := svc.Submit(ctx, &taskv1.SubmitRequest{Namespace: "demo", TaskType: "echo"})
	require.NoError(t, err)
	assert.Equal(t, 1, created)
	assert.Equal(t, 1, namespaceLookups)

	// 别人的 namespace：PermissionDenied，且不查 namespace、不落库。
	_, err = svc.Submit(ctx, &taskv1.SubmitRequest{Namespace: "other", TaskType: "echo"})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyPermissionDenied)
	assert.Equal(t, 1, created, "must not create a task across namespaces")
	assert.Equal(t, 1, namespaceLookups, "must not look up another namespace")
}

// 鉴权关闭时必须和加鉴权之前完全一致。
func TestSubmit_WithoutAuthIsUnchanged(t *testing.T) {
	t.Parallel()

	var created int
	svc := newService(
		&fakeNamespaceStore{get: func(_ context.Context, name string) (*reposchema.Namespace, error) {
			return &reposchema.Namespace{Name: name}, nil
		}},
		&fakeTaskStore{create: func(_ context.Context, task *reposchema.Task) (*reposchema.Task, error) {
			created++
			return task, nil
		}},
	)

	_, err := svc.Submit(context.Background(), &taskv1.SubmitRequest{Namespace: "anything", TaskType: "echo"})
	require.NoError(t, err)
	assert.Equal(t, 1, created)
}

func TestGetAndCancel_CrossNamespaceLooksNotFound(t *testing.T) {
	t.Parallel()

	taskID := uuid.New()
	row := &reposchema.Task{
		Id:        taskID,
		Namespace: "demo",
		State:     schemav1.TaskState_INITED.String(),
	}

	var updates int
	svc := newService(
		&fakeNamespaceStore{},
		&fakeTaskStore{
			get: func(_ context.Context, id uuid.UUID) (*reposchema.Task, error) {
				if id != taskID {
					return nil, pkgerr.NoRecord
				}
				return row, nil
			},
			update: func(_ context.Context, task *reposchema.Task) (bool, error) {
				updates++
				return true, nil
			},
		},
	)

	other := auth.WithNamespace(context.Background(), "other")

	_, err := svc.Get(other, &taskv1.GetRequest{Id: taskID.String()})
	require.Error(t, err)
	assertKey(t, err, srverr.TaskNotFound.Key())

	_, err = svc.Cancel(other, &taskv1.CancelRequest{Id: taskID.String()})
	require.Error(t, err)
	assertKey(t, err, srverr.TaskNotFound.Key())
	assert.Zero(t, updates, "must not modify a task across namespaces")

	// 自己的 namespace：两个方法都正常。
	own := auth.WithNamespace(context.Background(), "demo")

	resp, err := svc.Get(own, &taskv1.GetRequest{Id: taskID.String()})
	require.NoError(t, err)
	assert.Equal(t, taskID.String(), resp.GetTask().GetId())

	_, err = svc.Cancel(own, &taskv1.CancelRequest{Id: taskID.String()})
	require.NoError(t, err)
	assert.Equal(t, 1, updates)
}
