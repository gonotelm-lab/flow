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

func TestZeroRevision(t *testing.T) {
	rev := zeroRevision()
	require.NotNil(t, rev)
	assert.Equal(t, discovRevisionName, rev.Name)
	assert.Equal(t, int64(0), rev.CurrentRevision)
	assert.Greater(t, rev.UpdateTime, int64(0))
}

func TestRegistry_Register_WhenClosing(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{})
	r.closing.Store(true)

	_, err := r.Register(context.Background())
	require.Error(t, err)
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	r := NewRegistry(repository.TxManager{}, repository.Store{})

	err := r.Unregister(context.Background(), 999)
	require.NoError(t, err)
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
	})

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
	})

	r.instances[100] = &cancellableInstance{
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
	_, ok := r.instances[100]
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
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: store,
	})

	ins := &Instance{
		Id:           123,
		ExpireTime:   10_000,
		FencingToken: 7_777,
	}
	expectedExpire := time.UnixMilli(10_000).Add(defaultExpiry).UnixMilli()

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
	})

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
	r := NewRegistry(repository.TxManager{}, repository.Store{
		Instance: store,
	})

	ins := &Instance{
		Id:           1,
		ExpireTime:   30_000,
		FencingToken: 2,
	}
	expectedExpire := time.UnixMilli(30_000).Add(defaultExpiry).UnixMilli()

	r.heartbeat(context.Background(), ins)

	assert.Equal(t, 1, calls)
	assert.Equal(t, expectedExpire, ins.ExpireTime)
}
