package task

import (
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// State 是任务的生命周期状态。
type State int

const (
	StateUnspecified State = iota
	// StateInited 已入库、等待 worker 认领。
	StateInited
	// StateRunning 已被某个 worker 认领。
	StateRunning
	StateDone
	StateFailed
	StateCancelled
)

func (s State) String() string {
	switch s {
	case StateInited:
		return "INITED"
	case StateRunning:
		return "RUNNING"
	case StateDone:
		return "DONE"
	case StateFailed:
		return "FAILED"
	case StateCancelled:
		return "CANCELLED"
	default:
		return "UNSPECIFIED"
	}
}

// Task 是服务端返回的任务对象：时间用 time.Time（零值表示「没有这个时刻」），
// Payload / Result / Error 是原样字节。
type Task struct {
	ID        string
	Namespace string
	TaskType  string

	Payload []byte
	Result  []byte
	Error   []byte

	State State

	CreateTime        time.Time
	UpdateTime        time.Time
	NextRunTime       time.Time
	LastHeartbeatTime time.Time

	MaxRetry  int64
	AttemptNo int32
	WorkerID  int64
}

func taskFromProto(p *schemav1.Task) *Task {
	if p == nil {
		return nil
	}

	return &Task{
		ID:                p.GetId(),
		Namespace:         p.GetNamespace(),
		TaskType:          p.GetTaskType(),
		Payload:           p.GetPayload(),
		Result:            p.GetResult(),
		Error:             p.GetError(),
		State:             stateFromProto(p.GetState()),
		CreateTime:        protoTime(p.GetCreateTime()),
		UpdateTime:        protoTime(p.GetUpdateTime()),
		NextRunTime:       millisToTime(p.GetNextRunTime()),
		LastHeartbeatTime: protoTime(p.GetLastHeartbeatTime()),
		MaxRetry:          p.GetMaxRetry(),
		AttemptNo:         p.GetAttemptNo(),
		WorkerID:          p.GetWorkerId(),
	}
}

// stateFromProto 写成显式 switch，不做编号强转：proto 那边加值或调号时，这里会落到
// UNSPECIFIED，不会跟着一起错。
func stateFromProto(s schemav1.TaskState) State {
	switch s {
	case schemav1.TaskState_INITED:
		return StateInited
	case schemav1.TaskState_RUNNING:
		return StateRunning
	case schemav1.TaskState_DONE:
		return StateDone
	case schemav1.TaskState_FAILED:
		return StateFailed
	case schemav1.TaskState_CANCELLED:
		return StateCancelled
	default:
		return StateUnspecified
	}
}

func millisToTime(millis int64) time.Time {
	if millis <= 0 {
		return time.Time{}
	}

	return time.UnixMilli(millis)
}

func protoTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil || !ts.IsValid() || ts.AsTime().IsZero() {
		return time.Time{}
	}

	return ts.AsTime()
}
