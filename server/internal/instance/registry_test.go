package instance

import (
	"context"
	stderr "errors"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRegistryConfig() RegistryConfig {
	return RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: 10 * time.Millisecond,
	}
}

func TestZeroRevision(t *testing.T) {
	rev := zeroRevision()
	require.NotNil(t, rev)
	assert.Equal(t, discovRevisionName, rev.Name)
	assert.Equal(t, int64(0), rev.CurrentRevision)
	assert.Greater(t, rev.UpdateTime, int64(0))
}

func TestRegistry_Register_WhenClosing(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{}, testRegistryConfig())
	r.closing.Store(true)

	_, err := r.Register(context.Background())
	require.Error(t, err)
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{}, testRegistryConfig())

	err := r.Unregister(context.Background(), 999)
	require.NoError(t, err)
}

func TestRegistry_GetAll(t *testing.T) {
	var capturedAliveAfter int64
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: &fakeInstanceStore{
			listActiveFn: func(
				_ context.Context,
				aliveAfterMs int64,
			) ([]*schema.Instance, error) {
				capturedAliveAfter = aliveAfterMs
				return []*schema.Instance{
					{
						Id:             22,
						Group:          InstanceGroup,
						Key:            "flow/instances/b",
						Value:          "v2",
						StartTime:      4000,
						ExpireTime:     2000,
						FencingToken:   3000,
						CreateRevision: 2,
					},
					nil,
					{
						Id:             11,
						Group:          InstanceGroup,
						Key:            "flow/instances/a",
						Value:          "v1",
						StartTime:      1000,
						ExpireTime:     5000,
						FencingToken:   6000,
						CreateRevision: 1,
					},
					{
						Id:             33,
						Group:          InstanceGroup,
						Key:            "flow/instances/c",
						Value:          "v3",
						StartTime:      4000,
						ExpireTime:     6000,
						FencingToken:   7000,
						CreateRevision: 3,
					},
				}, nil
			},
		},
	}, testRegistryConfig())

	begin := time.Now().UnixMilli()
	got, err := r.GetAll(context.Background())
	end := time.Now().UnixMilli()

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.GreaterOrEqual(t, capturedAliveAfter, begin)
	assert.LessOrEqual(t, capturedAliveAfter, end)

	assert.Equal(t, int64(11), got[0].Id)
	assert.Equal(t, "flow/instances/a", got[0].Key)
	assert.Equal(t, int64(1), got[0].CreateRevision)

	assert.Equal(t, int64(22), got[1].Id)
	assert.Equal(t, "flow/instances/b", got[1].Key)
	assert.Equal(t, int64(2), got[1].CreateRevision)

	assert.Equal(t, int64(33), got[2].Id)
	assert.Equal(t, "flow/instances/c", got[2].Key)
	assert.Equal(t, int64(3), got[2].CreateRevision)

	assert.Equal(t, int64(1000), got[0].StartTime)
	assert.Equal(t, int64(4000), got[1].StartTime)
	assert.Equal(t, int64(4000), got[2].StartTime)
}

func TestRegistry_GetAll_ListActiveFailed(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: &fakeInstanceStore{
			listActiveFn: func(
				_ context.Context,
				_ int64,
			) ([]*schema.Instance, error) {
				return nil, stderr.New("db list active failed")
			},
		},
	}, testRegistryConfig())

	_, err := r.GetAll(context.Background())
	require.Error(t, err)
}

func TestRegistry_Register_TransactionFailed(t *testing.T) {
	var called bool
	r := NewRegistry(repository.TxManager{}, repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getOrInitForUpdateFn: func(
				_ context.Context,
				_ *schema.GlobalRevision,
			) (*schema.GlobalRevision, error) {
				called = true
				return nil, stderr.New("global revision unavailable")
			},
		},
	}, testRegistryConfig())

	_, err := r.Register(testTxContext())
	require.Error(t, err)
	assert.True(t, called)
}

func TestRegistry_Unregister_TransactionFailed(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getOrInitForUpdateFn: func(
				_ context.Context,
				_ *schema.GlobalRevision,
			) (*schema.GlobalRevision, error) {
				return nil, stderr.New("global revision unavailable")
			},
		},
	}, testRegistryConfig())

	r.locals[100] = &cancellableInstance{
		Instance: &Instance{
			Id:    100,
			Group: InstanceGroup,
			Key:   "flow/instances/x",
			Value: "v",
		},
		cancel: func() {},
	}

	err := r.Unregister(testTxContext(), 100)
	require.Error(t, err)

	r.mu.RLock()
	_, ok := r.locals[100]
	r.mu.RUnlock()
	assert.True(t, ok)
}

func TestRegistry_Heartbeat_Success(t *testing.T) {
	var (
		calls          int
		gotID          int64
		gotExpireTime  int64
		gotExpectToken int64
	)
	store := &fakeInstanceStore{
		updateExpireTimeFn: func(
			_ context.Context,
			id int64,
			expireTimeMs int64,
			expectToken int64,
		) (bool, error) {
			calls++
			gotID = id
			gotExpireTime = expireTimeMs
			gotExpectToken = expectToken
			return true, nil
		},
	}
	cfg := testRegistryConfig()
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: store,
	}, cfg)

	ins := &Instance{
		Id:           123,
		ExpireTime:   10_000,
		FencingToken: 7_777,
	}
	expectedExpire := time.UnixMilli(10_000).Add(cfg.Expiry).UnixMilli()

	r.heartbeat(context.Background(), ins)

	assert.Equal(t, 1, calls)
	assert.Equal(t, int64(123), gotID)
	assert.Equal(t, expectedExpire, gotExpireTime)
	assert.Equal(t, int64(7_777), gotExpectToken)
	assert.Equal(t, expectedExpire, ins.ExpireTime)
}

func TestRegistry_Heartbeat_ErrorRollback(t *testing.T) {
	var calls int
	store := &fakeInstanceStore{
		updateExpireTimeFn: func(
			_ context.Context,
			_ int64,
			_ int64,
			_ int64,
		) (bool, error) {
			calls++
			return false, stderr.New("db failed")
		},
	}
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: store,
	}, testRegistryConfig())

	ins := &Instance{
		Id:           1,
		ExpireTime:   20_000,
		FencingToken: 2,
	}
	r.heartbeat(context.Background(), ins)

	assert.Equal(t, 3, calls)
	assert.Equal(t, int64(20_000), ins.ExpireTime)
}

func TestRegistry_Heartbeat_FencingMismatch(t *testing.T) {
	var calls int
	store := &fakeInstanceStore{
		updateExpireTimeFn: func(
			_ context.Context,
			_ int64,
			_ int64,
			_ int64,
		) (bool, error) {
			calls++
			return false, nil
		},
	}
	cfg := testRegistryConfig()
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: store,
	}, cfg)

	ins := &Instance{
		Id:           1,
		ExpireTime:   30_000,
		FencingToken: 2,
	}
	expectedExpire := time.UnixMilli(30_000).Add(cfg.Expiry).UnixMilli()

	r.heartbeat(context.Background(), ins)

	assert.Equal(t, 1, calls)
	assert.Equal(t, expectedExpire, ins.ExpireTime)
}

func TestRegistry_Heartbeat_AutoReRegister(t *testing.T) {
	var (
		updateCalls int
		createCalls int
		appendCalls int
		getRevCalls int
		incrCalls   int
		nextID      int64 = 1000
		curRev      int64 = 10
	)

	store := &fakeInstanceStore{
		updateExpireTimeFn: func(
			_ context.Context,
			_ int64,
			_ int64,
			_ int64,
		) (bool, error) {
			updateCalls++
			return false, nil
		},
		createFn: func(_ context.Context, ins *schema.Instance) (*schema.Instance, error) {
			createCalls++
			copied := *ins
			copied.Id = nextID
			nextID++
			return &copied, nil
		},
	}
	events := &fakeInstanceEventStore{
		appendFn: func(_ context.Context, _ *schema.InstanceEvent) error {
			appendCalls++
			return nil
		},
	}
	revs := &fakeGlobalRevisionStore{
		getOrInitForUpdateFn: func(
			_ context.Context,
			zero *schema.GlobalRevision,
		) (*schema.GlobalRevision, error) {
			getRevCalls++
			return &schema.GlobalRevision{
				Name:            zero.Name,
				CurrentRevision: curRev,
				UpdateTime:      zero.UpdateTime,
			}, nil
		},
		incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
			incrCalls++
			curRev++
			return nil
		},
	}

	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance:       store,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	defer r.Close()

	parentCtx := testTxContext()
	oldCtx, oldCancel := context.WithCancel(parentCtx)
	old := &cancellableInstance{
		Instance: &Instance{
			Id:           1,
			Group:        InstanceGroup,
			Key:          "flow/instances/old",
			Value:        "v",
			ExpireTime:   12345,
			FencingToken: 99,
		},
		cancel:    oldCancel,
		parentCtx: parentCtx,
	}
	r.locals[old.Id] = old

	r.heartbeat(oldCtx, old.Instance)

	r.mu.RLock()
	_, oldExists := r.locals[old.Id]
	var replacement *cancellableInstance
	for id, inst := range r.locals {
		if id != old.Id {
			replacement = inst
		}
	}
	r.mu.RUnlock()

	assert.False(t, oldExists)
	require.NotNil(t, replacement)
	assert.NotEqual(t, int64(1), replacement.Id)
	assert.Equal(t, 1, updateCalls)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, appendCalls)
	assert.Equal(t, 1, getRevCalls)
	assert.Equal(t, 1, incrCalls)
}

func TestRegistry_Heartbeat_AutoReRegister_UsesParentCtx(t *testing.T) {
	var (
		createCalls int
		nextID      int64 = 2000
		curRev      int64 = 20
	)

	instanceStore := &fakeInstanceStore{
		updateExpireTimeFn: func(_ context.Context, _ int64, _ int64, _ int64) (bool, error) {
			return false, nil
		},
		createFn: func(ctx context.Context, ins *schema.Instance) (*schema.Instance, error) {
			createCalls++
			if ctx.Err() != nil {
				return nil, stderr.New("register ctx should not be canceled")
			}

			copied := *ins
			copied.Id = nextID
			nextID++
			return &copied, nil
		},
	}
	revs := &fakeGlobalRevisionStore{
		getOrInitForUpdateFn: func(_ context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error) {
			return &schema.GlobalRevision{
				Name:            zero.Name,
				CurrentRevision: curRev,
				UpdateTime:      zero.UpdateTime,
			}, nil
		},
		incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
			curRev++
			return nil
		},
	}
	events := &fakeInstanceEventStore{
		appendFn: func(_ context.Context, _ *schema.InstanceEvent) error {
			return nil
		},
	}

	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	defer r.Close()

	parentCtx := testTxContext()
	oldCtx, oldCancel := context.WithCancel(parentCtx)
	old := &cancellableInstance{
		Instance: &Instance{
			Id:           1,
			Group:        InstanceGroup,
			Key:          "flow/instances/old-parent",
			Value:        "v",
			ExpireTime:   12345,
			FencingToken: 99,
		},
		cancel:    oldCancel,
		parentCtx: parentCtx,
	}
	r.locals[old.Id] = old

	// 模拟 heartbeat 触发点拿到的是已取消 ctx；重注册应该仍使用 old.parentCtx。
	oldCancel()
	r.heartbeat(oldCtx, old.Instance)

	r.mu.RLock()
	_, oldExists := r.locals[old.Id]
	r.mu.RUnlock()

	assert.Equal(t, 1, createCalls)
	assert.False(t, oldExists)
}

func TestRegistry_Heartbeat_AutoReRegister_RegisterFailed_KeepOld(t *testing.T) {
	var (
		createCalls   int
		appendCalls   int
		incrCalls     int
		oldCancelCall int
		curRev        int64 = 30
	)

	instanceStore := &fakeInstanceStore{
		updateExpireTimeFn: func(_ context.Context, _ int64, _ int64, _ int64) (bool, error) {
			return false, nil
		},
		createFn: func(_ context.Context, _ *schema.Instance) (*schema.Instance, error) {
			createCalls++
			return nil, stderr.New("create replacement failed")
		},
	}
	revs := &fakeGlobalRevisionStore{
		getOrInitForUpdateFn: func(_ context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error) {
			return &schema.GlobalRevision{
				Name:            zero.Name,
				CurrentRevision: curRev,
				UpdateTime:      zero.UpdateTime,
			}, nil
		},
		incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
			incrCalls++
			curRev++
			return nil
		},
	}
	events := &fakeInstanceEventStore{
		appendFn: func(_ context.Context, _ *schema.InstanceEvent) error {
			appendCalls++
			return nil
		},
	}

	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	defer r.Close()

	parentCtx := testTxContext()
	oldCtx, baseCancel := context.WithCancel(parentCtx)
	old := &cancellableInstance{
		Instance: &Instance{
			Id:           2,
			Group:        InstanceGroup,
			Key:          "flow/instances/old-keep",
			Value:        "v",
			ExpireTime:   12345,
			FencingToken: 100,
		},
		cancel: func() {
			oldCancelCall++
			baseCancel()
		},
		parentCtx: parentCtx,
	}
	r.locals[old.Id] = old

	r.heartbeat(oldCtx, old.Instance)

	r.mu.RLock()
	got, exists := r.locals[old.Id]
	r.mu.RUnlock()

	assert.Equal(t, 1, createCalls)
	assert.True(t, exists)
	assert.Equal(t, old, got)
	assert.Equal(t, 0, oldCancelCall)
	assert.Equal(t, 0, appendCalls)
	assert.Equal(t, 0, incrCalls)
}

func TestRegistry_TryAutoReRegister_Closing_NoOp(t *testing.T) {
	var createCalls int

	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: &fakeInstanceStore{
			createFn: func(_ context.Context, ins *schema.Instance) (*schema.Instance, error) {
				createCalls++
				return ins, nil
			},
		},
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	defer r.Close()

	parentCtx := testTxContext()
	old := &cancellableInstance{
		Instance: &Instance{
			Id:           3,
			Group:        InstanceGroup,
			Key:          "flow/instances/old-closing",
			Value:        "v",
			ExpireTime:   12345,
			FencingToken: 101,
		},
		cancel:    func() {},
		parentCtx: parentCtx,
	}
	r.locals[old.Id] = old
	r.closing.Store(true)

	err := r.tryAutoReRegister(context.Background(), old.Instance)
	require.NoError(t, err)
	assert.Equal(t, 0, createCalls)

	r.mu.RLock()
	_, exists := r.locals[old.Id]
	r.mu.RUnlock()
	assert.True(t, exists)
}
