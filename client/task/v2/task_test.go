package task

import (
	"context"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	taskv1 "github.com/gonotelm-lab/flow/api/task/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeConn 记录每次调用收到的 ctx 与请求，并按需填回响应。
type fakeConn struct {
	gotCtx  context.Context
	gotReq  any
	submit  *taskv1.SubmitResponse
	get     *taskv1.GetResponse
	calls   int
	methods []string
}

func (f *fakeConn) Invoke(
	ctx context.Context, method string, args, reply any, _ ...grpc.CallOption,
) error {
	f.gotCtx = ctx
	f.gotReq = args
	f.calls++
	f.methods = append(f.methods, method)

	switch out := reply.(type) {
	case *taskv1.SubmitResponse:
		if f.submit != nil {
			proto.Merge(out, f.submit)
		}
	case *taskv1.GetResponse:
		if f.get != nil {
			proto.Merge(out, f.get)
		}
	}

	return nil
}

func (f *fakeConn) NewStream(
	context.Context, *grpc.StreamDesc, string, ...grpc.CallOption,
) (grpc.ClientStream, error) {
	panic("fakeConn.NewStream: not stubbed")
}

func outgoingAuth(ctx context.Context) []string {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return nil
	}

	return md.Get("authorization")
}

// 构造不校验 Config：错的配置在第一次调用时返回 ErrEmptyNamespace。
func TestNew_DoesNotValidateConfig(t *testing.T) {
	t.Parallel()

	client, err := New("localhost:7091", Config{})
	require.NoError(t, err, "New only dials, it does not validate config")

	_, err = client.Submit(context.Background(), "echo", nil)
	require.ErrorIs(t, err, ErrEmptyNamespace)
	require.NoError(t, client.Close())

	// 正常配置照常可用（gRPC 是懒连接，构造成功不等于连上了）。
	ok, err := New("localhost:7091", Config{Namespace: "demo", APIKey: "sk-demo"})
	require.NoError(t, err)
	require.NoError(t, ok.Close())
}

// NewWithConn 没有 error 位，配置问题由每个方法在调用前返回，不 panic。
func TestNewWithConn_ValidatesAtCallTime(t *testing.T) {
	t.Parallel()

	client := NewWithConn(&fakeConn{}, Config{Namespace: ""})
	ctx := context.Background()

	_, err := client.Submit(ctx, "echo", nil)
	require.ErrorIs(t, err, ErrEmptyNamespace)

	_, err = client.Get(ctx, "8b0e0f5a-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, ErrEmptyNamespace)

	require.ErrorIs(t, client.Cancel(ctx, "8b0e0f5a-0000-0000-0000-000000000000"), ErrEmptyNamespace)
}

// namespace 来自 Config，api_key 由连接包装注入，调用方都不用每次传。
func TestSubmit_BindsNamespaceAndInjectsKey(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{submit: &taskv1.SubmitResponse{
		Task: &schemav1.Task{Id: "task-1", Namespace: "demo", TaskType: "echo", State: schemav1.TaskState_INITED},
	}}
	client := NewWithConn(conn, Config{Namespace: "demo", APIKey: "sk-demo"})

	got, err := client.Submit(context.Background(), "echo", []byte(`{"a":1}`), WithMaxRetry(3))
	require.NoError(t, err)

	req, ok := conn.gotReq.(*taskv1.SubmitRequest)
	require.True(t, ok)
	assert.Equal(t, "demo", req.GetNamespace(), "namespace must come from Config")
	assert.Equal(t, "echo", req.GetTaskType())
	assert.Equal(t, int64(3), req.GetMaxRetry())
	assert.Equal(t, []string{"Bearer sk-demo"}, outgoingAuth(conn.gotCtx))

	require.NotNil(t, got)
	assert.Equal(t, "task-1", got.ID)
	assert.Equal(t, StateInited, got.State)
}

func TestGet_InjectsKeyOnEveryCall(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{get: &taskv1.GetResponse{
		Task: &schemav1.Task{Id: "task-1", Namespace: "demo"},
	}}
	client := NewWithConn(conn, Config{Namespace: "demo", APIKey: "sk-demo"})

	_, err := client.Get(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"/api.task.v1.TaskService/Get"}, conn.methods)
	assert.Equal(t, []string{"Bearer sk-demo"}, outgoingAuth(conn.gotCtx))
}

// 没配 key（服务端默认关闭鉴权）时不能发出任何凭据头。
func TestSubmit_WithoutKeySendsNoCredential(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{}
	client := NewWithConn(conn, Config{Namespace: "demo"})

	_, err := client.Submit(context.Background(), "echo", nil)
	require.NoError(t, err)
	assert.Empty(t, outgoingAuth(conn.gotCtx))
}

func TestTaskFromProto(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	got := taskFromProto(&schemav1.Task{
		Id:                "task-1",
		Namespace:         "demo",
		TaskType:          "echo",
		Payload:           []byte("in"),
		Result:            []byte("out"),
		Error:             []byte("boom"),
		State:             schemav1.TaskState_RUNNING,
		CreateTime:        timestamppb.New(created),
		NextRunTime:       created.Add(time.Minute).UnixMilli(),
		MaxRetry:          3,
		AttemptNo:         2,
		WorkerId:          42,
		LastHeartbeatTime: timestamppb.New(created),
	})

	require.NotNil(t, got)
	assert.Equal(t, "task-1", got.ID)
	assert.Equal(t, "demo", got.Namespace)
	assert.Equal(t, StateRunning, got.State)
	assert.Equal(t, "RUNNING", got.State.String())
	assert.Equal(t, []byte("in"), got.Payload)
	assert.Equal(t, []byte("out"), got.Result)
	assert.Equal(t, []byte("boom"), got.Error)
	assert.Equal(t, created, got.CreateTime)
	// 毫秒转出来的是本地时区的 time.Time，用 Equal 比时刻而不是比结构。
	assert.True(t, got.NextRunTime.Equal(created.Add(time.Minute)), "got %s", got.NextRunTime)
	assert.Equal(t, int64(3), got.MaxRetry)
	assert.Equal(t, int32(2), got.AttemptNo)
	assert.Equal(t, int64(42), got.WorkerID)
	assert.Equal(t, created, got.LastHeartbeatTime)

	// 零值毫秒表示「没有这个时刻」，不能变成 1970 年。
	assert.True(t, taskFromProto(&schemav1.Task{}).NextRunTime.IsZero())
	assert.Nil(t, taskFromProto(nil))
}

func TestStateFromProto_UnknownValueFallsBack(t *testing.T) {
	t.Parallel()

	assert.Equal(t, StateUnspecified, stateFromProto(schemav1.TaskState(99)))
	assert.Equal(t, "UNSPECIFIED", StateUnspecified.String())
}
