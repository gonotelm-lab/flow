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

func TestSweeper_NextSweepInterval(t *testing.T) {
	s := NewSweeper(&repository.TxManager{}, &repository.Store{}, SweeperConfig{})
	s.cfg.Interval = 100 * time.Millisecond

	min := 80 * time.Millisecond
	max := 120 * time.Millisecond
	for i := 0; i < 100; i++ {
		got := s.nextSweepInterval()
		assert.GreaterOrEqual(t, got, min)
		assert.LessOrEqual(t, got, max)
	}
}

func TestSweeper_SweepOnce_NoExpired(t *testing.T) {
	store := &fakeInstanceStore{
		listExpiredFn: func(_ context.Context, _ int64, _ int) ([]*schema.Instance, error) {
			return nil, nil
		},
	}
	s := NewSweeper(&repository.TxManager{}, &repository.Store{
		Instance: store,
	}, SweeperConfig{})

	deleted, err := s.SweepOnce(testTxContext())
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)
}

func TestSweeper_SweepOnce_ListExpiredError(t *testing.T) {
	s := NewSweeper(&repository.TxManager{}, &repository.Store{
		Instance: &fakeInstanceStore{
			listExpiredFn: func(_ context.Context, _ int64, _ int) ([]*schema.Instance, error) {
				return nil, stderr.New("list failed")
			},
		},
	}, SweeperConfig{})

	deleted, err := s.SweepOnce(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0, deleted)
}

func TestSweeper_SweepOnce_Success(t *testing.T) {
	expired := []*schema.Instance{
		{Id: 1, Group: testInstanceGroup, Key: "k1", Value: "v1"},
		{Id: 2, Group: testInstanceGroup, Key: "k2", Value: "v2"},
	}
	var (
		deleteCalls int
		incrCalls   int
		revisions   []int64
	)

	instanceStore := &fakeInstanceStore{
		listExpiredFn: func(_ context.Context, _ int64, _ int) ([]*schema.Instance, error) {
			return expired, nil
		},
		deleteExpiredFn: func(_ context.Context, id int64, _ int64) (bool, error) {
			deleteCalls++
			return id != 2, nil // 第二条模拟已经被其他实例抢删
		},
	}
	eventStore := &fakeInstanceEventStore{
		appendFn: func(_ context.Context, event *schema.InstanceEvent) error {
			revisions = append(revisions, event.Revision)
			return nil
		},
	}
	revStore := &fakeGlobalRevisionStore{
		getOrInitForUpdateFn: func(_ context.Context, _ *schema.GlobalRevision) (*schema.GlobalRevision, error) {
			return &schema.GlobalRevision{
				Name:            discovRevisionName,
				CurrentRevision: 10,
				UpdateTime:      1,
			}, nil
		},
		incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
			incrCalls++
			return nil
		},
	}
	s := NewSweeper(&repository.TxManager{}, &repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  eventStore,
		GlobalRevision: revStore,
	}, SweeperConfig{})

	deleted, err := s.SweepOnce(testTxContext())
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
	assert.Equal(t, 2, deleteCalls)
	assert.Equal(t, []int64{11}, revisions)
	assert.Equal(t, 1, incrCalls)
}

func TestSweeper_SweepOnce_DeleteError(t *testing.T) {
	instanceStore := &fakeInstanceStore{
		listExpiredFn: func(_ context.Context, _ int64, _ int) ([]*schema.Instance, error) {
			return []*schema.Instance{
				{Id: 1, Group: testInstanceGroup, Key: "k1", Value: "v1"},
			}, nil
		},
		deleteExpiredFn: func(_ context.Context, _ int64, _ int64) (bool, error) {
			return false, stderr.New("delete failed")
		},
	}
	revStore := &fakeGlobalRevisionStore{
		getOrInitForUpdateFn: func(_ context.Context, _ *schema.GlobalRevision) (*schema.GlobalRevision, error) {
			return &schema.GlobalRevision{Name: discovRevisionName, CurrentRevision: 1}, nil
		},
	}
	s := NewSweeper(&repository.TxManager{}, &repository.Store{
		Instance:       instanceStore,
		InstanceEvent:  &fakeInstanceEventStore{},
		GlobalRevision: revStore,
	}, SweeperConfig{})

	deleted, err := s.SweepOnce(testTxContext())
	require.Error(t, err)
	assert.Equal(t, 0, deleted)
}

func TestSweeper_StartClose(t *testing.T) {
	s := NewSweeper(&repository.TxManager{}, &repository.Store{
		Instance: &fakeInstanceStore{
			listExpiredFn: func(_ context.Context, _ int64, _ int) ([]*schema.Instance, error) {
				return nil, nil
			},
		},
	}, SweeperConfig{})
	s.cfg.Interval = 2 * time.Millisecond

	s.Start(testTxContext())
	time.Sleep(8 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Close()
		s.Close() // close 幂等
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweeper close timeout")
	}
}
