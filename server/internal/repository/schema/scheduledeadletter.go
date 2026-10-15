package schema

import "github.com/google/uuid"

// ScheduleDeadLetter 是一次「重试全部失败」的 fire。
//
// 状态机：
//
//	RETRYING  慢重试进行中，阻塞该 schedule 的新触发
//	EXHAUSTED 慢重试耗尽，等待人工 Replay / Discard，不再阻塞新触发
//	REPLAYED  人工重投成功发起（复用原 FireId，接收方可去重）
//	DISCARDED 人工放弃
//
// 同一 schedule 在任一时刻最多一条 RETRYING（部分唯一索引约束），但 EXHAUSTED 可以
// 随时间累积多条。
type ScheduleDeadLetter struct {
	Id            uuid.UUID `gorm:"column:id;primaryKey"`
	ScheduleId    uuid.UUID `gorm:"column:schedule_id"`
	Namespace     string    `gorm:"column:namespace"`
	FireId        uuid.UUID `gorm:"column:fire_id"`
	ScheduledTime int64     `gorm:"column:scheduled_time"`
	AttemptCount  int       `gorm:"column:attempt_count"`
	// DeadLetterState* 之一
	State          string `gorm:"column:state"`
	NextAttemptAt  int64  `gorm:"column:next_attempt_at"`
	LastHttpStatus int    `gorm:"column:last_http_status"`
	LastError      string `gorm:"column:last_error"`
	// 各次尝试的 JSON 摘要
	Attempts   []byte `gorm:"column:attempts"`
	CreateTime int64  `gorm:"column:create_time"`
	UpdateTime int64  `gorm:"column:update_time"`
}

func (ScheduleDeadLetter) TableName() string {
	return "schedule_dead_letters"
}
