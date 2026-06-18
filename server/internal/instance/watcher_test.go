package instance

import (
	"context"
	stderr "errors"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgsql "github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatcher_CurrentRevision_NoRecord(t *testing.T) {
	w := NewWatcher(repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return nil, pkgsql.ErrNoRecord
			},
		},
	}, WatcherConfig{})

	rev, err := w.CurrentRevision(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), rev)
}

func TestWatcher_CurrentRevision_Error(t *testing.T) {
	w := NewWatcher(repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return nil, stderr.New("db unavailable")
			},
		},
	}, WatcherConfig{})

	_, err := w.CurrentRevision(context.Background())
	require.Error(t, err)
}

func TestWatcher_Watch_Validation(t *testing.T) {
	w := NewWatcher(repository.Store{}, WatcherConfig{})
	cb := func(context.Context, *InstanceEvent) error { return nil }

	err := w.WatchWithRevision(context.Background(), "", 0, cb)
	require.Error(t, err)

	err = w.WatchWithRevision(context.Background(), InstanceGroup, -1, cb)
	require.Error(t, err)

	err = w.WatchWithRevision(context.Background(), InstanceGroup, 0, nil)
	require.Error(t, err)
}

func TestWatcher_Watch_FromCurrentRevision(t *testing.T) {
	var (
		listCalls        int
		gotFirstLastRev  int64 = -1
		gotEventRevision int64
	)
	w := NewWatcher(repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return &schema.GlobalRevision{
					Name:            discovRevisionName,
					CurrentRevision: 5,
				}, nil
			},
		},
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, lastRevision int64, _ int) ([]*schema.InstanceEvent, error) {
				listCalls++
				if listCalls == 1 {
					gotFirstLastRev = lastRevision
					return []*schema.InstanceEvent{
						{
							Revision:   6,
							Group:      InstanceGroup,
							Key:        "k1",
							Value:      "v1",
							Type:       InstanceEventPut.String(),
							CreateTime: 123,
						},
					}, nil
				}
				return nil, nil
			},
		},
	}, WatcherConfig{})
	w.cfg.Interval = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := w.Watch(ctx, InstanceGroup, func(_ context.Context, event *InstanceEvent) error {
		gotEventRevision = event.Revision
		cancel()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), gotFirstLastRev)
	assert.Equal(t, int64(6), gotEventRevision)
}

func TestWatcher_Watch_CallbackError(t *testing.T) {
	var callbackCalls int
	w := NewWatcher(repository.Store{
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, _ int64, _ int) ([]*schema.InstanceEvent, error) {
				return []*schema.InstanceEvent{
					{
						Revision:   1,
						Group:      InstanceGroup,
						Key:        "k1",
						Value:      "v1",
						Type:       InstanceEventPut.String(),
						CreateTime: 123,
					},
				}, nil
			},
		},
	}, WatcherConfig{})

	err := w.WatchWithRevision(context.Background(), InstanceGroup, 0, func(_ context.Context, _ *InstanceEvent) error {
		callbackCalls++
		return stderr.New("callback failed")
	})
	require.Error(t, err)
	assert.Equal(t, 1, callbackCalls)
}

func TestWatcher_Watch_RetryOnListError(t *testing.T) {
	var (
		attempts  int
		callbacks int
	)
	w := NewWatcher(repository.Store{
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, _ int64, _ int) ([]*schema.InstanceEvent, error) {
				attempts++
				if attempts == 1 {
					return nil, stderr.New("temporary db error")
				}
				return []*schema.InstanceEvent{
					{
						Revision:   2,
						Group:      InstanceGroup,
						Key:        "k2",
						Value:      "v2",
						Type:       InstanceEventDelete.String(),
						CreateTime: 234,
					},
				}, nil
			},
		},
	}, WatcherConfig{})
	w.cfg.Interval = time.Millisecond
	w.cfg.MaxRetryBackoff = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := w.WatchWithRevision(ctx, InstanceGroup, 1, func(_ context.Context, _ *InstanceEvent) error {
		callbacks++
		cancel()
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
	assert.Equal(t, 1, callbacks)
}

func TestWatcher_WatchWithRevision_ContextCanceled(t *testing.T) {
	var calls int
	w := NewWatcher(repository.Store{
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, _ int64, _ int) ([]*schema.InstanceEvent, error) {
				calls++
				return nil, nil
			},
		},
	}, WatcherConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.WatchWithRevision(ctx, InstanceGroup, 0, func(_ context.Context, _ *InstanceEvent) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, calls)
}

func TestGrowBackoff(t *testing.T) {
	assert.Equal(t, 2*time.Second, growBackoff(time.Second, 5*time.Second))
	assert.Equal(t, 5*time.Second, growBackoff(4*time.Second, 5*time.Second))
	assert.Equal(t, defaultWatchInterval*2, growBackoff(0, 0))
}

func TestSleepContext_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepContext(ctx, time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
