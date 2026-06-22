package postgres

import (
	"context"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanTasks(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM tasks")
}

func newTestTask(namespace, taskType, state string, nextRunTime int64) *schema.Task {
	now := nowMs()
	return &schema.Task{
		Id:          uuid.New(),
		Namespace:   namespace,
		TaskType:    taskType,
		State:       state,
		Payload:     []byte(`{"hello":"world"}`),
		CreateTime:  now,
		NextRunTime: nextRunTime,
		UpdateTime:  now,
		WorkerId:    0,
		MaxRetry:    3,
		AttemptNo:   0,
	}
}

func TestTaskStore_Create(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-create", "email", "pending", 1000)
	got, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, task.Id, got.Id)
	assert.Equal(t, task.Namespace, got.Namespace)

	var saved schema.Task
	err = gTestDB.WithContext(ctx).Where("id = ?", task.Id).Take(&saved).Error
	require.NoError(t, err)
	assert.Equal(t, task.TaskType, saved.TaskType)
	assert.Equal(t, task.State, saved.State)
	assert.Equal(t, string(task.Payload), string(saved.Payload))
}

func TestTaskStore_Create_DuplicateID(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-dup", "email", "pending", 1000)
	_, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)

	dup := newTestTask("ns-dup", "email", "pending", 2000)
	dup.Id = task.Id
	_, err = gTestTaskStore.Create(ctx, dup)
	assert.ErrorIs(t, err, sql.ErrDuplicatedKey)
}

func TestTaskStore_Get(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-get", "email", "pending", 1000)
	created, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)

	got, err := gTestTaskStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, created.Namespace, got.Namespace)
	assert.Equal(t, created.TaskType, got.TaskType)
	assert.Equal(t, created.State, got.State)
}

func TestTaskStore_Get_NotFound(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	_, err := gTestTaskStore.Get(ctx, uuid.New())
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestTaskStore_Claim(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	otherNamespace := newTestTask("ns-other", "email", "pending", 100)
	otherType := newTestTask("ns-claim", "sms", "pending", 200)
	otherState := newTestTask("ns-claim", "email", "done", 300)
	firstMatch := newTestTask("ns-claim", "email", "pending", 400)
	secondMatch := newTestTask("ns-claim", "email", "retry", 500)
	laterMatch := newTestTask("ns-claim", "email", "pending", 600)

	for _, task := range []*schema.Task{
		otherNamespace,
		otherType,
		otherState,
		firstMatch,
		secondMatch,
		laterMatch,
	} {
		_, err := gTestTaskStore.Create(ctx, task)
		require.NoError(t, err)
	}

	got, err := gTestTaskStore.Claim(ctx, "ns-claim", "email", []string{"pending", "retry"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, firstMatch.Id, got.Id)
	assert.Equal(t, firstMatch.NextRunTime, got.NextRunTime)
}

func TestTaskStore_Claim_NotFound(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	_, err := gTestTaskStore.Claim(ctx, "ns-missing", "email", []string{"pending"})
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestTaskStore_ClaimUpdate(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-claim-update", "email", "pending", 1000)
	_, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)

	ok, err := gTestTaskStore.ClaimUpdate(ctx, task.Id, "pending", &store.ClaimUpdateParams{
		WorkerId:   42,
		NewState:   "running",
		UpdateTime: 2000,
	})
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestTaskStore.Get(ctx, task.Id)
	require.NoError(t, err)
	assert.Equal(t, "running", got.State)
	assert.Equal(t, int64(42), got.WorkerId)
	assert.Equal(t, int64(2000), got.UpdateTime)
}

func TestTaskStore_ClaimUpdate_StateMismatch(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-claim-update-mismatch", "email", "pending", 1000)
	_, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)

	ok, err := gTestTaskStore.ClaimUpdate(ctx, task.Id, "running", &store.ClaimUpdateParams{
		WorkerId:   99,
		NewState:   "done",
		UpdateTime: 3000,
	})
	require.NoError(t, err)
	assert.False(t, ok)

	got, err := gTestTaskStore.Get(ctx, task.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending", got.State)
	assert.Equal(t, int64(0), got.WorkerId)
	assert.NotEqual(t, int64(3000), got.UpdateTime)
}

func TestTaskStore_Update(t *testing.T) {
	cleanTasks(t)
	ctx := context.Background()

	task := newTestTask("ns-update", "email", "pending", 1000)
	_, err := gTestTaskStore.Create(ctx, task)
	require.NoError(t, err)

	task.State = "done"
	task.Result = []byte(`{"ok":true}`)
	task.Error = []byte(``)
	task.UpdateTime = 4000
	task.WorkerId = 7
	task.AttemptNo = 2

	ok, err := gTestTaskStore.Update(ctx, task)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := gTestTaskStore.Get(ctx, task.Id)
	require.NoError(t, err)
	assert.Equal(t, "done", got.State)
	assert.Equal(t, string(task.Result), string(got.Result))
	assert.Equal(t, int64(4000), got.UpdateTime)
	assert.Equal(t, int64(7), got.WorkerId)
	assert.Equal(t, 2, got.AttemptNo)
}
