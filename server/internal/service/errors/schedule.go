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
	// 因为「同一 schedule 同时最多一个 fire」，此时 TriggerNow 与死信重投都必须拒绝。
	ScheduleBusy = pkgerr.New(codes.FailedPrecondition, KeyScheduleBusy)

	// ScheduleEnded 表示 schedule 已经进入终态（STOPPED / FINISHED / FAILED），
	// 终态是墓碑：不能改定义、不能手动触发、不能再 stop，死信也不能重投。
	ScheduleEnded = pkgerr.New(codes.FailedPrecondition, KeyScheduleEnded)

	ScheduleDeadLetterNotFound = pkgerr.New(codes.NotFound, KeyScheduleDeadLetterNotFound)

	// ScheduleDeadLetterNotReplayable 表示死信还在慢重试中或已经重投过，不能重复重投。
	ScheduleDeadLetterNotReplayable = pkgerr.New(
		codes.FailedPrecondition, KeyScheduleDeadLetterNotReplayable)
)
