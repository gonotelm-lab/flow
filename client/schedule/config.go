package schedule

import (
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
)

// DefaultContentType 与调度器的缺省值保持一致。
const DefaultContentType = "application/octet-stream"

type options struct {
	namespace      string
	name           string
	idempotencyKey string
	contentType    string
}

// Option 用于在注册/更新时补上定义里的可选字段。
type Option func(*options)

// WithNamespace 设置 namespace。
//
// 也可以直接给 Schedule.Namespace 赋值，两种方式等价。
func WithNamespace(namespace string) Option {
	return func(o *options) { o.namespace = namespace }
}

func WithName(name string) Option {
	return func(o *options) { o.name = name }
}

// WithIdempotencyKey 设置幂等键。
//
// 传了键之后，重复注册（网络重试、进程重启重放）会返回已存在的那条 schedule，
// 而不是新建一条。键在 namespace 内唯一，schedule 被删除后释放。
func WithIdempotencyKey(key string) Option {
	return func(o *options) { o.idempotencyKey = key }
}

// WithContentType 指定回调时使用的 Content-Type。
func WithContentType(contentType string) Option {
	return func(o *options) { o.contentType = contentType }
}

func (o *options) apply(s *schemav1.Schedule) {
	if s == nil {
		return
	}
	if o.namespace != "" {
		s.Namespace = o.namespace
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
	if s.GetContentType() == "" {
		s.ContentType = DefaultContentType
	}
}

type listOptions struct {
	namespace    string
	scheduleType schemav1.ScheduleType
	state        schemav1.ScheduleState
	id           string
	page         int
	pageSize     int
}

// ListOption 用于拼装 List 的筛选条件。
type ListOption func(*listOptions)

// WithListNamespace 限定 List 的 namespace 范围。
//
// 名字与注册用的 WithNamespace 区分开：注册时它写进定义，列表时它只是筛选条件。
func WithListNamespace(namespace string) ListOption {
	return func(o *listOptions) { o.namespace = namespace }
}

func WithScheduleType(t schemav1.ScheduleType) ListOption {
	return func(o *listOptions) { o.scheduleType = t }
}

func WithState(s schemav1.ScheduleState) ListOption {
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
// 便利构造函数
//
// 调用方不需要（也不应该）手拼 proto 的 oneof。
// ---------------------------------------------------------------------------

// OneShot 构造一条一次性触发：在 fireAt 时刻回调 callbackURL。
//
// fireAt 必须严格晚于当前时间（服务端用数据库时间判定并拒绝过期注册）。
func OneShot(fireAt time.Time, callbackURL string, payload []byte, opts ...Option) *schemav1.Schedule {
	s := &schemav1.Schedule{
		CallbackUrl: callbackURL,
		Payload:     payload,
		Trigger: &schemav1.Schedule_OneShot{
			OneShot: &schemav1.OneShotTrigger{FireAt: fireAt.UnixMilli()},
		},
	}
	newOptions(opts).apply(s)
	return s
}

// After 构造一条「延迟 d 之后触发一次」的一次性任务。
//
// 相对时间由客户端换算成绝对时刻，服务端不接受 delay 语义——避免「到底从谁的时刻算起」。
func After(d time.Duration, callbackURL string, payload []byte, opts ...Option) *schemav1.Schedule {
	return OneShot(time.Now().Add(d), callbackURL, payload, opts...)
}

// Cron 构造一条重复触发。
//
// expr 是标准 5 字段（分 时 日 月 周），最小粒度 1 分钟；timezone 是 IANA 时区名，
// 留空表示 UTC。
//
// 方言提醒：星期是 0-6 且 **0 = Sunday**，月份 1-12，不支持 L / W / # 与 @ 描述符，
// 与 Quartz 并不相同（Quartz 是 1-7 且 1=Sunday、月份 0-11、7 字段）。
func Cron(expr, timezone, callbackURL string, payload []byte, opts ...Option) *schemav1.Schedule {
	s := &schemav1.Schedule{
		CallbackUrl: callbackURL,
		Payload:     payload,
		Trigger: &schemav1.Schedule_Cron{
			Cron: &schemav1.CronTrigger{Expr: expr, Timezone: timezone},
		},
	}
	newOptions(opts).apply(s)
	return s
}
