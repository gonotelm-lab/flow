package e2e

import (
	"context"
	"testing"

	task "github.com/gonotelm-lab/flow/client/task/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTaskClient(t *testing.T) {
	requireE2E(t)

	ctx := context.Background()
	nsA, keyA := newNamespace(t)
	nsB, keyB := newNamespace(t)

	client, err := task.New(serverAddr(t), task.Config{Namespace: nsA, APIKey: keyA})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	created, err := client.Submit(ctx, "e2e", []byte(`{"msg":"hi"}`))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	assert.Equal(t, nsA, created.Namespace)

	got, err := client.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, task.StateInited, got.State)

	other, err := task.New(serverAddr(t), task.Config{Namespace: nsB, APIKey: keyB})
	require.NoError(t, err)
	defer func() { _ = other.Close() }()

	if !authEnabled(t) {
		// 鉴权关闭：key 被完全忽略，跨 namespace 也照旧可用——与加鉴权之前一致。
		keyless, err := task.New(serverAddr(t), task.Config{Namespace: nsA})
		require.NoError(t, err)
		defer func() { _ = keyless.Close() }()
		_, err = keyless.Submit(ctx, "e2e", nil)
		require.NoError(t, err, "鉴权关闭时不带 key 也应成功")

		_, err = other.Get(ctx, created.ID)
		require.NoError(t, err, "鉴权关闭时跨 namespace 读任务仍可用")
		return
	}

	// 不带 key / key 不认识 → UNAUTHENTICATED
	keyless, err := task.New(serverAddr(t), task.Config{Namespace: nsA})
	require.NoError(t, err)
	defer func() { _ = keyless.Close() }()
	_, err = keyless.Submit(ctx, "e2e", nil)
	assert.Equal(t, codes.Unauthenticated, status.Code(err), err)

	badKey, err := task.New(serverAddr(t), task.Config{Namespace: nsA, APIKey: "sk-not-a-real-key"})
	require.NoError(t, err)
	defer func() { _ = badKey.Close() }()
	_, err = badKey.Submit(ctx, "e2e", nil)
	assert.Equal(t, codes.Unauthenticated, status.Code(err), err)

	// 请求里写了别人的 namespace → PERMISSION_DENIED
	cross, err := task.New(serverAddr(t), task.Config{Namespace: nsB, APIKey: keyA})
	require.NoError(t, err)
	defer func() { _ = cross.Close() }()
	_, err = cross.Submit(ctx, "e2e", nil)
	assert.Equal(t, codes.PermissionDenied, status.Code(err), err)

	// 用 B 的 key 按 id 读/取消 A 的任务 → NOT_FOUND，且状态不变
	_, err = other.Get(ctx, created.ID)
	assert.Equal(t, codes.NotFound, status.Code(err), err)
	assert.Equal(t, codes.NotFound, status.Code(other.Cancel(ctx, created.ID)), "cross-namespace cancel")

	after, err := client.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, task.StateInited, after.State, "跨 namespace 的取消不该生效")

	// 自己的任务可以正常取消
	require.NoError(t, client.Cancel(ctx, created.ID))
	cancelled, err := client.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, task.StateCancelled, cancelled.State)
}
