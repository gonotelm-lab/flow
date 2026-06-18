package schema

type Instance struct {
	Id             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Group          string `gorm:"column:group"`
	Key            string `gorm:"column:key"`
	Value          string `gorm:"column:value"`
	StartTime      int64  `gorm:"column:start_time"`
	ExpireTime     int64  `gorm:"column:expire_time"`
	FencingToken   int64  `gorm:"column:fencing_token"`
	CreateRevision int64  `gorm:"column:create_revision"`
	Extras         []byte `gorm:"column:extras"`
}

func (Instance) TableName() string {
	return "instances"
}
