package schema

type GlobalRevision struct {
	Name            string `gorm:"column:name;primaryKey"`
	CurrentRevision int64  `gorm:"column:current_revision"`
	UpdateTime      int64  `gorm:"column:update_time"`
}

func (GlobalRevision) TableName() string {
	return "global_revisions"
}
