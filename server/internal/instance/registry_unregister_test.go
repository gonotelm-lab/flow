package instance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lifecycleStoreState 是一个最小内存 store，语义与 postgres 实现保持一致：
// Delete 用 RowsAffected 判定是否真删除了行，DeleteExpired 带过期条件判定。
type lifecycleStoreState struct {
	mu       sync.Mutex
	events   []*schema.InstanceEvent
	rows     map[int64]*schema.Instance
	nextID   int64
	revision int64
}

func newLifecycleStore() (repository.Store, *lifecycleStoreState) {
	st := &lifecycleStoreState{nextID: 1, rows: map[int64]*schema.Instance{}}

	store := repository.Store{
		Instance: &fakeInstanceStore{
			createFn: func(_ context.Context, ins *schema.Instance) (*schema.Instance, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				copied := *ins
				copied.Id = st.nextID
				st.nextID++
				st.rows[copied.Id] = &copied
				return &copied, nil
			},
			deleteFn: func(_ context.Context, id int64) (bool, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				if _, ok := st.rows[id]; !ok {
					return false, nil
				}
				delete(st.rows, id)
				return true, nil
			},
			updateExpireTimeFn: func(_ context.Context, id int64, expireTimeMs int64, _ int64) (bool, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				ins, ok := st.rows[id]
				if !ok {
					return false, nil
				}
				ins.ExpireTime = expireTimeMs
				return true, nil
			},
			listExpiredFn: func(_ context.Context, expireBeforeMs int64, _ int) ([]*schema.Instance, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				var out []*schema.Instance
				for _, ins := range st.rows {
					if ins.ExpireTime <= expireBeforeMs {
						copied := *ins
						out = append(out, &copied)
					}
				}
				return out, nil
			},
			deleteExpiredFn: func(_ context.Context, id int64, expireBeforeMs int64) (bool, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				ins, ok := st.rows[id]
				if !ok || ins.ExpireTime > expireBeforeMs {
					return false, nil
				}
				delete(st.rows, id)
				return true, nil
			},
		},
		InstanceEvent: &fakeInstanceEventStore{
			appendFn: func(_ context.Context, event *schema.InstanceEvent) error {
				st.mu.Lock()
				defer st.mu.Unlock()
				copied := *event
				st.events = append(st.events, &copied)
				return nil
			},
		},
		GlobalRevision: &fakeGlobalRevisionStore{
			getOrInitForUpdateFn: func(_ context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error) {
				st.mu.Lock()
				defer st.mu.Unlock()
				return &schema.GlobalRevision{Name: zero.Name, CurrentRevision: st.revision}, nil
			},
			incrRevisionFn: func(_ context.Context, _ string, _ int64) error {
				st.mu.Lock()
				defer st.mu.Unlock()
				st.revision++
				return nil
			},
		},
	}

	return store, st
}

func (s *lifecycleStoreState) eventSnapshot() ([]*schema.InstanceEvent, map[int64]*schema.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]*schema.InstanceEvent, len(s.events))
	copy(events, s.events)

	rows := make(map[int64]*schema.Instance, len(s.rows))
	for id, row := range s.rows {
		rows[id] = row
	}

	return events, rows
}

func (s *lifecycleStoreState) countEvents(key string, typ InstanceEventType) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, e := range s.events {
		if e.Key == key && e.Type == typ.String() {
			n++
		}
	}
	return n
}

func expireAndSweep(t *testing.T, store *repository.Store) {
	t.Helper()

	time.Sleep(60 * time.Millisecond)
	sweeper := NewSweeper(&repository.TxManager{}, store, SweeperConfig{BatchSize: 10})
	deleted, err := sweeper.SweepOnce(testTxContext())
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "the expired instance should be swept")
}

// 回归：sweeper 先清理过期实例并写入 DELETE 后，本机 Unregister 不得为同一个 key 重复写 DELETE。
func TestRegistry_Unregister_NoDuplicateDeleteAfterSweeperCleanup(t *testing.T) {
	store, st := newLifecycleStore()

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            50 * time.Millisecond,
		KeepaliveInterval: time.Hour, // 心跳静默，模拟进程卡住 / 网络分区
	})
	defer registry.Close()

	ins, err := registry.Register(testTxContext(), testInstanceGroup, "v")
	require.NoError(t, err)

	expireAndSweep(t, &store)
	require.Equal(t, 1, st.countEvents(ins.key, InstanceEventDelete))

	require.NoError(t, registry.Unregister(testTxContext(), ins.id))

	events, _ := st.eventSnapshot()
	for _, e := range events {
		t.Logf("event: rev=%d type=%s key=%s", e.Revision, e.Type, e.Key)
	}
	assert.Equal(t, 1, st.countEvents(ins.key, InstanceEventPut), "expected exactly one PUT")
	assert.Equal(t, 1, st.countEvents(ins.key, InstanceEventDelete),
		"Unregister must not append a second DELETE for an already deleted instance")
	assert.Len(t, events, 2, "no event (and no revision bump) should be produced by the no-op unregister")
}

// 回归：心跳自愈重注册后，locals 必须以新实例 id 为键，否则注销会被跳过（既不删行也不写 DELETE）。
func TestRegistry_AutoReRegister_RekeysLocals(t *testing.T) {
	store, st := newLifecycleStore()

	registry := NewRegistry(&repository.TxManager{}, &store, RegistryConfig{
		Expiry:            50 * time.Millisecond,
		KeepaliveInterval: time.Hour,
	})
	defer registry.Close()

	ins, err := registry.Register(testTxContext(), testInstanceGroup, "v")
	require.NoError(t, err)
	oldID, oldKey := ins.id, ins.key

	expireAndSweep(t, &store)
	require.Equal(t, 1, st.countEvents(oldKey, InstanceEventDelete), "sweeper writes DELETE for the stale instance")

	// 心跳发现 update 落空 → 自动重注册（新行、新 key）。
	registry.heartbeat(testTxContext(), ins)
	newID, newKey := ins.id, ins.key
	require.NotEqual(t, oldID, newID)
	require.NotEqual(t, oldKey, newKey)

	registry.mu.RLock()
	_, trackedByNewID := registry.locals[newID]
	_, trackedByOldID := registry.locals[oldID]
	localsLen := len(registry.locals)
	registry.mu.RUnlock()

	assert.True(t, trackedByNewID, "locals should be keyed by the current instance id")
	assert.False(t, trackedByOldID, "stale locals entry keyed by the old instance id should be dropped")
	assert.Equal(t, 1, localsLen)

	require.NoError(t, registry.Unregister(testTxContext(), newID))

	_, rows := st.eventSnapshot()
	assert.NotContains(t, rows, newID, "re-registered instance row should be deleted by Unregister")
	assert.Equal(t, 1, st.countEvents(newKey, InstanceEventDelete),
		"Unregister should emit exactly one DELETE for the re-registered instance")
	assert.Equal(t, 1, st.countEvents(newKey, InstanceEventPut))
}
