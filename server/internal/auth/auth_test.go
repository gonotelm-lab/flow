package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		md      metadata.MD
		want    string
		wantErr error
	}{
		{
			name:    "missing authorization",
			md:      metadata.MD{},
			wantErr: ErrMissingKey,
		},
		{
			name:    "only other headers",
			md:      metadata.Pairs("x-trace-id", "abc"),
			wantErr: ErrMissingKey,
		},
		{
			name: "standard bearer",
			md:   metadata.Pairs(MetadataKey, "Bearer sk-abc"),
			want: "sk-abc",
		},
		{
			name: "scheme is case-insensitive",
			md:   metadata.Pairs(MetadataKey, "bearer sk-abc"),
			want: "sk-abc",
		},
		{
			name: "surrounding whitespace is trimmed",
			md:   metadata.Pairs(MetadataKey, "  Bearer   sk-abc  "),
			want: "sk-abc",
		},
		{
			name:    "bare key is rejected",
			md:      metadata.Pairs(MetadataKey, "sk-abc"),
			wantErr: ErrMalformedKey,
		},
		{
			name:    "other schemes are rejected",
			md:      metadata.Pairs(MetadataKey, "Basic sk-abc"),
			wantErr: ErrMalformedKey,
		},
		{
			name:    "empty token after bearer",
			md:      metadata.Pairs(MetadataKey, "Bearer "),
			wantErr: ErrEmptyKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := BearerToken(tt.md)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAllows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	authed := WithNamespace(ctx, "demo")

	tests := []struct {
		name      string
		ctx       context.Context
		namespace string
		want      bool
	}{
		{
			name:      "always true when auth is disabled (no namespace in ctx)",
			ctx:       ctx,
			namespace: "anything",
			want:      true,
		},
		{
			name:      "authenticated with matching namespace",
			ctx:       authed,
			namespace: "demo",
			want:      true,
		},
		{
			name:      "authenticated with different namespace",
			ctx:       authed,
			namespace: "other",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Allows(tt.ctx, tt.namespace))
		})
	}
}

func TestNamespaceFromContext(t *testing.T) {
	t.Parallel()

	namespace, ok := NamespaceFromContext(context.Background())
	assert.False(t, ok)
	assert.Empty(t, namespace)

	namespace, ok = NamespaceFromContext(WithNamespace(context.Background(), "demo"))
	require.True(t, ok)
	assert.Equal(t, "demo", namespace)

	// 「认证成功但 namespace 为空」必须 fail closed，不能落到「没鉴权就放行」那条分支。
	_, ok = NamespaceFromContext(WithNamespace(context.Background(), ""))
	assert.False(t, ok)
	assert.False(t, Allows(WithNamespace(context.Background(), ""), "anything"))
}
