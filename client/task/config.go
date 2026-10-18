package task

type submitOptions struct {
	maxRetry int
}

// SubmitOption 调整 v1 的提交参数。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 SubmitOption。
type SubmitOption func(*submitOptions)

// WithMaxRetry 设置最大重试次数（不含首次尝试）。
//
// Deprecated: 请改用 github.com/gonotelm-lab/flow/client/task/v2 的 WithMaxRetry。
func WithMaxRetry(n int) SubmitOption {
	return func(o *submitOptions) {
		o.maxRetry = n
	}
}
