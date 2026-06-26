package task

type submitOptions struct {
	maxRetry int
}

type SubmitOption func(*submitOptions)

func WithMaxRetry(n int) SubmitOption {
	return func(o *submitOptions) {
		o.maxRetry = n
	}
}
