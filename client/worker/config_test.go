package worker

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigWithDefaults(t *testing.T) {
	cfg := ConfigWithDefaults(Config{
		Namespace: "ns",
		TaskType:  "render",
	})

	require.Equal(t, 1, cfg.MaxConcurrency)
	require.Equal(t, 5*time.Second, cfg.HeartbeatInterval)
	require.NotNil(t, cfg.Codec)
	require.NotNil(t, cfg.Logger)
}

func TestConfigWithDefaults_PreservesCustom(t *testing.T) {
	logger := slog.Default()
	cfg := ConfigWithDefaults(Config{
		Namespace:         "ns",
		TaskType:          "render",
		MaxConcurrency:    8,
		HeartbeatInterval: 2 * time.Second,
		Codec:             JSONCodec{},
		Logger:            logger,
	})

	require.Equal(t, 8, cfg.MaxConcurrency)
	require.Equal(t, 2*time.Second, cfg.HeartbeatInterval)
	require.Equal(t, logger, cfg.Logger)
}
