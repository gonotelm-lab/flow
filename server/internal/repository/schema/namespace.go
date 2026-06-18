package schema

type Namespace struct {
	Id          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string `gorm:"column:name"`
	Description string `gorm:"column:description"`
	ApiKey      string `gorm:"column:api_key"`
	Creator     string `gorm:"column:creator"`
	CreateTime  int64  `gorm:"column:create_time"`
	UpdateTime  int64  `gorm:"column:update_time"`
}

func (Namespace) TableName() string {
	return "namespaces"
}
