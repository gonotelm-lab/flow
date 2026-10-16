package worker

import (
	"log/slog"
	"time"

	"github.com/gonotelm-lab/flow/client/worker/internal/runtime"
)

// TraceMode 和它的常量定义在 runtime 包，这里用类型别名导出，作为 worker 的对外 API。
type TraceMode = runtime.TraceMode

const (
	TraceModeChild = runtime.TraceModeChild
	TraceModeLink  = runtime.TraceModeLink
)

type Config struct {
	Namespace         string
	TaskType          string
	Name              string
	MaxConcurrency    int
	HeartbeatInterval time.Duration
	Codec             Codec
	Logger            *slog.Logger
	TraceMode         TraceMode
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
	if cfg.TraceMode == "" {
		cfg.TraceMode = TraceModeChild
	}
	return cfg
}
