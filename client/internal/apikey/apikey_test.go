package apikey

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// recordingConn 记下每次 Invoke 收到的 ctx，用来断言凭据有没有真的送出去。
type recordingConn struct {
	gotCtx context.Context
	calls  int
}

func (c *recordingConn) Invoke(
	ctx context.Context, _ string, _, _ any, _ ...grpc.CallOption,
) error {
	c.gotCtx = ctx
	c.calls++
	return nil
}

func (c *recordingConn) NewStream(
	context.Context, *grpc.StreamDesc, string, ...grpc.CallOption,
) (grpc.ClientStream, error) {
	panic("recordingConn.NewStream: not stubbed")
}

func outgoingAuth(t *testing.T, ctx context.Context) []string {
	t.Helper()

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return nil
	}

	return md.Get(Header)
}

func TestWithKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want []string
	}{
		{name: "injects the credential", key: "sk-demo", want: []string{"Bearer sk-demo"}},
		{name: "empty key injects nothing", key: "", want: nil},
		{name: "blank key injects nothing", key: "   ", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := WithKey(context.Background(), tt.key)
			assert.Equal(t, tt.want, outgoingAuth(t, ctx))
		})
	}
}

func TestWrapConn_InjectsOnEveryCall(t *testing.T) {
	t.Parallel()

	inner := &recordingConn{}
	conn := WrapConn(inner, "sk-demo")

	require.NoError(t, conn.Invoke(context.Background(), "/api.task.v1.TaskService/Submit", nil, nil))
	require.NoError(t, conn.Invoke(context.Background(), "/api.task.v1.TaskService/Get", nil, nil))

	assert.Equal(t, 2, inner.calls)
	assert.Equal(t, []string{"Bearer sk-demo"}, outgoingAuth(t, inner.gotCtx))
}

// 没有 key 时不包一层：默认关闭鉴权的服务端不该收到任何多余的头。
func TestWrapConn_WithoutKeyIsTransparent(t *testing.T) {
	t.Parallel()

	inner := &recordingConn{}
	conn := WrapConn(inner, "")

	assert.Same(t, grpc.ClientConnInterface(inner), conn)

	require.NoError(t, conn.Invoke(context.Background(), "/api.task.v1.TaskService/Submit", nil, nil))
	assert.Empty(t, outgoingAuth(t, inner.gotCtx))
}

func TestWrapConn_NilConn(t *testing.T) {
	t.Parallel()

	assert.Nil(t, WrapConn(nil, "sk-demo"))
}
