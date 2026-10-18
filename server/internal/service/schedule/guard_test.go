package schedule

import (
	"context"
	"testing"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
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

type fakeScheduleStore struct {
	store.Schedule

	get  func(ctx context.Context, id uuid.UUID) (*reposchema.Schedule, error)
	list func(ctx context.Context, params *store.ScheduleListParams) ([]*reposchema.Schedule, int64, error)
}

func (f *fakeScheduleStore) Get(ctx context.Context, id uuid.UUID) (*reposchema.Schedule, error) {
	if f.get == nil {
		panic("fakeScheduleStore.Get: not stubbed")
	}
	return f.get(ctx, id)
}

func (f *fakeScheduleStore) List(
	ctx context.Context, params *store.ScheduleListParams,
) ([]*reposchema.Schedule, int64, error) {
	if f.list == nil {
		panic("fakeScheduleStore.List: not stubbed")
	}
	return f.list(ctx, params)
}

func assertKey(t *testing.T, err error, key pkgerr.DomainErrorKey) {
	t.Helper()

	domainErr, ok := pkgerr.As(err)
	require.True(t, ok, "expected a domain error, got %v", err)
	assert.Equal(t, key, domainErr.Key())
}

// 归属判断必须在「查 namespace 是否存在」之前。
func TestRegister_CrossNamespaceIsRejected(t *testing.T) {
	t.Parallel()

	var namespaceLookups int
	svc := NewService(&repository.Store{
		Namespace: &fakeNamespaceStore{get: func(_ context.Context, name string) (*reposchema.Namespace, error) {
			namespaceLookups++
			return &reposchema.Namespace{Name: name}, nil
		}},
	})

	ctx := auth.WithNamespace(context.Background(), "demo")

	_, err := svc.Register(ctx, &schedulev1.RegisterRequest{
		Schedule: &schemav1.Schedule{
			Namespace:   "other",
			CallbackUrl: "https://example.com/hook",
			Trigger: &schemav1.Schedule_Cron{
				Cron: &schemav1.CronTrigger{Expr: "*/5 * * * *"},
			},
		},
	})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyPermissionDenied)
	assert.Zero(t, namespaceLookups, "must not look up another namespace")
}

// List 的 namespace 是可选筛选，为空时会跨 namespace 返回全量，鉴权打开时必须改写。
func TestList_IsScopedToCallerNamespace(t *testing.T) {
	t.Parallel()

	var gotNamespaces []string
	sched := &fakeScheduleStore{list: func(
		_ context.Context, params *store.ScheduleListParams,
	) ([]*reposchema.Schedule, int64, error) {
		gotNamespaces = append(gotNamespaces, params.Namespace)
		return nil, 0, nil
	}}
	svc := NewService(&repository.Store{Schedule: sched})

	// 鉴权关闭：请求里写什么就是什么（包括空 = 全量），和加鉴权之前一致。
	_, err := svc.List(context.Background(), &schedulev1.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{""}, gotNamespaces)

	// 已认证：空筛选被改写成调用方自己的 namespace。
	own := auth.WithNamespace(context.Background(), "demo")
	_, err = svc.List(own, &schedulev1.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"", "demo"}, gotNamespaces)

	// 已认证且显式写了自己的 namespace：照常放行。
	_, err = svc.List(own, &schedulev1.ListRequest{Namespace: "demo"})
	require.NoError(t, err)
	assert.Equal(t, []string{"", "demo", "demo"}, gotNamespaces)

	// 已认证但显式写了别人的 namespace：拒绝，且不落库查询。
	_, err = svc.List(own, &schedulev1.ListRequest{Namespace: "other"})
	require.Error(t, err)
	assertKey(t, err, pkgerr.KeyPermissionDenied)
	assert.Equal(t, []string{"", "demo", "demo"}, gotNamespaces)
}

// Get / Stop / Delete 都走 core.mustGet：跨 namespace 一律当成不存在。
func TestGetStopDelete_CrossNamespaceLooksNotFound(t *testing.T) {
	t.Parallel()

	scheduleID := uuid.New()
	sched := &fakeScheduleStore{get: func(_ context.Context, id uuid.UUID) (*reposchema.Schedule, error) {
		if id != scheduleID {
			return nil, pkgerr.NoRecord
		}
		return &reposchema.Schedule{Id: id, Namespace: "demo"}, nil
	}}
	svc := NewService(&repository.Store{Schedule: sched})

	other := auth.WithNamespace(context.Background(), "other")

	_, err := svc.Get(other, &schedulev1.GetRequest{Id: scheduleID.String()})
	require.Error(t, err)
	assertKey(t, err, srverr.ScheduleNotFound.Key())

	_, err = svc.Stop(other, &schedulev1.StopRequest{Id: scheduleID.String()})
	require.Error(t, err)
	assertKey(t, err, srverr.ScheduleNotFound.Key())

	_, err = svc.Delete(other, &schedulev1.DeleteRequest{Id: scheduleID.String()})
	require.Error(t, err)
	assertKey(t, err, srverr.ScheduleNotFound.Key())

	// 自己的 namespace：读得到。
	own := auth.WithNamespace(context.Background(), "demo")
	resp, err := svc.Get(own, &schedulev1.GetRequest{Id: scheduleID.String()})
	require.NoError(t, err)
	assert.Equal(t, scheduleID.String(), resp.GetId())
}
