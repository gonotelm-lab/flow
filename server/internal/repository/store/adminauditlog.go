package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type AdminAuditLogListParams struct {
	TargetType string
	TargetId   string
	Action     string
	Namespace  string
	Offset     int
	Limit      int
}

// AdminAuditLog 是通用审计表。表结构对 target 无假设，当前只写 schedule 的
// admin 管理接口操作；task / namespace / worker 后续接入不需要改表。
//
// 写入是异步的（见 internal/audit），因此这里不参与业务事务。
type AdminAuditLog interface {
	Create(ctx context.Context, log *schema.AdminAuditLog) error
	CreateBatch(ctx context.Context, logs []*schema.AdminAuditLog) error
	List(ctx context.Context, params *AdminAuditLogListParams) ([]*schema.AdminAuditLog, int64, error)
}
