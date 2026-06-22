package postgres

import (
	"context"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanTaskEvents(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM task_events")
}

func newTestTaskEvent(taskID uuid.UUID, eventType string, createTime int64) *schema.TaskEvent {
	return &schema.TaskEvent{
		TaskId:     taskID,
		EventType:  eventType,
		CreateTime: createTime,
		Payload:    []byte(`{"event":"ok"}`),
	}
}

func TestTaskEventStore_Append(t *testing.T) {
	cleanTaskEvents(t)
	ctx := context.Background()

	event := newTestTaskEvent(uuid.New(), "CLAIMED", 1000)
	err := gTestTaskEventStore.Append(ctx, event)
	require.NoError(t, err)
	assert.NotZero(t, event.Id)
}

func TestTaskEventStore_ListByTaskID(t *testing.T) {
	cleanTaskEvents(t)
	ctx := context.Background()

	taskID := uuid.New()
	otherTaskID := uuid.New()
	first := newTestTaskEvent(taskID, "CLAIMED", 1000)
	second := newTestTaskEvent(taskID, "DONE", 2000)
	other := newTestTaskEvent(otherTaskID, "CLAIMED", 1500)

	require.NoError(t, gTestTaskEventStore.Append(ctx, first))
	require.NoError(t, gTestTaskEventStore.Append(ctx, second))
	require.NoError(t, gTestTaskEventStore.Append(ctx, other))

	got, err := gTestTaskEventStore.ListByTaskID(ctx, taskID, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, first.Id, got[0].Id)
	assert.Equal(t, second.Id, got[1].Id)
}

func TestTaskEventStore_ListByTaskID_Empty(t *testing.T) {
	cleanTaskEvents(t)
	ctx := context.Background()

	got, err := gTestTaskEventStore.ListByTaskID(ctx, uuid.New(), 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}
