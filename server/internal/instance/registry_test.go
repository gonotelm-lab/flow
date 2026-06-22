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
	r := NewRegistry(&repository.TxManager{}, &repository.Store{}, testRegistryConfig())
	r.closing.Store(true)

	_, err := r.Register(context.Background(), testInstanceGroup, "v")
	require.Error(t, err)
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	r := NewRegistry(&repository.TxManager{}, &repository.Store{}, testRegistryConfig())

	err := r.Unregister(context.Background(), 999)
	require.NoError(t, err)
}

func TestRegistry_GetAll(t *testing.T) {
	var capturedAliveAfter int64
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance: &fakeInstanceStore{
			listActiveFn: func(
				_ context.Context,
				aliveAfterMs int64,
			) ([]*schema.Instance, error) {
				capturedAliveAfter = aliveAfterMs
				return []*schema.Instance{
					{
						Id:             22,
						Group:          testInstanceGroup,
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
						Group:          testInstanceGroup,
						Key:            "flow/instances/a",
						Value:          "v1",
						StartTime:      1000,
						ExpireTime:     5000,
						FencingToken:   6000,
						CreateRevision: 1,
					},
					{
						Id:             33,
						Group:          testInstanceGroup,
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
	got, err := r.GetAllPeers(context.Background())
	end := time.Now().UnixMilli()

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.GreaterOrEqual(t, capturedAliveAfter, begin)
	assert.LessOrEqual(t, capturedAliveAfter, end)

	assert.Equal(t, int64(11), got[0].id)
	assert.Equal(t, "flow/instances/a", got[0].key)
	assert.Equal(t, int64(1), got[0].createRevision)

	assert.Equal(t, int64(22), got[1].id)
	assert.Equal(t, "flow/instances/b", got[1].key)
	assert.Equal(t, int64(2), got[1].createRevision)

	assert.Equal(t, int64(33), got[2].id)
	assert.Equal(t, "flow/instances/c", got[2].key)
	assert.Equal(t, int64(3), got[2].createRevision)

	assert.Equal(t, int64(1000), got[0].startTime)
	assert.Equal(t, int64(4000), got[1].startTime)
	assert.Equal(t, int64(4000), got[2].startTime)
}

func TestRegistry_GetAll_ListActiveFailed(t *testing.T) {
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance: &fakeInstanceStore{
			listActiveFn: func(
				_ context.Context,
				_ int64,
			) ([]*schema.Instance, error) {
				return nil, stderr.New("db list active failed")
			},
		},
	}, testRegistryConfig())

	_, err := r.GetAllPeers(context.Background())
	require.Error(t, err)
}

func TestRegistry_Register_TransactionFailed(t *testing.T) {
	var called bool
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
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

	_, err := r.Register(testTxContext(), testInstanceGroup, "v")
	require.Error(t, err)
	assert.True(t, called)
}

func TestRegistry_Unregister_TransactionFailed(t *testing.T) {
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
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
			id:    100,
			group: testInstanceGroup,
			key:   "flow/instances/x",
			value: "v",
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
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance: store,
	}, cfg)

	ins := &Instance{
		id:           123,
		expireTime:   10_000,
		fencingToken: 7_777,
	}
	begin := time.Now().UnixMilli()

	r.heartbeat(context.Background(), ins)
	end := time.Now().UnixMilli()
	minExpire := begin + cfg.Expiry.Milliseconds()
	maxExpire := end + cfg.Expiry.Milliseconds()

	assert.Equal(t, 1, calls)
	assert.Equal(t, int64(123), gotID)
	assert.GreaterOrEqual(t, gotExpireTime, minExpire)
	assert.LessOrEqual(t, gotExpireTime, maxExpire)
	assert.Equal(t, int64(7_777), gotExpectToken)
	assert.GreaterOrEqual(t, ins.expireTime, minExpire)
	assert.LessOrEqual(t, ins.expireTime, maxExpire)
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
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance: store,
	}, testRegistryConfig())

	ins := &Instance{
		id:           1,
		expireTime:   20_000,
		fencingToken: 2,
	}
	r.heartbeat(context.Background(), ins)

	assert.Equal(t, 3, calls)
	assert.Equal(t, int64(20_000), ins.expireTime)
}

func TestRegistry_Heartbeat_FencingMismatch(t *testing.T) {
	var (
		updateCalls int
		createCalls int
		appendCalls int
		getRevCalls int
		incrCalls   int
		nextID      int64 = 880
		curRev      int64 = 7
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
		getOrInitForUpdateFn: func(_ context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error) {
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
	cfg := testRegistryConfig()
	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance:       store,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, cfg)

	ins := &Instance{
		id:           1,
		group:        testInstanceGroup,
		key:          "flow/instances/fencing-mismatch",
		value:        "v",
		expireTime:   30_000,
		fencingToken: 2,
	}
	oldID := ins.id

	r.heartbeat(testTxContext(), ins)

	assert.NotEqual(t, oldID, ins.id)
	assert.Equal(t, 1, updateCalls)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, appendCalls)
	assert.Equal(t, 1, getRevCalls)
	assert.Equal(t, 1, incrCalls)
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

	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance:       store,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	ins := &Instance{
		id:           1,
		group:        testInstanceGroup,
		key:          "flow/instances/old",
		value:        "v",
		expireTime:   12345,
		fencingToken: 99,
	}
	oldID := ins.id

	r.heartbeat(testTxContext(), ins)

	assert.NotEqual(t, oldID, ins.id)
	assert.Equal(t, 1, updateCalls)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, appendCalls)
	assert.Equal(t, 1, getRevCalls)
	assert.Equal(t, 1, incrCalls)
}

func TestRegistry_Heartbeat_AutoReRegister_UsesHeartbeatCtx(t *testing.T) {
	var (
		createCalls int
		nextID      int64 = 2000
		curRev      int64 = 20
	)
	type ctxKey struct{}
	markerKey := ctxKey{}
	const markerValue = "heartbeat-ctx"

	instanceStore := &fakeInstanceStore{
		updateExpireTimeFn: func(_ context.Context, _ int64, _ int64, _ int64) (bool, error) {
			return false, nil
		},
		createFn: func(ctx context.Context, ins *schema.Instance) (*schema.Instance, error) {
			createCalls++
			if ctx.Err() != nil {
				return nil, stderr.New("register ctx should not be canceled")
			}
			if got := ctx.Value(markerKey); got != markerValue {
				return nil, stderr.New("register ctx should come from heartbeat ctx")
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

	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	ins := &Instance{
		id:           1,
		group:        testInstanceGroup,
		key:          "flow/instances/old-parent",
		value:        "v",
		expireTime:   12345,
		fencingToken: 99,
	}
	heartbeatCtx := context.WithValue(testTxContext(), markerKey, markerValue)
	r.heartbeat(heartbeatCtx, ins)

	assert.Equal(t, 1, createCalls)
}

func TestRegistry_Heartbeat_AutoReRegister_RegisterFailed_KeepOld(t *testing.T) {
	var (
		createCalls int
		appendCalls int
		incrCalls   int
		getRevCalls int
		curRev      int64 = 30
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
	events := &fakeInstanceEventStore{
		appendFn: func(_ context.Context, _ *schema.InstanceEvent) error {
			appendCalls++
			return nil
		},
	}

	r := NewRegistry(&repository.TxManager{}, &repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  events,
		GlobalRevision: revs,
	}, RegistryConfig{
		Expiry:            12 * time.Second,
		KeepaliveInterval: time.Hour,
	})
	ins := &Instance{
		id:           2,
		group:        testInstanceGroup,
		key:          "flow/instances/old-keep",
		value:        "v",
		expireTime:   12345,
		fencingToken: 100,
	}
	oldID := ins.id
	r.heartbeat(testTxContext(), ins)

	assert.Equal(t, 1, createCalls)
	assert.Equal(t, oldID, ins.id)
	assert.Equal(t, 1, getRevCalls)
	assert.Equal(t, 0, appendCalls)
	assert.Equal(t, 0, incrCalls)
}

func TestRegistry_TryAutoReRegister_Closing_NoOp(t *testing.T) {
	var createCalls int

	r := NewRegistry(&repository.TxManager{}, &repository.Store{
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
	ins := &Instance{
		id:           3,
		group:        testInstanceGroup,
		key:          "flow/instances/old-closing",
		value:        "v",
		expireTime:   12345,
		fencingToken: 101,
	}
	r.closing.Store(true)

	err := r.tryAutoReRegister(context.Background(), ins)
	require.NoError(t, err)
	assert.Equal(t, 0, createCalls)
}
