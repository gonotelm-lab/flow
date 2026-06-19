package instance

import (
	"context"
	stderr "errors"
	"sync"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultProducerTimeout = 3 * time.Second
	defaultWatchTimeout    = 5 * time.Second
)

func TestRegistryWatcher_RegisterAndUnregisterEvents(t *testing.T) {
	watchReady := make(chan struct{})
	var watchReadyOnce sync.Once
	store, _ := newDiscoveryInMemoryStore(0, func() {
		watchReadyOnce.Do(func() { close(watchReady) })
	})

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            time.Second,
		KeepaliveInterval: 20 * time.Millisecond,
	})
	defer registry.Close()

	watcher := NewWatcher(&store, WatcherConfig{
		Interval:        time.Millisecond,
		BatchSize:       16,
		MaxRetryBackoff: 10 * time.Millisecond,
	})

	watchCtx, watchCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer watchCancel()

	acc := newWatchAccumulator(0)
	watchErrCh := make(chan error, 1)
	go func() {
		watchErrCh <- watcher.Watch(
			watchCtx,
			testInstanceGroup,
			acc.callback(2, watchCancel),
		)
	}()

	select {
	case <-watchReady:
	case <-time.After(time.Second):
		t.Fatal("watcher did not initialize in time")
	}

	producerErrCh := runRegisterUnregisterRoundsAsync(registry, 1)
	requireNoErrorWithin(t, producerErrCh, time.Second, "register/unregister goroutine timed out")
	requireNoErrorWithin(t, watchErrCh, 2*time.Second, "watch goroutine did not exit in time")

	got := acc.snapshot()
	assert.Equal(t, 2, got.totalSeen)
	assert.Equal(t, 1, got.putSeen)
	assert.Equal(t, 1, got.deleteSeen)
	assert.Equal(t, int64(2), got.lastRev)
	assert.False(t, got.orderBroken)
}

type discoveryInMemoryState struct {
	mu sync.Mutex

	nextID          int64
	currentRevision int64
	instances       map[int64]*schema.Instance
	events          []*schema.InstanceEvent

	listCalls             int
	listFailuresRemaining int
}

func newDiscoveryInMemoryStore(
	listFailures int,
	onGetRevision func(),
) (repository.Store, *discoveryInMemoryState) {
	state := &discoveryInMemoryState{
		nextID:                1,
		instances:             make(map[int64]*schema.Instance),
		events:                make([]*schema.InstanceEvent, 0, 128),
		listFailuresRemaining: listFailures,
	}

	store := repository.Store{
		Instance: &fakeInstanceStore{
			createFn: func(_ context.Context, ins *schema.Instance) (*schema.Instance, error) {
				state.mu.Lock()
				defer state.mu.Unlock()

				copied := *ins
				copied.Id = state.nextID
				state.nextID++
				state.instances[copied.Id] = &copied
				return &copied, nil
			},
			deleteFn: func(_ context.Context, id int64) error {
				state.mu.Lock()
				defer state.mu.Unlock()
				delete(state.instances, id)
				return nil
			},
			updateExpireTimeFn: func(
				_ context.Context,
				id int64,
				expireTimeMs int64,
				expectToken int64,
			) (bool, error) {
				state.mu.Lock()
				defer state.mu.Unlock()

				inst, ok := state.instances[id]
				if !ok {
					return false, nil
				}
				if inst.FencingToken != expectToken {
					return false, nil
				}

				inst.ExpireTime = expireTimeMs
				return true, nil
			},
		},
		InstanceEvent: &fakeInstanceEventStore{
			appendFn: func(_ context.Context, event *schema.InstanceEvent) error {
				state.mu.Lock()
				defer state.mu.Unlock()

				copied := *event
				state.events = append(state.events, &copied)
				return nil
			},
			listFn: func(
				_ context.Context,
				group string,
				lastRevision int64,
				limit int,
			) ([]*schema.InstanceEvent, error) {
				state.mu.Lock()
				defer state.mu.Unlock()

				state.listCalls++
				if state.listFailuresRemaining > 0 {
					state.listFailuresRemaining--
					return nil, stderr.New("temporary list failure")
				}

				result := make([]*schema.InstanceEvent, 0, limit)
				for _, event := range state.events {
					if event.Group != group || event.Revision <= lastRevision {
						continue
					}

					copied := *event
					result = append(result, &copied)
					if limit > 0 && len(result) >= limit {
						break
					}
				}

				return result, nil
			},
		},
		GlobalRevision: &fakeGlobalRevisionStore{
			getOrInitForUpdateFn: func(
				_ context.Context,
				zero *schema.GlobalRevision,
			) (*schema.GlobalRevision, error) {
				state.mu.Lock()
				defer state.mu.Unlock()

				return &schema.GlobalRevision{
					Name:            zero.Name,
					CurrentRevision: state.currentRevision,
					UpdateTime:      time.Now().UnixMilli(),
				}, nil
			},
			incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
				state.mu.Lock()
				defer state.mu.Unlock()
				state.currentRevision++
				return nil
			},
			getFn: func(_ context.Context, name string) (*schema.GlobalRevision, error) {
				if onGetRevision != nil {
					onGetRevision()
				}

				state.mu.Lock()
				defer state.mu.Unlock()

				return &schema.GlobalRevision{
					Name:            name,
					CurrentRevision: state.currentRevision,
					UpdateTime:      time.Now().UnixMilli(),
				}, nil
			},
		},
	}

	return store, state
}

func (s *discoveryInMemoryState) snapshot() (currentRevision int64, listCalls int, listFailuresRemaining int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.currentRevision, s.listCalls, s.listFailuresRemaining
}

type watchSnapshot struct {
	totalSeen   int
	putSeen     int
	deleteSeen  int
	lastRev     int64
	orderBroken bool
}

type watchAccumulator struct {
	mu          sync.Mutex
	totalSeen   int
	putSeen     int
	deleteSeen  int
	lastRev     int64
	orderBroken bool
}

func newWatchAccumulator(startRevision int64) *watchAccumulator {
	return &watchAccumulator{
		lastRev: startRevision,
	}
}

func (a *watchAccumulator) callback(expectedTotal int, cancel context.CancelFunc) WatchCallback {
	return func(_ context.Context, event *InstanceEvent) error {
		a.mu.Lock()
		if event.Revision != a.lastRev+1 {
			a.orderBroken = true
		}
		a.lastRev = event.Revision
		switch event.EventType {
		case InstanceEventPut:
			a.putSeen++
		case InstanceEventDelete:
			a.deleteSeen++
		}
		a.totalSeen++
		done := a.totalSeen == expectedTotal
		a.mu.Unlock()

		if done {
			cancel()
		}
		return nil
	}
}

func (a *watchAccumulator) snapshot() watchSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	return watchSnapshot{
		totalSeen:   a.totalSeen,
		putSeen:     a.putSeen,
		deleteSeen:  a.deleteSeen,
		lastRev:     a.lastRev,
		orderBroken: a.orderBroken,
	}
}

func runRegisterUnregisterRounds(registry *Registry, rounds int) error {
	for i := 0; i < rounds; i++ {
		ins, err := registry.Register(testTxContext(), testInstanceGroup)
		if err != nil {
			return err
		}
		if err := registry.Unregister(testTxContext(), ins.Id); err != nil {
			return err
		}
	}
	return nil
}

func runRegisterUnregisterRoundsAsync(registry *Registry, rounds int) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- runRegisterUnregisterRounds(registry, rounds)
	}()
	return errCh
}

func requireNoErrorWithin(t *testing.T, errCh <-chan error, timeout time.Duration, timeoutMsg string) {
	t.Helper()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(timeout):
		t.Fatal(timeoutMsg)
	}
}

func runWatchWithRevisionAndProducer(
	t *testing.T,
	watcher *Watcher,
	registry *Registry,
	startRevision int64,
	rounds int,
) watchSnapshot {
	t.Helper()

	watchCtx, watchCancel := context.WithTimeout(context.Background(), defaultWatchTimeout)
	defer watchCancel()

	acc := newWatchAccumulator(startRevision)
	watchErrCh := make(chan error, 1)
	go func() {
		watchErrCh <- watcher.WatchWithRevision(
			watchCtx,
			testInstanceGroup,
			startRevision,
			acc.callback(rounds*2, watchCancel),
		)
	}()

	producerErrCh := runRegisterUnregisterRoundsAsync(registry, rounds)
	requireNoErrorWithin(t, producerErrCh, defaultProducerTimeout, "producer goroutine timed out")
	requireNoErrorWithin(t, watchErrCh, defaultWatchTimeout, "watch goroutine timed out")

	return acc.snapshot()
}

func TestRegistryWatcher_BurstEventsWithSmallBatch(t *testing.T) {
	const rounds = 40

	store, state := newDiscoveryInMemoryStore(0, nil)

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            time.Second,
		KeepaliveInterval: time.Second,
	})
	defer registry.Close()

	watcher := NewWatcher(&store, WatcherConfig{
		Interval:        time.Millisecond,
		BatchSize:       1, // 极端场景：最小批次，强制分页轮询。
		MaxRetryBackoff: 10 * time.Millisecond,
	})

	got := runWatchWithRevisionAndProducer(t, watcher, registry, 0, rounds)

	assert.Equal(t, rounds*2, got.totalSeen)
	assert.Equal(t, rounds, got.putSeen)
	assert.Equal(t, rounds, got.deleteSeen)
	assert.Equal(t, int64(rounds*2), got.lastRev)
	assert.False(t, got.orderBroken, "revision should stay strictly increasing in burst scenario")

	currentRevision, _, _ := state.snapshot()
	assert.Equal(t, int64(rounds*2), currentRevision)
}

func TestRegistryWatcher_BurstWithTransientListFailure(t *testing.T) {
	const (
		rounds       = 20
		listFailures = 6
	)

	store, state := newDiscoveryInMemoryStore(listFailures, nil)

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            time.Second,
		KeepaliveInterval: time.Second,
	})
	defer registry.Close()

	watcher := NewWatcher(&store, WatcherConfig{
		Interval:        time.Millisecond,
		BatchSize:       2,
		MaxRetryBackoff: 20 * time.Millisecond,
	})

	got := runWatchWithRevisionAndProducer(t, watcher, registry, 0, rounds)

	assert.Equal(t, rounds*2, got.totalSeen)
	assert.Equal(t, rounds, got.putSeen)
	assert.Equal(t, rounds, got.deleteSeen)
	assert.Equal(t, int64(rounds*2), got.lastRev)
	assert.False(t, got.orderBroken, "revision should stay strictly increasing after transient failures")

	currentRevision, listCalls, remainingFailures := state.snapshot()
	assert.Equal(t, int64(rounds*2), currentRevision)
	assert.Greater(t, listCalls, listFailures)
	assert.Equal(t, 0, remainingFailures)
}

func TestRegistryWatcher_ResumeFromNonZeroRevision(t *testing.T) {
	const (
		preRounds  = 12
		postRounds = 15
	)

	store, state := newDiscoveryInMemoryStore(0, nil)

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            time.Second,
		KeepaliveInterval: time.Second,
	})
	defer registry.Close()

	// 先制造一批历史事件，模拟 watcher 重启恢复场景。
	require.NoError(t, runRegisterUnregisterRounds(registry, preRounds))

	startRevision, _, _ := state.snapshot()
	require.Equal(t, int64(preRounds*2), startRevision)

	watcher := NewWatcher(&store, WatcherConfig{
		Interval:        time.Millisecond,
		BatchSize:       1,
		MaxRetryBackoff: 10 * time.Millisecond,
	})

	got := runWatchWithRevisionAndProducer(t, watcher, registry, startRevision, postRounds)

	assert.Equal(t, postRounds*2, got.totalSeen)
	assert.Equal(t, postRounds, got.putSeen)
	assert.Equal(t, postRounds, got.deleteSeen)
	assert.Equal(t, startRevision+int64(postRounds*2), got.lastRev)
	assert.False(t, got.orderBroken, "revision should stay strictly increasing after resume")

	currentRevision, _, _ := state.snapshot()
	assert.Equal(t, startRevision+int64(postRounds*2), currentRevision)
}
