package admin

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	apischema "github.com/gonotelm-lab/flow/api/schema/v1"
	reposchema "github.com/gonotelm-lab/flow/server/internal/repository/schema"
	srverr "github.com/gonotelm-lab/flow/server/internal/service/error"
	"github.com/gonotelm-lab/flow/server/pkg/sql"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) createNamespace(
	ctx context.Context,
	namespace *apischema.Namespace,
) (*apischema.Namespace, error) {
	now := time.Now()
	ns, err := s.store.Namespace.Create(ctx,
		&reposchema.Namespace{
			Name:        namespace.GetName(),
			Description: namespace.GetDescription(),
			ApiKey:      generateApiKey(),
			Creator:     namespace.GetCreator(),
			CreateTime:  now.UnixMilli(),
			UpdateTime:  now.UnixMilli(),
		})
	if err != nil {
		if errors.Is(err, sql.ErrDuplicatedKey) {
			return nil, srverr.NamespaceExists
		}

		return nil, errors.WithMessage(err, "failed to create namespace")
	}

	return toApiNamespace(ns), nil
}

func (s *Service) getNamespace(
	ctx context.Context,
	name string,
) (*apischema.Namespace, error) {
	ns, err := s.store.Namespace.Get(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRecord) {
			return nil, srverr.NamespaceNotFound
		}

		return nil, errors.WithMessage(err, "failed to get namespace")
	}

	apiNs := toApiNamespace(ns)
	apiNs.ApiKey = ""

	return apiNs, nil
}

func toApiNamespace(ns *reposchema.Namespace) *apischema.Namespace {
	if ns == nil {
		return nil
	}

	return &apischema.Namespace{
		Name:          ns.Name,
		Description:   ns.Description,
		Creator:       ns.Creator,
		CreateTime:    timestamppb.New(time.UnixMilli(ns.CreateTime)),
		UpdateTime:    timestamppb.New(time.UnixMilli(ns.UpdateTime)),
		ApiKey:        ns.ApiKey,
		ApiKeyPreview: maskApiKey(ns.ApiKey),
	}
}

func maskApiKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	const (
		prefixVisible = 3
		suffixVisible = 4
	)
	if len(apiKey) <= prefixVisible+suffixVisible {
		return strings.Repeat("*", len(apiKey))
	}

	maskLen := len(apiKey) - prefixVisible - suffixVisible
	return apiKey[:prefixVisible] + strings.Repeat("*", maskLen) + apiKey[len(apiKey)-suffixVisible:]
}

func generateApiKey() string {
	// TODO 加密后存储
	randText := rand.Text()
	return "sk-" + randText
}
