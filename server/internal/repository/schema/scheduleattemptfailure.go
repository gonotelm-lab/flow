package schema

import "github.com/google/uuid"

// ScheduleAttemptFailure 记录每一次「失败的」投递尝试。成功不落记录，只更新
// schedules 行上的 last_success_at 与聚合计数。
type ScheduleAttemptFailure struct {
	Id         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ScheduleId uuid.UUID `gorm:"column:schedule_id"`
	Namespace  string    `gorm:"column:namespace"`
	FireId     uuid.UUID `gorm:"column:fire_id"`
	AttemptNo  int       `gorm:"column:attempt_no"`
	// AttemptPhaseFast 或 AttemptPhaseDeadLetter
	Phase         string `gorm:"column:phase"`
	ScheduledTime int64  `gorm:"column:scheduled_time"`
	AttemptTime   int64  `gorm:"column:attempt_time"`
	// 0 表示无响应（超时 / 连接失败 / TLS 失败）
	HttpStatus int    `gorm:"column:http_status"`
	DurationMs int64  `gorm:"column:duration_ms"`
	Error      string `gorm:"column:error"`
	CreateTime int64  `gorm:"column:create_time"`
}

func (ScheduleAttemptFailure) TableName() string {
	return "schedule_attempt_failures"
}
