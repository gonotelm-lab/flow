package schema

import "github.com/google/uuid"

// 审计动作。当前只覆盖 admin 管理接口的操作——客户端 gRPC 的
// Register / Update / Pause / Resume / Delete 不产生审计记录。
const (
	AuditActionCreateSchedule    = "CREATE_SCHEDULE"
	AuditActionUpdateSchedule    = "UPDATE_SCHEDULE"
	AuditActionPauseSchedule     = "PAUSE_SCHEDULE"
	AuditActionResumeSchedule    = "RESUME_SCHEDULE"
	AuditActionDeleteSchedule    = "DELETE_SCHEDULE"
	AuditActionTriggerSchedule   = "TRIGGER_SCHEDULE"
	AuditActionReplayDeadLetter  = "REPLAY_DEAD_LETTER"
	AuditActionDiscardDeadLetter = "DISCARD_DEAD_LETTER"
)

// 审计目标类型。表设计为通用，task / namespace / worker 后续接入无需改表。
const (
	AuditTargetSchedule = "SCHEDULE"
	// AuditTargetScheduleDeadLetter 用于死信的重投 / 放弃：操作对象是死信记录本身，
	// 不是 schedule，记成 SCHEDULE 会让「这条记录到底动了什么」变得含糊。
	AuditTargetScheduleDeadLetter = "SCHEDULE_DEAD_LETTER"
)

// 审计来源。
const (
	AuditSourceAdminHTTP = "ADMIN_HTTP"
)

// 审计结果。
const (
	AuditResultOk    = "OK"
	AuditResultError = "ERROR"
)

// AdminAuditLog 是 admin 管理接口操作的审计记录。
//
// 注意：当前服务端没有任何鉴权，因此 Operator 一律留空（保留字段，待用户体系接入后
// 填 user id），唯一非自填的痕迹是 SourceIp / UserAgent。这意味着现阶段它是
// 「协作记录」而不是「安全审计」，这一点必须在文档里写清楚。
type AdminAuditLog struct {
	Id         uuid.UUID `gorm:"column:id;primaryKey"`
	Action     string    `gorm:"column:action"`
	TargetType string    `gorm:"column:target_type"`
	TargetId   string    `gorm:"column:target_id"`
	Namespace  string    `gorm:"column:namespace"`
	// 保留字段，当前恒为空
	Operator  string `gorm:"column:operator"`
	SourceIp  string `gorm:"column:source_ip"`
	UserAgent string `gorm:"column:user_agent"`
	Source    string `gorm:"column:source"`
	// 变更前 / 删除前关键字段的 JSON 快照
	Snapshot     []byte `gorm:"column:snapshot"`
	Result       string `gorm:"column:result"`
	ErrorMessage string `gorm:"column:error_message"`
	CreateTime   int64  `gorm:"column:create_time"`
}

func (AdminAuditLog) TableName() string {
	return "admin_audit_logs"
}
