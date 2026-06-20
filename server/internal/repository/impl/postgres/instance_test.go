package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanInstances(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM instances")
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func newTestInstance(suffix string) *schema.Instance {
	now := nowMs()
	return &schema.Instance{
		Group:          "test-group",
		Key:            fmt.Sprintf("key-%s-%d", suffix, now),
		Value:          `{"addr":"127.0.0.1:8080"}`,
		StartTime:      now,
		ExpireTime:     now + 10_000,
		FencingToken:   now,
		CreateRevision: 1,
	}
}

func TestInstanceStore_Create(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins := newTestInstance("create")
	got, err := gTestInstanceStore.Create(ctx, ins)
	require.NoError(t, err)
	assert.NotZero(t, got.Id)
	assert.Equal(t, ins.Key, got.Key)
}

func TestInstanceStore_Create_DuplicateKey(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins1 := newTestInstance("dup")
	_, err := gTestInstanceStore.Create(ctx, ins1)
	require.NoError(t, err)

	ins2 := newTestInstance("dup2")
	ins2.Key = ins1.Key
	_, err = gTestInstanceStore.Create(ctx, ins2)
	assert.ErrorIs(t, err, sql.ErrDuplicatedKey)
}

func TestInstanceStore_Get(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins := newTestInstance("get")
	created, err := gTestInstanceStore.Create(ctx, ins)
	require.NoError(t, err)

	got, err := gTestInstanceStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, created.Key, got.Key)
	assert.Equal(t, created.FencingToken, got.FencingToken)
}

func TestInstanceStore_Get_NotFound(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	_, err := gTestInstanceStore.Get(ctx, 999999)
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestInstanceStore_ListActive(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	now := nowMs()

	expired := newTestInstance("list-active-expired")
	expired.ExpireTime = now - 1
	_, err := gTestInstanceStore.Create(ctx, expired)
	require.NoError(t, err)

	activeEarly := newTestInstance("list-active-early")
	activeEarly.StartTime = now - 1000
	activeEarly.ExpireTime = now + 10_000
	activeEarlyCreated, err := gTestInstanceStore.Create(ctx, activeEarly)
	require.NoError(t, err)

	activeLate := newTestInstance("list-active-late")
	activeLate.StartTime = now + 1000
	activeLate.ExpireTime = now + 20_000
	activeLateCreated, err := gTestInstanceStore.Create(ctx, activeLate)
	require.NoError(t, err)

	got, err := gTestInstanceStore.ListActive(ctx, now)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, activeEarlyCreated.Id, got[0].Id)
	assert.Equal(t, activeLateCreated.Id, got[1].Id)
}

func TestInstanceStore_Delete(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins := newTestInstance("del")
	created, err := gTestInstanceStore.Create(ctx, ins)
	require.NoError(t, err)

	err = gTestInstanceStore.Delete(ctx, created.Id)
	require.NoError(t, err)

	_, err = gTestInstanceStore.Get(ctx, created.Id)
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestInstanceStore_UpdateExpireTime(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins := newTestInstance("expire")
	created, err := gTestInstanceStore.Create(ctx, ins)
	require.NoError(t, err)

	newExpire := nowMs() + 30_000
	updated, err := gTestInstanceStore.UpdateExpireTime(ctx, created.Id, newExpire, created.FencingToken)
	require.NoError(t, err)
	require.True(t, updated)

	got, err := gTestInstanceStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, newExpire, got.ExpireTime)
}

func TestInstanceStore_UpdateExpireTime_TokenMismatch(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	ins := newTestInstance("expire-token-mismatch")
	created, err := gTestInstanceStore.Create(ctx, ins)
	require.NoError(t, err)

	oldExpire := created.ExpireTime
	newExpire := nowMs() + 30_000
	updated, err := gTestInstanceStore.UpdateExpireTime(ctx, created.Id, newExpire, created.FencingToken+1)
	require.NoError(t, err)
	assert.False(t, updated)

	got, err := gTestInstanceStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, oldExpire, got.ExpireTime)
}

func TestInstanceStore_ListExpired(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	now := nowMs()
	expired := newTestInstance("list-expired")
	expired.ExpireTime = now - 1
	expiredCreated, err := gTestInstanceStore.Create(ctx, expired)
	require.NoError(t, err)

	active := newTestInstance("list-active")
	active.ExpireTime = now + 30_000
	_, err = gTestInstanceStore.Create(ctx, active)
	require.NoError(t, err)

	got, err := gTestInstanceStore.ListExpired(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, expiredCreated.Id, got[0].Id)
}

func TestInstanceStore_DeleteExpired(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	now := nowMs()
	expired := newTestInstance("delete-expired")
	expired.ExpireTime = now - 1
	created, err := gTestInstanceStore.Create(ctx, expired)
	require.NoError(t, err)

	deleted, err := gTestInstanceStore.DeleteExpired(ctx, created.Id, now)
	require.NoError(t, err)
	assert.True(t, deleted)

	_, err = gTestInstanceStore.Get(ctx, created.Id)
	assert.ErrorIs(t, err, sql.ErrNoRecord)
}

func TestInstanceStore_DeleteExpired_NotExpired(t *testing.T) {
	cleanInstances(t)
	ctx := context.Background()

	now := nowMs()
	active := newTestInstance("delete-not-expired")
	active.ExpireTime = now + 30_000
	created, err := gTestInstanceStore.Create(ctx, active)
	require.NoError(t, err)

	deleted, err := gTestInstanceStore.DeleteExpired(ctx, created.Id, now)
	require.NoError(t, err)
	assert.False(t, deleted)

	got, err := gTestInstanceStore.Get(ctx, created.Id)
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
}
