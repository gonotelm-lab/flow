// flow/client/worker/internal/runtime/runtime_test.go
package runtime

import (
	"context"
	"log/slog"
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
)

func TestRuntime_StartStop(t *testing.T) {
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	defer cleanup()

	rt := New(RuntimeConfig{
		Conn:              conn,
		Namespace:         "ns",
		TaskType:          "render",
		Name:              "w1",
		MaxConcurrency:    1,
		HeartbeatInterval: 20 * time.Millisecond,
		Handler: func(ctx context.Context, task *schemav1.Task) (workerv1.ReportAction, []byte) {
			return workerv1.ReportAction_SUCCESS, []byte("ok")
		},
		Logger: slog.Default(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, rt.Start(ctx))
	require.Greater(t, rt.WorkerID(), int64(0))

	cancel()
	require.NoError(t, rt.Stop(context.Background()))
}