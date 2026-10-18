package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const authConfPath = "../../etc/conf.toml.tpl"

// 默认关闭是兼容的前提：默认配置下行为必须和加鉴权之前一致，这里用真实模板钉住。
func TestApiServerAuth_DefaultOff(t *testing.T) {
	t.Setenv("FLOW_API_AUTH_ENABLED", "")

	cfg, err := Load(authConfPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.ApiServer)
	require.NotNil(t, cfg.ApiServer.Auth, "template should contain an [apiServer.auth] section")

	assert.False(t, cfg.ApiServer.Auth.Enabled)
	assert.False(t, cfg.ApiServer.AuthEnabled())
}

func TestApiServerAuth_CanBeEnabled(t *testing.T) {
	t.Setenv("FLOW_API_AUTH_ENABLED", "true")

	cfg, err := Load(authConfPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.ApiServer)
	require.NotNil(t, cfg.ApiServer.Auth)

	assert.True(t, cfg.ApiServer.Auth.Enabled)
	assert.True(t, cfg.ApiServer.AuthEnabled())
}

// 没有 [apiServer.auth] 段的配置（例如别处程序化构造的 Config）同样视为关闭。
func TestApiServerAuth_MissingSectionIsOff(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ApiServer
		want bool
	}{
		{name: "nil ApiServer", cfg: nil, want: false},
		{name: "no auth section", cfg: &ApiServer{}, want: false},
		{name: "auth section without enabled", cfg: &ApiServer{Auth: &AuthConfig{}}, want: false},
		{name: "explicitly enabled", cfg: &ApiServer{Auth: &AuthConfig{Enabled: true}}, want: true},
		{name: "explicitly disabled", cfg: &ApiServer{Auth: &AuthConfig{Enabled: false}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.AuthEnabled())
		})
	}
}
