package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanNamespaces(t *testing.T) {
	t.Helper()
	gTestDB.Exec("DELETE FROM namespaces")
}

func TestNamespaceStore_Create(t *testing.T) {
	cleanNamespaces(t)
	ctx := context.Background()

	ns := &schema.Namespace{
		Name:        fmt.Sprintf("ns-%d", time.Now().UnixNano()),
		Description: "test",
		ApiKey:      fmt.Sprintf("ak-%d", time.Now().UnixNano()),
		Creator:     "tester",
	}
	got, err := gTestNamespaceStore.Create(ctx, ns)
	require.NoError(t, err)
	assert.NotZero(t, got.Id)
	assert.Equal(t, ns.Name, got.Name)
}

func TestNamespaceStore_Create_DuplicateName(t *testing.T) {
	cleanNamespaces(t)
	ctx := context.Background()

	ns := &schema.Namespace{
		Name:   fmt.Sprintf("dup-%d", time.Now().UnixNano()),
		ApiKey: fmt.Sprintf("ak-dup1-%d", time.Now().UnixNano()),
	}
	_, err := gTestNamespaceStore.Create(ctx, ns)
	require.NoError(t, err)

	ns2 := &schema.Namespace{
		Name:   ns.Name,
		ApiKey: fmt.Sprintf("ak-dup2-%d", time.Now().UnixNano()),
	}
	_, err = gTestNamespaceStore.Create(ctx, ns2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, pkgerr.DuplicatedResource)
}

func TestNamespaceStore_Get(t *testing.T) {
	cleanNamespaces(t)
	ctx := context.Background()

	ns := &schema.Namespace{
		Name:   fmt.Sprintf("get-%d", time.Now().UnixNano()),
		ApiKey: fmt.Sprintf("ak-get-%d", time.Now().UnixNano()),
	}
	created, err := gTestNamespaceStore.Create(ctx, ns)
	require.NoError(t, err)

	got, err := gTestNamespaceStore.Get(ctx, created.Name)
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
}

func TestNamespaceStore_Get_NotFound(t *testing.T) {
	cleanNamespaces(t)
	ctx := context.Background()

	_, err := gTestNamespaceStore.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, pkgerr.NoRecord)
}

func TestNamespaceStore_List(t *testing.T) {
	cleanNamespaces(t)
	ctx := context.Background()

	now := time.Now().UnixNano()
	ns1 := &schema.Namespace{
		Name:        fmt.Sprintf("ns-list-1-%d", now),
		Description: "first",
		ApiKey:      fmt.Sprintf("ak-1-%d", now),
		Creator:     "a",
	}
	ns2 := &schema.Namespace{
		Name:        fmt.Sprintf("ns-list-2-%d", now),
		Description: "second",
		ApiKey:      fmt.Sprintf("ak-2-%d", now),
		Creator:     "b",
	}
	ns3 := &schema.Namespace{
		Name:        fmt.Sprintf("ns-list-3-%d", now),
		Description: "third",
		ApiKey:      fmt.Sprintf("ak-3-%d", now),
		Creator:     "c",
	}

	_, err := gTestNamespaceStore.Create(ctx, ns1)
	require.NoError(t, err)
	_, err = gTestNamespaceStore.Create(ctx, ns2)
	require.NoError(t, err)
	_, err = gTestNamespaceStore.Create(ctx, ns3)
	require.NoError(t, err)

	t.Run("all", func(t *testing.T) {
		got, total, err := gTestNamespaceStore.List(ctx, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 3)
	})

	t.Run("pagination", func(t *testing.T) {
		got, total, err := gTestNamespaceStore.List(ctx, 0, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 2)
	})

	t.Run("offset", func(t *testing.T) {
		got, total, err := gTestNamespaceStore.List(ctx, 2, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 1)
	})

	t.Run("empty", func(t *testing.T) {
		got, total, err := gTestNamespaceStore.List(ctx, 100, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, got, 0)
	})
}
