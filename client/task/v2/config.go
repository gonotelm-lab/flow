package task

import (
	"errors"
	"strings"
)

// Config 是客户端配置：一个 Client 对应一个 namespace。
type Config struct {
	// Namespace 为空时调用返回 ErrEmptyNamespace（构造不校验）。
	Namespace string
	// APIKey 是该 namespace 的 api_key：服务端开启鉴权后必填，关闭时留空即可。
	APIKey string
}

// ErrEmptyNamespace 没有配置 Config.Namespace。
var ErrEmptyNamespace = errors.New("task: namespace is required")

func (cfg Config) validate() error {
	if strings.TrimSpace(cfg.Namespace) == "" {
		return ErrEmptyNamespace
	}

	return nil
}

type submitOptions struct {
	maxRetry int
}

// SubmitOption 调整提交参数。
type SubmitOption func(*submitOptions)

// WithMaxRetry 设置最大重试次数（不含首次尝试）。
func WithMaxRetry(n int) SubmitOption {
	return func(o *submitOptions) {
		o.maxRetry = n
	}
}
