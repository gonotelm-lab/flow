// flow/client/worker/internal/runtime/testutil/mock_server.go
package testutil

import (
	"context"
	"sync"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MockWorkerService struct {
	workerv1.UnimplementedWorkerServiceServer

	mu sync.Mutex

	WorkerID      int64
	Reports       []workerv1.ReportRequest
	PollTasks     []*schemav1.Task
	PollResponses [][]*schemav1.Task // 每次 Poll 返回一批，按调用顺序消费
	pollCall      int
}

func Register(s *grpc.Server, svc *MockWorkerService) {
	workerv1.RegisterWorkerServiceServer(s, svc)
}

func (m *MockWorkerService) Register(ctx context.Context, req *schemav1.Worker) (*schemav1.Worker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WorkerID++
	req.Id = m.WorkerID
	now := timestamppb.Now()
	req.CreateTime = now
	req.HeartbeatTime = now
	return req, nil
}

func (m *MockWorkerService) Unregister(ctx context.Context, req *workerv1.UnregisterRequest) (*workerv1.UnregisterResponse, error) {
	return &workerv1.UnregisterResponse{}, nil
}

func (m *MockWorkerService) Heartbeat(ctx context.Context, req *workerv1.HeartbeatRequest) (*workerv1.HeartbeatResponse, error) {
	return &workerv1.HeartbeatResponse{HeartbeatTime: timestamppb.Now()}, nil
}

func (m *MockWorkerService) Poll(ctx context.Context, req *workerv1.PollRequest) (*workerv1.PollResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pollCall < len(m.PollResponses) {
		tasks := m.PollResponses[m.pollCall]
		m.pollCall++
		if len(tasks) > 0 {
			return &workerv1.PollResponse{Task: tasks[0]}, nil
		}
	}
	return &workerv1.PollResponse{}, nil
}

func (m *MockWorkerService) Report(ctx context.Context, req *workerv1.ReportRequest) (*workerv1.ReportResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reports = append(m.Reports, *req)
	return &workerv1.ReportResponse{}, nil
}

func (m *MockWorkerService) ReportsSnapshot() []workerv1.ReportRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]workerv1.ReportRequest, len(m.Reports))
	copy(out, m.Reports)
	return out
}
