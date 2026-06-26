// flow/client/worker/internal/runtime/reporter_test.go
package runtime

import (
	"context"
	"log/slog"
	"net"
	"testing"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func startMockServer(t *testing.T, svc *testutil.MockWorkerService) (*grpc.ClientConn, func()) {
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

	return conn, func() {
		conn.Close()
		s.Stop()
	}
}

func TestReporter_ReportTask(t *testing.T) {
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := workerv1.NewWorkerServiceClient(conn)
	reporter := NewReporter(client, slog.Default())

	err := reporter.ReportTask(context.Background(), 1, &schemav1.Task{Id: "550e8400-e29b-41d4-a716-446655440000"}, workerv1.ReportAction_SUCCESS, []byte("done"))
	require.NoError(t, err)

	reports := mock.ReportsSnapshot()
	require.Len(t, reports, 1)
	require.Equal(t, workerv1.ReportAction_SUCCESS, reports[0].Action)
	require.Equal(t, []byte("done"), reports[0].Payload)
}
