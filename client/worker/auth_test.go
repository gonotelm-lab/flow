package worker

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// authRecordingServer 记录 Register 收到的 metadata，用来断言凭据有没有真的送出去。
type authRecordingServer struct {
	workerv1.UnimplementedWorkerServiceServer

	mu       sync.Mutex
	register metadata.MD
}

func (s *authRecordingServer) Register(
	ctx context.Context, w *schemav1.Worker,
) (*schemav1.Worker, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	s.mu.Lock()
	s.register = md
	s.mu.Unlock()

	return &schemav1.Worker{Id: 1, Namespace: w.GetNamespace(), TaskType: w.GetTaskType()}, nil
}

func (s *authRecordingServer) Unregister(
	context.Context, *workerv1.UnregisterRequest,
) (*workerv1.UnregisterResponse, error) {
	return &workerv1.UnregisterResponse{}, nil
}

func (s *authRecordingServer) Heartbeat(
	context.Context, *workerv1.HeartbeatRequest,
) (*workerv1.HeartbeatResponse, error) {
	return &workerv1.HeartbeatResponse{}, nil
}

// Poll 模拟长轮询：阻塞到 ctx 结束，避免测试期间空转。
func (s *authRecordingServer) Poll(
	ctx context.Context, _ *workerv1.PollRequest,
) (*workerv1.PollResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (s *authRecordingServer) incomingAuth(t *testing.T) []string {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.register == nil {
		return nil
	}

	return s.register.Get("authorization")
}

func startAuthRecorder(t *testing.T) (*grpc.ClientConn, *authRecordingServer, func()) {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	recorder := &authRecordingServer{}
	workerv1.RegisterWorkerServiceServer(srv, recorder)
	go func() { _ = srv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	return conn, recorder, func() {
		_ = conn.Close()
		srv.Stop()
	}
}

// 构造不校验 Config：namespace 的必填校验发生在 Start（第一个会发 RPC 的入口）。
func TestStart_RequiresNamespace(t *testing.T) {
	t.Parallel()

	client, err := New("localhost:7091", Config{TaskType: "echo"})
	require.NoError(t, err, "New only dials, it does not validate config")

	require.ErrorIs(t, client.Start(), ErrEmptyNamespace)
	require.NoError(t, client.Close())

	blank, err := New("localhost:7091", Config{Namespace: "   ", TaskType: "echo"})
	require.NoError(t, err)
	require.ErrorIs(t, blank.Start(), ErrEmptyNamespace)
	require.NoError(t, blank.Close())
}

// 注册、轮询、心跳、上报、注销都走同一个包装过的连接，看注册这一处就够。
func TestClient_InjectsAPIKeyOnRegister(t *testing.T) {
	t.Parallel()

	conn, recorder, cleanup := startAuthRecorder(t)
	defer cleanup()

	client := NewWithConn(conn, Config{
		Namespace:         "demo",
		TaskType:          "echo",
		APIKey:            "sk-demo",
		HeartbeatInterval: time.Hour,
	})
	client.Handle(func(context.Context, []byte) (Result, error) { return OkResult{}, nil })

	require.NoError(t, client.Start())
	require.NoError(t, client.Close())

	assert.Equal(t, []string{"Bearer sk-demo"}, recorder.incomingAuth(t))
}

// 没配 key（服务端默认关闭鉴权）时不能发出任何凭据头。
func TestClient_WithoutKeySendsNoCredential(t *testing.T) {
	t.Parallel()

	conn, recorder, cleanup := startAuthRecorder(t)
	defer cleanup()

	client := NewWithConn(conn, Config{
		Namespace:         "demo",
		TaskType:          "echo",
		HeartbeatInterval: time.Hour,
	})
	client.Handle(func(context.Context, []byte) (Result, error) { return OkResult{}, nil })

	require.NoError(t, client.Start())
	require.NoError(t, client.Close())

	assert.Empty(t, recorder.incomingAuth(t))
}
