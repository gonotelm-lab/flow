package e2e

import (
	"context"
	"testing"

	scheduleclient "github.com/gonotelm-lab/flow/client/schedule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestScheduleClient(t *testing.T) {
	requireE2E(t)

	ctx := context.Background()
	nsA, keyA := newNamespace(t)
	nsB, keyB := newNamespace(t)

	client, err := scheduleclient.New(serverAddr(t), scheduleclient.Config{Namespace: nsA, APIKey: keyA})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	created, err := client.Register(ctx, scheduleclient.Cron("*/5 * * * *", "UTC", "https://example.com/hook", []byte(`{"a":1}`)))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	assert.Equal(t, nsA, created.Namespace)

	got, err := client.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	list, total, err := client.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	ids := make([]string, 0, len(list))
	for _, item := range list {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, created.ID, "自己的 schedule 应当出现在列表里")

	other, err := scheduleclient.New(serverAddr(t), scheduleclient.Config{Namespace: nsB, APIKey: keyB})
	require.NoError(t, err)
	defer func() { _ = other.Close() }()

	if authEnabled(t) {
		// 不带 key → UNAUTHENTICATED
		keyless, err := scheduleclient.New(serverAddr(t), scheduleclient.Config{Namespace: nsA})
		require.NoError(t, err)
		defer func() { _ = keyless.Close() }()
		_, err = keyless.Register(ctx, scheduleclient.Cron("*/5 * * * *", "UTC", "https://example.com/hook", nil))
		assert.Equal(t, codes.Unauthenticated, status.Code(err), err)

		// 请求里写了别人的 namespace → PERMISSION_DENIED
		cross, err := scheduleclient.New(serverAddr(t), scheduleclient.Config{Namespace: nsB, APIKey: keyA})
		require.NoError(t, err)
		defer func() { _ = cross.Close() }()
		_, err = cross.Register(ctx, scheduleclient.Cron("*/5 * * * *", "UTC", "https://example.com/hook", nil))
		assert.Equal(t, codes.PermissionDenied, status.Code(err), err)
		_, _, err = cross.List(ctx)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), err)

		// B 的客户端看不到 A 的 schedule，按 id 读也是 NOT_FOUND
		otherList, otherTotal, err := other.List(ctx)
		require.NoError(t, err)
		assert.Zero(t, otherTotal, "B 不该看到 A 的 schedule")
		assert.Empty(t, otherList)
		_, err = other.Get(ctx, created.ID)
		assert.Equal(t, codes.NotFound, status.Code(err), err)
	}

	// 自己的 key：stop + delete 正常
	stopped, err := client.Stop(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, scheduleclient.StateStopped, stopped.State)
	require.NoError(t, client.Delete(ctx, created.ID))
}
