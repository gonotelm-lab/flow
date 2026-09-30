package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAutoInit(t *testing.T) {
	const confPath = "../../etc/conf.toml.tpl"

	t.Run("default_true", func(t *testing.T) {
		t.Setenv("FLOW_DB_AUTO_INIT", "")
		cfg, err := Load(confPath)
		require.NoError(t, err)
		require.True(t, cfg.DB.AutoInit)
	})

	t.Run("override_false", func(t *testing.T) {
		t.Setenv("FLOW_DB_AUTO_INIT", "false")
		cfg, err := Load(confPath)
		require.NoError(t, err)
		require.False(t, cfg.DB.AutoInit)
	})
}
