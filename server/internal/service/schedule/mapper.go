package schedule

import (
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 数据库里存的是短字符串（ACTIVE / PAUSED / ...），proto 枚举值带枚举名前缀
// （proto3 的枚举值在包级可见，不带前缀会和同包的 TaskState 撞名）。两边都需要显式映射。

var scheduleTypeToProto = map[string]schemav1.ScheduleType{
	schema.ScheduleTypeOneShot: schemav1.ScheduleType_SCHEDULE_TYPE_ONE_SHOT,
	schema.ScheduleTypeCron:    schemav1.ScheduleType_SCHEDULE_TYPE_CRON,
}

var scheduleTypeFromProto = map[schemav1.ScheduleType]string{
	schemav1.ScheduleType_SCHEDULE_TYPE_ONE_SHOT: schema.ScheduleTypeOneShot,
	schemav1.ScheduleType_SCHEDULE_TYPE_CRON:     schema.ScheduleTypeCron,
}

var scheduleStateToProto = map[string]schemav1.ScheduleState{
	schema.ScheduleStateActive:   schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE,
	schema.ScheduleStatePaused:   schemav1.ScheduleState_SCHEDULE_STATE_PAUSED,
	schema.ScheduleStateFinished: schemav1.ScheduleState_SCHEDULE_STATE_FINISHED,
	schema.ScheduleStateFailed:   schemav1.ScheduleState_SCHEDULE_STATE_FAILED,
}

var scheduleStateFromProto = map[schemav1.ScheduleState]string{
	schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE:   schema.ScheduleStateActive,
	schemav1.ScheduleState_SCHEDULE_STATE_PAUSED:   schema.ScheduleStatePaused,
	schemav1.ScheduleState_SCHEDULE_STATE_FINISHED: schema.ScheduleStateFinished,
	schemav1.ScheduleState_SCHEDULE_STATE_FAILED:   schema.ScheduleStateFailed,
}

var fireStateToProto = map[string]schemav1.FireState{
	schema.FireStatePending:    schemav1.FireState_FIRE_STATE_PENDING,
	schema.FireStateDelivering: schemav1.FireState_FIRE_STATE_DELIVERING,
	schema.FireStateDeadLetter: schemav1.FireState_FIRE_STATE_DEAD_LETTER,
}

var deadLetterStateToProto = map[string]schemav1.DeadLetterState{
	schema.DeadLetterStateRetrying:  schemav1.DeadLetterState_DEAD_LETTER_STATE_RETRYING,
	schema.DeadLetterStateExhausted: schemav1.DeadLetterState_DEAD_LETTER_STATE_EXHAUSTED,
	schema.DeadLetterStateReplayed:  schemav1.DeadLetterState_DEAD_LETTER_STATE_REPLAYED,
	schema.DeadLetterStateDiscarded: schemav1.DeadLetterState_DEAD_LETTER_STATE_DISCARDED,
}

var deadLetterStateFromProto = map[schemav1.DeadLetterState]string{
	schemav1.DeadLetterState_DEAD_LETTER_STATE_RETRYING:  schema.DeadLetterStateRetrying,
	schemav1.DeadLetterState_DEAD_LETTER_STATE_EXHAUSTED: schema.DeadLetterStateExhausted,
	schemav1.DeadLetterState_DEAD_LETTER_STATE_REPLAYED:  schema.DeadLetterStateReplayed,
	schemav1.DeadLetterState_DEAD_LETTER_STATE_DISCARDED: schema.DeadLetterStateDiscarded,
}

func ts(millis int64) *timestamppb.Timestamp {
	if millis <= 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(millis))
}

// ToProtoSchedule 把存储模型转成对外的 proto。
//
// fire_state 为空（槽位空闲）时输出 FIRE_STATE_UNSPECIFIED——按枚举定义，UNSPECIFIED
// 就表示「当前没有未完成的 fire」。
func ToProtoSchedule(s *schema.Schedule) *schemav1.Schedule {
	if s == nil {
		return nil
	}

	out := &schemav1.Schedule{
		Id:                  s.Id.String(),
		Namespace:           s.Namespace,
		Name:                s.Name,
		CallbackUrl:         s.CallbackUrl,
		ContentType:         s.ContentType,
		Payload:             s.Payload,
		IdempotencyKey:      IdempotencyKeyOf(s),
		State:               scheduleStateToProto[s.State],
		NextFireAt:          s.NextFireAt,
		AttemptNo:           int32(s.AttemptNo),
		LastSuccessAt:       s.LastSuccessAt,
		ConsecutiveFailures: int32(s.ConsecutiveFailures),
		TotalFailures:       s.TotalFailures,
		SkippedCount:        s.SkippedCount,
		LastError:           s.LastError,
		CreateTime:          ts(s.CreateTime),
		UpdateTime:          ts(s.UpdateTime),
	}

	// 触发类型不在 Schedule 上单独占字段，而是由 oneof 分支本身表达——这样
	// 「一次性却填了 cron」这种状态在类型层面就不可表达。库里仍存 schedule_type，
	// 那是为了筛选与建索引。
	switch s.ScheduleType {
	case schema.ScheduleTypeOneShot:
		out.Trigger = &schemav1.Schedule_OneShot{
			OneShot: &schemav1.OneShotTrigger{FireAt: s.FireAt},
		}
	case schema.ScheduleTypeCron:
		out.Trigger = &schemav1.Schedule_Cron{
			Cron: &schemav1.CronTrigger{Expr: s.CronExpr, Timezone: s.Timezone},
		}
	}

	if s.FireState != nil {
		out.FireState = fireStateToProto[*s.FireState]
	}

	return out
}

// FromProtoSchedule 把请求里的 proto 拆成存储模型。
//
// 只做形状转换，不做业务校验——校验与归一化统一由 Core.applyTrigger 负责，避免
// 两处校验逻辑漂移。运行态字段（state / next_fire_at / fire 槽位 / 统计）一律忽略：
// 它们由调度器维护，不接受调用方写入。
func FromProtoSchedule(p *schemav1.Schedule) *schema.Schedule {
	if p == nil {
		return nil
	}

	out := &schema.Schedule{
		Namespace:   p.GetNamespace(),
		Name:        p.GetName(),
		CallbackUrl: p.GetCallbackUrl(),
		ContentType: p.GetContentType(),
		Payload:     p.GetPayload(),
	}
	key := p.GetIdempotencyKey()
	out.IdempotencyKey = normalizeIdempotencyKey(&key)

	// 触发类型从 oneof 分支推导：库里要存 schedule_type 才能筛选与建索引，
	// 但对外契约上类型就是「哪个分支被设了」。
	switch trigger := p.GetTrigger().(type) {
	case *schemav1.Schedule_OneShot:
		out.ScheduleType = schema.ScheduleTypeOneShot
		out.FireAt = trigger.OneShot.GetFireAt()
	case *schemav1.Schedule_Cron:
		out.ScheduleType = schema.ScheduleTypeCron
		out.CronExpr = trigger.Cron.GetExpr()
		out.Timezone = trigger.Cron.GetTimezone()
	}

	return out
}

// 供运维面（service/admin）复用，避免两边各写一份枚举映射导致漂移。
func ScheduleTypeFromProto(t schemav1.ScheduleType) string {
	return scheduleTypeFromProto[t]
}

func ScheduleStateFromProto(s schemav1.ScheduleState) string {
	return scheduleStateFromProto[s]
}

func DeadLetterStateToProto(state string) schemav1.DeadLetterState {
	return deadLetterStateToProto[state]
}

func DeadLetterStateFromProto(s schemav1.DeadLetterState) string {
	return deadLetterStateFromProto[s]
}
