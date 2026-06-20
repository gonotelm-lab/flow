package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanInstanceEvents(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM instance_events")
}

func newTestEvent(revision int64, group, key, eventType string) *schema.InstanceEvent {
	return &schema.InstanceEvent{
		Revision:   revision,
		Group:      group,
		Key:        key,
		Value:      fmt.Sprintf(`{"addr":"10.0.0.%d"}`, revision),
		Type:       eventType,
		CreateTime: nowMs(),
	}
}

func TestInstanceEventStore_Append(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	ev := newTestEvent(1, "grp-a", "key-1", "PUT")
	err := gTestInstanceEventStore.Append(ctx, ev)
	require.NoError(t, err)
}

func TestInstanceEventStore_Append_DuplicateRevision(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	ev1 := newTestEvent(1, "grp-a", "key-1", "PUT")
	err := gTestInstanceEventStore.Append(ctx, ev1)
	require.NoError(t, err)

	ev2 := newTestEvent(1, "grp-a", "key-2", "PUT")
	err = gTestInstanceEventStore.Append(ctx, ev2)
	assert.ErrorIs(t, err, sql.ErrDuplicatedKey)
}

func TestInstanceEventStore_Last(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	events := []*schema.InstanceEvent{
		newTestEvent(10, "grp-a", "key-1", "PUT"),
		newTestEvent(20, "grp-a", "key-2", "PUT"),
		newTestEvent(30, "grp-a", "key-1", "DELETE"),
	}
	for _, ev := range events {
		require.NoError(t, gTestInstanceEventStore.Append(ctx, ev))
	}

	got, err := gTestInstanceEventStore.Last(ctx, "grp-a")
	require.NoError(t, err)
	assert.Equal(t, int64(30), got.Revision)
	assert.Equal(t, "DELETE", got.Type)
}

func TestInstanceEventStore_Last_NotFound(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	_, err := gTestInstanceEventStore.Last(ctx, "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestInstanceEventStore_Last_MultiGroup(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(1, "grp-a", "k1", "PUT")))
	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(2, "grp-b", "k2", "PUT")))
	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(3, "grp-a", "k3", "PUT")))

	got, err := gTestInstanceEventStore.Last(ctx, "grp-b")
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.Revision)
}

func TestInstanceEventStore_List(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	for i := int64(1); i <= 5; i++ {
		ev := newTestEvent(i, "grp-a", fmt.Sprintf("key-%d", i), "PUT")
		require.NoError(t, gTestInstanceEventStore.Append(ctx, ev))
	}

	got, err := gTestInstanceEventStore.List(ctx, "grp-a", 0, 10)
	require.NoError(t, err)
	assert.Len(t, got, 5)
	assert.Equal(t, int64(1), got[0].Revision)
	assert.Equal(t, int64(5), got[4].Revision)
}

func TestInstanceEventStore_List_AfterRevision(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	for i := int64(1); i <= 5; i++ {
		ev := newTestEvent(i, "grp-a", fmt.Sprintf("key-%d", i), "PUT")
		require.NoError(t, gTestInstanceEventStore.Append(ctx, ev))
	}

	got, err := gTestInstanceEventStore.List(ctx, "grp-a", 3, 10)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, int64(4), got[0].Revision)
	assert.Equal(t, int64(5), got[1].Revision)
}

func TestInstanceEventStore_List_WithLimit(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	for i := int64(1); i <= 5; i++ {
		ev := newTestEvent(i, "grp-a", fmt.Sprintf("key-%d", i), "PUT")
		require.NoError(t, gTestInstanceEventStore.Append(ctx, ev))
	}

	got, err := gTestInstanceEventStore.List(ctx, "grp-a", 0, 3)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, int64(3), got[2].Revision)
}

func TestInstanceEventStore_List_FilterByGroup(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(1, "grp-a", "k1", "PUT")))
	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(2, "grp-b", "k2", "PUT")))
	require.NoError(t, gTestInstanceEventStore.Append(ctx, newTestEvent(3, "grp-a", "k3", "PUT")))

	got, err := gTestInstanceEventStore.List(ctx, "grp-a", 0, 10)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, ev := range got {
		assert.Equal(t, "grp-a", ev.Group)
	}
}

func TestInstanceEventStore_List_Empty(t *testing.T) {
	cleanInstanceEvents(t)
	ctx := context.Background()

	got, err := gTestInstanceEventStore.List(ctx, "grp-a", 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}
