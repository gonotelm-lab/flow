package schema

import "github.com/google/uuid"

type TaskEvent struct {
	Id         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TaskId     uuid.UUID `gorm:"column:task_id"`
	EventType  string    `gorm:"column:event_type"`
	CreateTime int64     `gorm:"column:create_time"`
	Payload    []byte    `gorm:"column:payload"`
}

func (TaskEvent) TableName() string {
	return "task_events"
}
