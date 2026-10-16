package schedule

import (
	"time"
)

// DefaultContentType 与调度器的缺省值保持一致。
const DefaultContentType = "application/octet-stream"

type options struct {
	name           string
	idempotencyKey string
	contentType    string
}

// Option 用于在构造 Spec 时补上定义里的可选字段。
type Option func(*options)

func WithName(name string) Option {
	return func(o *options) { o.name = name }
}

// WithIdempotencyKey 设置幂等键。
//
// 键在 namespace 内唯一，schedule 停止或删除后释放。不设的话 Register 会自己生成一个
// 随机键，只有显式给键才有「重复注册返回同一条」的效果。
func WithIdempotencyKey(key string) Option {
	return func(o *options) { o.idempotencyKey = key }
}

// WithContentType 指定回调时使用的 Content-Type。
func WithContentType(contentType string) Option {
	return func(o *options) { o.contentType = contentType }
}

func (o *options) apply(s *Spec) {
	if s == nil {
		return
	}
	if o.name != "" {
		s.Name = o.name
	}
	if o.idempotencyKey != "" {
		s.IdempotencyKey = o.idempotencyKey
	}
	if o.contentType != "" {
		s.ContentType = o.contentType
	}
	if s.ContentType == "" {
		s.ContentType = DefaultContentType
	}
}

type listOptions struct {
	scheduleType Type
	state        State
	id           string
	page         int
	pageSize     int
}

// ListOption 用于拼装 List 的筛选条件。namespace 由 Config 决定，不在这里传。
type ListOption func(*listOptions)

func WithScheduleType(t Type) ListOption {
	return func(o *listOptions) { o.scheduleType = t }
}

func WithState(s State) ListOption {
	return func(o *listOptions) { o.state = s }
}

func WithID(id string) ListOption {
	return func(o *listOptions) { o.id = id }
}

func WithPage(page, pageSize int) ListOption {
	return func(o *listOptions) {
		o.page = page
		o.pageSize = pageSize
	}
}

func newOptions(opts []Option) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func newListOptions(opts []ListOption) *listOptions {
	o := &listOptions{}
	for _, opt := range opts {
		opt(o)
	}
	if o.page <= 0 {
		o.page = 1
	}
	if o.pageSize <= 0 {
		o.pageSize = 20
	}
	return o
}

// ---------------------------------------------------------------------------
// 便利构造函数：调用方不必手拼触发方式。
// ---------------------------------------------------------------------------

// OneShot 构造一条一次性触发的定义：在 fireAt 时刻回调 callbackURL。
//
// fireAt 必须严格晚于当前时间，服务端用数据库时间判定并拒绝过期注册。
func OneShot(fireAt time.Time, callbackURL string, payload []byte, opts ...Option) Spec {
	s := Spec{
		Trigger:     OneShotTrigger{FireAt: fireAt},
		CallbackURL: callbackURL,
		Payload:     payload,
	}
	newOptions(opts).apply(&s)
	return s
}

// After 构造一条「延迟 d 之后触发一次」的一次性任务。
//
// 相对时间由客户端换算成绝对时刻，服务端不接受 delay 语义，免去「从谁的时刻算起」的歧义。
func After(d time.Duration, callbackURL string, payload []byte, opts ...Option) Spec {
	return OneShot(time.Now().Add(d), callbackURL, payload, opts...)
}

// Cron 构造一条重复触发的定义。expr 是标准 5 字段，timezone 是 IANA 时区名，留空表示 UTC。
func Cron(expr, timezone, callbackURL string, payload []byte, opts ...Option) Spec {
	s := Spec{
		Trigger:     CronTrigger{Expr: expr, Timezone: timezone},
		CallbackURL: callbackURL,
		Payload:     payload,
	}
	newOptions(opts).apply(&s)
	return s
}
