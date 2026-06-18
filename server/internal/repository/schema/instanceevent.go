package schema

type InstanceEvent struct {
	Revision   int64  `gorm:"column:revision;primaryKey"`
	Group      string `gorm:"column:group"`
	Key        string `gorm:"column:key"`
	Value      string `gorm:"column:value"`
	Type       string `gorm:"column:type"`
	CreateTime int64  `gorm:"column:create_time"`
}

func (InstanceEvent) TableName() string {
	return "instance_events"
}
