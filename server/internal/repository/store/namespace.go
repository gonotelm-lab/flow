package store

import (
	"context"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

type Namespace interface {
	Create(ctx context.Context, namespace *schema.Namespace) (*schema.Namespace, error)
	Get(ctx context.Context, name string) (*schema.Namespace, error)
	// GetByApiKey 按 api_key 反查 namespace，客户端面鉴权用它。
	GetByApiKey(ctx context.Context, apiKey string) (*schema.Namespace, error)
	List(ctx context.Context, offset, limit int) ([]*schema.Namespace, int64, error)
	Update(ctx context.Context, ns *schema.Namespace) error
}
