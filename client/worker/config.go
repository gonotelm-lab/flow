package worker

import (
	"errors"
	"log/slog"
	"strings"
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
	// Namespace 必填：一个 worker 只消费一个 namespace 的任务。
	// 为空时由 Start 返回 ErrEmptyNamespace（构造不校验）。
	Namespace string
	TaskType  string
	Name      string
	// APIKey 是该 namespace 的 api_key：服务端开启鉴权后必填，关闭时留空即可。
	APIKey string

	MaxConcurrency    int
	HeartbeatInterval time.Duration
	Codec             Codec
	Logger            *slog.Logger
	TraceMode         TraceMode
}

// ErrEmptyNamespace 没有配置 Config.Namespace。
var ErrEmptyNamespace = errors.New("worker: namespace is required")

func (cfg Config) validate() error {
	if strings.TrimSpace(cfg.Namespace) == "" {
		return ErrEmptyNamespace
	}

	return nil
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
