package errors

import (
	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	KeyScheduleNotFound                pkgerr.DomainErrorKey = "SCHEDULE_NOT_FOUND"
	KeyScheduleBusy                    pkgerr.DomainErrorKey = "SCHEDULE_BUSY"
	KeyScheduleEnded                   pkgerr.DomainErrorKey = "SCHEDULE_ENDED"
	KeyScheduleDeadLetterNotFound      pkgerr.DomainErrorKey = "SCHEDULE_DEAD_LETTER_NOT_FOUND"
	KeyScheduleDeadLetterNotReplayable pkgerr.DomainErrorKey = "SCHEDULE_DEAD_LETTER_NOT_REPLAYABLE"
)

var (
	ScheduleNotFound = pkgerr.New(codes.NotFound, KeyScheduleNotFound)

	// ScheduleBusy 表示该 schedule 当前有未完成的 fire（含死信慢重试阶段）。
	// 因为「同一 schedule 同时最多一个 fire」，此时 TriggerNow 与死信 Replay 都必须拒绝。
	ScheduleBusy = pkgerr.New(codes.FailedPrecondition, KeyScheduleBusy)

	// ScheduleEnded 表示一次性任务已进入 FINISHED / FAILED 终态，无法再 Pause / Resume。
	ScheduleEnded = pkgerr.New(codes.FailedPrecondition, KeyScheduleEnded)

	ScheduleDeadLetterNotFound = pkgerr.New(codes.NotFound, KeyScheduleDeadLetterNotFound)

	// ScheduleDeadLetterNotReplayable 表示死信还在慢重试中或已经处理过，不能重复 Replay / Discard。
	ScheduleDeadLetterNotReplayable = pkgerr.New(
		codes.FailedPrecondition, KeyScheduleDeadLetterNotReplayable)
)
