// flow/client/worker/worker_test.go
package worker

import (
	"context"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func startBufconnServer(t *testing.T, svc *testutil.MockWorkerService) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	testutil.Register(s, svc)
	go func() { _ = s.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	return conn, func() { conn.Close(); s.Stop() }
}

func TestClient_RunProcessesTask(t *testing.T) {
	taskID := "550e8400-e29b-41d4-a716-446655440000"
	mock := &testutil.MockWorkerService{
		PollResponses: [][]*schemav1.Task{
			{{Id: taskID, Payload: []byte(`{"name":"bob"}`)}},
		},
	}
	conn, cleanup := startBufconnServer(t, mock)
	defer cleanup()

	var handled atomic.Int32
	client := NewWithConn(conn, Config{
		Namespace: "ns",
		TaskType:  "render",
		Logger:    slog.Default(),
	})
	RegisterTyped[map[string]string, map[string]string](client, func(ctx context.Context, in map[string]string) (map[string]string, error) {
		handled.Add(1)
		return map[string]string{"hello": in["name"]}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- client.Run(ctx) }()

	require.Eventually(t, func() bool { return handled.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return len(mock.ReportsSnapshot()) == 1 }, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-errCh)
}

func TestClient_StartClose(t *testing.T) {
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startBufconnServer(t, mock)
	defer cleanup()

	client := NewWithConn(conn, Config{Namespace: "ns", TaskType: "render"})
	client.Handle(func(ctx context.Context, payload []byte) (Result, error) {
		return OkResult{Data: payload}, nil
	})

	require.NoError(t, client.Start())
	require.NoError(t, client.Close())
}
