package worker

import (
	"log/slog"
	"time"
)

type Config struct {
	Namespace         string
	TaskType          string
	Name              string
	MaxConcurrency    int
	HeartbeatInterval time.Duration
	Codec             Codec
	Logger            *slog.Logger
}

func ConfigWithDefaults(cfg Config) Config {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	if cfg.Codec == nil {
		cfg.Codec = JSONCodec{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}
