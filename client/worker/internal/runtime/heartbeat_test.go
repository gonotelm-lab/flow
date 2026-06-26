// flow/client/worker/internal/runtime/heartbeat_test.go
package runtime

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/client/worker/internal/runtime/testutil"
	"github.com/stretchr/testify/require"
)

func TestHeartbeatLoop_SendsHeartbeat(t *testing.T) {
	mock := &testutil.MockWorkerService{}
	conn, cleanup := startMockServer(t, mock)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hb := NewHeartbeatLoop(conn, 1, 20*time.Millisecond, slog.Default(),
		func() []string { return nil },
		func(ids []string) {},
	)
	go hb.Run(ctx)

	require.Eventually(t, func() bool {
		return mock.HeartbeatCountSnapshot() > 0
	}, time.Second, 10*time.Millisecond)

	cancel()
	time.Sleep(30 * time.Millisecond)
}
