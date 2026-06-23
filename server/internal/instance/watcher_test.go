package instance

import (
	"context"
	stderr "errors"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustReadWatchResponse(
	t *testing.T,
	watchCh WatchChan,
	timeout time.Duration,
) (WatchResponse, bool) {
	t.Helper()

	select {
	case resp, ok := <-watchCh:
		return resp, ok
	case <-time.After(timeout):
		t.Fatal("watch response timeout")
		return WatchResponse{}, false
	}
}

func requireWatchClosed(t *testing.T, watchCh WatchChan, timeout time.Duration) {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		t.Fatal("watch channel did not close in time")
	default:
	}

	for {
		select {
		case resp, ok := <-watchCh:
			if !ok {
				return
			}
			require.NoError(t, resp.Err())
		case <-timer.C:
			t.Fatal("watch channel did not close in time")
		}
	}
}

func TestWatcher_CurrentRevision_NoRecord(t *testing.T) {
	w := NewWatcher(&repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return nil, pkgerr.NoRecord
			},
		},
	}, WatcherConfig{})

	rev, err := w.CurrentRevision(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), rev)
}

func TestWatcher_CurrentRevision_Error(t *testing.T) {
	w := NewWatcher(&repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return nil, stderr.New("db unavailable")
			},
		},
	}, WatcherConfig{})

	_, err := w.CurrentRevision(context.Background())
	require.Error(t, err)
}

func TestWatcher_WatchWithRevision_Validation(t *testing.T) {
	w := NewWatcher(&repository.Store{}, WatcherConfig{})

	watchCh := w.WatchWithRevision(context.Background(), "", 0)
	resp, ok := mustReadWatchResponse(t, watchCh, time.Second)
	require.True(t, ok)
	require.Error(t, resp.Err())
	assert.True(t, resp.Canceled)
	assert.Contains(t, resp.Err().Error(), "group")
	requireWatchClosed(t, watchCh, time.Second)

	watchCh = w.WatchWithRevision(context.Background(), testInstanceGroup, -1)
	resp, ok = mustReadWatchResponse(t, watchCh, time.Second)
	require.True(t, ok)
	require.Error(t, resp.Err())
	assert.True(t, resp.Canceled)
	assert.Contains(t, resp.Err().Error(), "lastRevision")
	requireWatchClosed(t, watchCh, time.Second)
}

func TestWatcher_Watch_FromCurrentRevision(t *testing.T) {
	var (
		listCalls        int
		gotFirstLastRev  int64 = -1
		gotEventRevision int64
	)
	w := NewWatcher(&repository.Store{
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
							Group:      testInstanceGroup,
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

	watchCh := w.Watch(ctx, testInstanceGroup)
	resp, ok := mustReadWatchResponse(t, watchCh, time.Second)
	require.True(t, ok)
	require.NoError(t, resp.Err())
	require.Len(t, resp.Events, 1)
	gotEventRevision = resp.Events[0].Revision
	cancel()
	requireWatchClosed(t, watchCh, time.Second)

	assert.Equal(t, int64(5), gotFirstLastRev)
	assert.Equal(t, int64(6), gotEventRevision)
}

func TestWatcher_Watch_CurrentRevisionError(t *testing.T) {
	w := NewWatcher(&repository.Store{
		GlobalRevision: &fakeGlobalRevisionStore{
			getFn: func(_ context.Context, _ string) (*schema.GlobalRevision, error) {
				return nil, stderr.New("db unavailable")
			},
		},
	}, WatcherConfig{})

	watchCh := w.Watch(context.Background(), testInstanceGroup)
	resp, ok := mustReadWatchResponse(t, watchCh, time.Second)
	require.True(t, ok)
	require.Error(t, resp.Err())
	assert.True(t, resp.Canceled)
	assert.Contains(t, resp.Err().Error(), "get current revision failed")
	requireWatchClosed(t, watchCh, time.Second)
}

func TestWatcher_Watch_RetryOnListError(t *testing.T) {
	var (
		attempts  int
		callbacks int
	)
	w := NewWatcher(&repository.Store{
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, _ int64, _ int) ([]*schema.InstanceEvent, error) {
				attempts++
				if attempts == 1 {
					return nil, stderr.New("temporary db error")
				}
				return []*schema.InstanceEvent{
					{
						Revision:   2,
						Group:      testInstanceGroup,
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

	watchCh := w.WatchWithRevision(ctx, testInstanceGroup, 1)
	resp, ok := mustReadWatchResponse(t, watchCh, time.Second)
	require.True(t, ok)
	require.NoError(t, resp.Err())
	require.Len(t, resp.Events, 1)
	callbacks += len(resp.Events)
	cancel()
	requireWatchClosed(t, watchCh, time.Second)

	assert.GreaterOrEqual(t, attempts, 2)
	assert.Equal(t, 1, callbacks)
}

func TestWatcher_WatchWithRevision_ContextCanceled(t *testing.T) {
	var calls int
	w := NewWatcher(&repository.Store{
		InstanceEvent: &fakeInstanceEventStore{
			listFn: func(_ context.Context, _ string, _ int64, _ int) ([]*schema.InstanceEvent, error) {
				calls++
				return nil, nil
			},
		},
	}, WatcherConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	watchCh := w.WatchWithRevision(ctx, testInstanceGroup, 0)
	requireWatchClosed(t, watchCh, time.Second)

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
