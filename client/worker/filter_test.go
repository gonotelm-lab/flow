// flow/client/worker/filter_test.go
package worker

import (
	"testing"

	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"google.golang.org/grpc/stats"
)

func TestFrequentMethodFilter(t *testing.T) {
	f := frequentMethodFilter()

	cases := []struct {
		method string
		want   bool
	}{
		{workerv1.WorkerService_Poll_FullMethodName, false},
		{workerv1.WorkerService_Heartbeat_FullMethodName, false},
		{workerv1.WorkerService_Register_FullMethodName, true},
		{workerv1.WorkerService_Report_FullMethodName, true},
	}
	for _, c := range cases {
		if got := f(&stats.RPCTagInfo{FullMethodName: c.method}); got != c.want {
			t.Fatalf("filter(%s) = %v, want %v", c.method, got, c.want)
		}
	}
}
