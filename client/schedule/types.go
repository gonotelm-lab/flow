package schedule

import (
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Trigger 是触发方式，只有 OneShotTrigger 与 CronTrigger 两个实现。
// 用接口而不是「类型字段 + 两个平铺字段」，是为了让「一次性却填了 cron」无法表达。
type Trigger interface {
	trigger()
}

// OneShotTrigger 在某个绝对时刻触发一次。
type OneShotTrigger struct {
	FireAt time.Time
}

// CronTrigger 按标准 5 字段表达式重复触发。
//
// 方言是 robfig/cron/v3：星期 0-6 且 0 是周日，月份 1-12，不支持 L / W / # 与 @ 描述符，
// 与 Quartz 在字段数、星期编号、月份编号上都不同。
type CronTrigger struct {
	Expr     string
	Timezone string
}

func (OneShotTrigger) trigger() {}
func (CronTrigger) trigger()    {}

// Spec 是注册时提交的定义。
//
// 运行态字段（id / state / next_fire_at / 统计 / 时间戳）由服务端维护，不在这个类型里，
// 调用方也无从提交；namespace 属于客户端本身（Config.Namespace），也不在定义里。
// 用 OneShot / After / Cron 构造，或直接填结构体。
type Spec struct {
	Name           string
	Trigger        Trigger
	CallbackURL    string
	ContentType    string
	Payload        []byte
	IdempotencyKey string
}

// State 是 schedule 的生命周期状态。
type State int

const (
	StateUnspecified State = iota
	StateActive
	// StateStopped 是人为单向停止的终态：不会再触发，也没有恢复入口。
	StateStopped
	StateFinished
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateActive:
		return "ACTIVE"
	case StateStopped:
		return "STOPPED"
	case StateFinished:
		return "FINISHED"
	case StateFailed:
		return "FAILED"
	default:
		return "UNSPECIFIED"
	}
}

// Type 是触发类型，只在 List 里当筛选用——Spec 自身用 Trigger 表达类型。
type Type int

const (
	TypeUnspecified Type = iota
	TypeOneShot
	TypeCron
)

func (t Type) String() string {
	switch t {
	case TypeOneShot:
		return "ONE_SHOT"
	case TypeCron:
		return "CRON"
	default:
		return "UNSPECIFIED"
	}
}

// FireState 是当前 fire 槽位状态。FireStateUnspecified 表示槽位空闲。
type FireState int

const (
	FireStateUnspecified FireState = iota
	FireStatePending
	FireStateDelivering
	FireStateDeadLetter
)

func (f FireState) String() string {
	switch f {
	case FireStatePending:
		return "PENDING"
	case FireStateDelivering:
		return "DELIVERING"
	case FireStateDeadLetter:
		return "DEAD_LETTER"
	default:
		return "UNSPECIFIED"
	}
}

// Schedule 是服务端返回的完整对象：定义（内嵌 Spec）+ namespace + 运行态。
//
// 时间字段用 time.Time，零值表示「没有这个时刻」，服务端的 unix 毫秒不在 SDK 表面出现。
// 定义改不了（没有 Update），要改只能重新注册一条。
type Schedule struct {
	Spec

	ID                  string
	Namespace           string
	State               State
	NextFireAt          time.Time
	FireState           FireState
	AttemptNo           int32
	LastSuccessAt       time.Time
	ConsecutiveFailures int32
	TotalFailures       int64
	SkippedCount        int64
	LastError           string
	CreateTime          time.Time
	UpdateTime          time.Time
}

// scheduleFromProto 把服务端返回的 proto 转成 SDK 类型。
func scheduleFromProto(p *schemav1.Schedule) *Schedule {
	if p == nil {
		return nil
	}

	out := &Schedule{
		Spec: Spec{
			Name:           p.GetName(),
			CallbackURL:    p.GetCallbackUrl(),
			ContentType:    p.GetContentType(),
			Payload:        p.GetPayload(),
			IdempotencyKey: p.GetIdempotencyKey(),
		},
		ID:                  p.GetId(),
		Namespace:           p.GetNamespace(),
		State:               stateFromProto(p.GetState()),
		NextFireAt:          millisToTime(p.GetNextFireAt()),
		FireState:           fireStateFromProto(p.GetFireState()),
		AttemptNo:           p.GetAttemptNo(),
		LastSuccessAt:       millisToTime(p.GetLastSuccessAt()),
		ConsecutiveFailures: p.GetConsecutiveFailures(),
		TotalFailures:       p.GetTotalFailures(),
		SkippedCount:        p.GetSkippedCount(),
		LastError:           p.GetLastError(),
		CreateTime:          protoTime(p.GetCreateTime()),
		UpdateTime:          protoTime(p.GetUpdateTime()),
	}

	switch t := p.GetTrigger().(type) {
	case *schemav1.Schedule_OneShot:
		out.Trigger = OneShotTrigger{FireAt: millisToTime(t.OneShot.GetFireAt())}
	case *schemav1.Schedule_Cron:
		out.Trigger = CronTrigger{Expr: t.Cron.GetExpr(), Timezone: t.Cron.GetTimezone()}
	}

	return out
}

// specToProto 只带定义字段；namespace 来自客户端配置，运行态由服务端维护。
func specToProto(s Spec, namespace string) *schemav1.Schedule {
	out := &schemav1.Schedule{
		Namespace:      namespace,
		Name:           s.Name,
		CallbackUrl:    s.CallbackURL,
		ContentType:    s.ContentType,
		Payload:        s.Payload,
		IdempotencyKey: s.IdempotencyKey,
	}

	switch t := s.Trigger.(type) {
	case OneShotTrigger:
		out.Trigger = &schemav1.Schedule_OneShot{
			OneShot: &schemav1.OneShotTrigger{FireAt: t.FireAt.UnixMilli()},
		}
	case CronTrigger:
		out.Trigger = &schemav1.Schedule_Cron{
			Cron: &schemav1.CronTrigger{Expr: t.Expr, Timezone: t.Timezone},
		}
	}

	return out
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

// 枚举转换写成显式 switch，不做编号强转：proto 那边加值或调号时，这里会安静地
// 落到 UNSPECIFIED，而不会被类型转换蒙混过去。

func stateFromProto(s schemav1.ScheduleState) State {
	switch s {
	case schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE:
		return StateActive
	case schemav1.ScheduleState_SCHEDULE_STATE_STOPPED:
		return StateStopped
	case schemav1.ScheduleState_SCHEDULE_STATE_FINISHED:
		return StateFinished
	case schemav1.ScheduleState_SCHEDULE_STATE_FAILED:
		return StateFailed
	default:
		return StateUnspecified
	}
}

func stateToProto(s State) schemav1.ScheduleState {
	switch s {
	case StateActive:
		return schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE
	case StateStopped:
		return schemav1.ScheduleState_SCHEDULE_STATE_STOPPED
	case StateFinished:
		return schemav1.ScheduleState_SCHEDULE_STATE_FINISHED
	case StateFailed:
		return schemav1.ScheduleState_SCHEDULE_STATE_FAILED
	default:
		return schemav1.ScheduleState_SCHEDULE_STATE_UNSPECIFIED
	}
}

func typeToProto(t Type) schemav1.ScheduleType {
	switch t {
	case TypeOneShot:
		return schemav1.ScheduleType_SCHEDULE_TYPE_ONE_SHOT
	case TypeCron:
		return schemav1.ScheduleType_SCHEDULE_TYPE_CRON
	default:
		return schemav1.ScheduleType_SCHEDULE_TYPE_UNSPECIFIED
	}
}

func fireStateFromProto(f schemav1.FireState) FireState {
	switch f {
	case schemav1.FireState_FIRE_STATE_PENDING:
		return FireStatePending
	case schemav1.FireState_FIRE_STATE_DELIVERING:
		return FireStateDelivering
	case schemav1.FireState_FIRE_STATE_DEAD_LETTER:
		return FireStateDeadLetter
	default:
		return FireStateUnspecified
	}
}
