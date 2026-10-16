package config

import (
	"testing"
	"time"

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

func TestWorkerConfig_Validate_Cleanup(t *testing.T) {
	base := func() *WorkerConfig {
		return &WorkerConfig{
			PollWait:           time.Second,
			PollCheckInterval:  time.Second,
			StaleTaskTimeout:   time.Second,
			StaleScanInterval:  time.Second,
			RetryScanInterval:  time.Second,
			RetryScanBatch:     1,
			StaleScanBatch:     1,
			WorkerTTL:          time.Minute,
			WorkerRetention:    7 * 24 * time.Hour,
			WorkerReapInterval: time.Hour,
			WorkerReapBatch:    500,
		}
	}

	require.NoError(t, base().Validate())

	t.Run("retention_must_exceed_ttl", func(t *testing.T) {
		cfg := base()
		cfg.WorkerRetention = cfg.WorkerTTL
		require.Error(t, cfg.Validate())

		cfg.WorkerRetention = cfg.WorkerTTL - time.Second
		require.Error(t, cfg.Validate())
	})

	t.Run("ttl_must_be_positive", func(t *testing.T) {
		cfg := base()
		cfg.WorkerTTL = 0
		require.Error(t, cfg.Validate())
	})

	t.Run("reap_params_must_be_positive", func(t *testing.T) {
		cfg := base()
		cfg.WorkerReapInterval = 0
		require.Error(t, cfg.Validate())

		cfg = base()
		cfg.WorkerReapBatch = 0
		require.Error(t, cfg.Validate())
	})
}
