package schema

import "github.com/google/uuid"

type Task struct {
	Id          uuid.UUID `gorm:"column:id;primaryKey"`
	Namespace   string    `gorm:"column:namespace"`
	TaskType    string    `gorm:"column:task_type"`
	State       string    `gorm:"column:state"`
	Payload     []byte    `gorm:"column:payload"`
	Result      []byte    `gorm:"column:result"`
	Error       []byte    `gorm:"column:error"`
	CreateTime  int64     `gorm:"column:create_time"`
	NextRunTime int64     `gorm:"column:next_run_time"`
	UpdateTime  int64     `gorm:"column:update_time"`
	WorkerId    int64     `gorm:"column:worker_id"`
	MaxRetry    int       `gorm:"column:max_retry"`
	AttemptNo          int   `gorm:"column:attempt_no"`
	LastHeartbeatTime  int64 `gorm:"column:last_heartbeat_time"`
}

func (Task) TableName() string {
	return "tasks"
}
