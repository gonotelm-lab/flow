package schedule

import (
	"context"
	"testing"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// 守两件事：namespace 必填的边界（RPC 客户端与纯验签接收方不同），以及凭据有没有真的送出去。

type recordingConn struct {
	gotCtx  context.Context
	gotReq  any
	methods []string
}

func (c *recordingConn) Invoke(
	ctx context.Context, method string, args, reply any, _ ...grpc.CallOption,
) error {
	c.gotCtx = ctx
	c.gotReq = args
	c.methods = append(c.methods, method)

	if out, ok := reply.(*schemav1.Schedule); ok {
		proto.Merge(out, &schemav1.Schedule{Id: "sched-1", Namespace: "demo"})
	}

	return nil
}

func (c *recordingConn) NewStream(
	context.Context, *grpc.StreamDesc, string, ...grpc.CallOption,
) (grpc.ClientStream, error) {
	panic("recordingConn.NewStream: not stubbed")
}

func outgoingAuth(ctx context.Context) []string {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return nil
	}

	return md.Get("authorization")
}

// 构造不校验 Config：namespace 的必填校验在每个 RPC 调用前，纯验签的接收方不需要它。
func TestNew_DoesNotValidateConfig(t *testing.T) {
	t.Parallel()

	client, err := New("localhost:7091", Config{APIKey: "sk-demo"})
	require.NoError(t, err, "New only dials, it does not validate config")

	_, _, err = client.List(context.Background())
	require.ErrorIs(t, err, ErrNoNamespace)
	require.NoError(t, client.Close())

	// 纯验签：addr 留空，只需要 key。
	verifier, err := New("", Config{APIKey: "sk-demo"})
	require.NoError(t, err)
	require.NoError(t, verifier.Close())
}

// NewWithConn 没有 error 位，配置问题由每个方法在调用前返回，不 panic。
func TestNewWithConn_NamespaceValidatedPerCall(t *testing.T) {
	t.Parallel()

	client := NewWithConn(&recordingConn{}, Config{})
	ctx := context.Background()

	_, err := client.Register(ctx, Cron("*/5 * * * *", "UTC", "https://example.com/hook", nil))
	require.ErrorIs(t, err, ErrNoNamespace)

	_, _, err = client.List(ctx)
	require.ErrorIs(t, err, ErrNoNamespace)

	_, err = client.Get(ctx, "sched-1")
	require.ErrorIs(t, err, ErrNoNamespace)

	_, err = client.Stop(ctx, "sched-1")
	require.ErrorIs(t, err, ErrNoNamespace)

	require.ErrorIs(t, client.Delete(ctx, "sched-1"), ErrNoNamespace)
}

func TestRegister_InjectsKeyAndNamespace(t *testing.T) {
	t.Parallel()

	conn := &recordingConn{}
	client := NewWithConn(conn, Config{Namespace: "demo", APIKey: "sk-demo"})

	got, err := client.Register(context.Background(), Cron(
		"*/5 * * * *", "UTC", "https://example.com/hook", []byte(`{"a":1}`),
	))
	require.NoError(t, err)
	require.NotNil(t, got)

	req, ok := conn.gotReq.(*schedulev1.RegisterRequest)
	require.True(t, ok)
	assert.Equal(t, "demo", req.GetSchedule().GetNamespace(), "namespace must come from Config")
	assert.NotEmpty(t, req.GetSchedule().GetIdempotencyKey(), "an idempotency key should be generated when absent")

	assert.Equal(t, []string{"Bearer sk-demo"}, outgoingAuth(conn.gotCtx))
	assert.Equal(t, []string{"/api.schedule.v1.ScheduleService/Register"}, conn.methods)
}

func TestGet_InjectsKey(t *testing.T) {
	t.Parallel()

	conn := &recordingConn{}
	client := NewWithConn(conn, Config{Namespace: "demo", APIKey: "sk-demo"})

	_, err := client.Get(context.Background(), "sched-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer sk-demo"}, outgoingAuth(conn.gotCtx))
}

// 没配 key（服务端默认关闭鉴权）时不能发出任何凭据头。
func TestList_WithoutKeySendsNoCredential(t *testing.T) {
	t.Parallel()

	conn := &recordingConn{}
	client := NewWithConn(conn, Config{Namespace: "demo"})

	_, _, err := client.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, outgoingAuth(conn.gotCtx))
}
