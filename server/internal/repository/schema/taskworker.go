package schema

type TaskWorker struct {
	Id            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name          string `gorm:"column:name"`
	Namespace     string `gorm:"column:namespace"`
	TaskType      string `gorm:"column:task_type"`
	CreateTime    int64  `gorm:"column:create_time"`
	HeartbeatTime int64  `gorm:"column:heartbeat_time"`
	LastWorkTime  int64  `gorm:"column:last_work_time"`
	TotalDealt    int64  `gorm:"column:total_dealt"`
	SuccessDealt  int64  `gorm:"column:success_dealt"`
}

func (TaskWorker) TableName() string {
	return "task_workers"
}
