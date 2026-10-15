package postgres

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/impl/util"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/store"
	"github.com/gonotelm-lab/flow/server/pkg/sql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminAuditLogStoreImpl struct {
	db *gorm.DB
}

func NewAdminAuditLogStoreImpl(db *gorm.DB) store.AdminAuditLog {
	return &AdminAuditLogStoreImpl{db: db}
}

func normalizeAuditLog(log *schema.AdminAuditLog) {
	// 兜底生成主键。
	//
	// admin_audit_logs.id 虽然写了 UUID DEFAULT uuidv7()，但 GORM 会把 Go 的零值 UUID
	// **显式**放进 INSERT，数据库默认值根本不会生效：第一条以全零主键成功，之后每条都撞
	// 主键。审计是异步写的，这种失败只会安静地出现在日志里，接口照样 200 —— 必须在这里兜住。
	if log.Id == uuid.Nil {
		if id, err := uuid.NewV7(); err == nil {
			log.Id = id
		} else {
			log.Id = uuid.New()
		}
	}

	log.Action = truncate(log.Action, 64)
	log.TargetType = truncate(log.TargetType, 32)
	log.TargetId = truncate(log.TargetId, 128)
	log.Namespace = truncate(log.Namespace, 128)
	log.Operator = truncate(log.Operator, 128)
	log.SourceIp = truncate(log.SourceIp, 64)
	log.UserAgent = truncate(log.UserAgent, 512)
	log.Source = truncate(log.Source, 32)
	log.Result = truncate(log.Result, 16)
	log.ErrorMessage = truncate(log.ErrorMessage, 1024)
}

func (s *AdminAuditLogStoreImpl) Create(ctx context.Context, log *schema.AdminAuditLog) error {
	db := util.GetDB(ctx, s.db)
	normalizeAuditLog(log)
	if err := db.Create(log).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}

func (s *AdminAuditLogStoreImpl) CreateBatch(
	ctx context.Context,
	logs []*schema.AdminAuditLog,
) error {
	if len(logs) == 0 {
		return nil
	}
	for _, log := range logs {
		normalizeAuditLog(log)
	}
	db := util.GetDB(ctx, s.db)
	if err := db.CreateInBatches(logs, 100).Error; err != nil {
		return sql.WrapError(err)
	}
	return nil
}

func (s *AdminAuditLogStoreImpl) List(
	ctx context.Context,
	params *store.AdminAuditLogListParams,
) ([]*schema.AdminAuditLog, int64, error) {
	db := util.GetDB(ctx, s.db)
	q := db.Model(&schema.AdminAuditLog{})

	if params.TargetType != "" {
		q = q.Where("target_type = ?", params.TargetType)
	}
	if params.TargetId != "" {
		q = q.Where("target_id = ?", params.TargetId)
	}
	if params.Action != "" {
		q = q.Where("action = ?", params.Action)
	}
	if params.Namespace != "" {
		q = q.Where("namespace = ?", params.Namespace)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	var list []*schema.AdminAuditLog
	if err := q.Offset(params.Offset).Limit(params.Limit).
		Order("create_time DESC").
		Find(&list).Error; err != nil {
		return nil, 0, sql.WrapError(err)
	}

	return list, total, nil
}
