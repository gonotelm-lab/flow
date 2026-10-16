package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gonotelm-lab/flow/api/signature"
)

// 回调请求头。接收方读的就是这些头；定义与签名实现同源（api/signature），
// 因此 SDK 永远不可能与 flowserver 写出的头对不上。
const (
	// HeaderScheduleID 本次回调所属的 schedule id
	HeaderScheduleID = signature.HeaderScheduleID
	// HeaderFireID 本次 fire 的 id；同一次 fire 的所有重试与接管重投都不变，
	// 接收方必须用它做幂等去重——这是 at-least-once 契约的去重依据。
	HeaderFireID = signature.HeaderFireID
	// HeaderScheduledTime 本次触发计划时刻（unix 毫秒）
	HeaderScheduledTime = signature.HeaderScheduledTime
	// HeaderTimestamp 签名时间戳（unix 秒）
	HeaderTimestamp = signature.HeaderTimestamp
	// HeaderNamespace 本次回调所属的 namespace
	HeaderNamespace = signature.HeaderNamespace
	// HeaderSignature 签名值
	HeaderSignature = signature.HeaderSignature
)

// 验签缺省值。
const (
	// DefaultTolerance 是 X-Flow-Timestamp 允许的偏差（前后各这么多）。
	// 5 分钟足够覆盖网络抖动与轻微时钟漂移，又远小于「重放一个旧回调」的收益窗口。
	DefaultTolerance = 5 * time.Minute
	// DefaultMaxBodySize 是 VerifyRequest 愿意读进内存的 body 上限。
	// 超过就报 ErrBodyTooLarge，而不是把内存交给对端决定。
	DefaultMaxBodySize = 10 << 20
)

var (
	// ErrStaleTimestamp 时间戳超出容差。可能是重放，也可能是两边时钟不同步。
	ErrStaleTimestamp = errors.New("schedule: callback timestamp outside tolerance")
	// ErrBodyTooLarge body 超过 WithMaxBodySize 设定的上限。
	ErrBodyTooLarge = errors.New("schedule: callback body exceeds size limit")
	// ErrNilRequest 传了 nil 的 *http.Request。
	ErrNilRequest = errors.New("schedule: nil request")
	// ErrNoAPIKey 客户端没有配置 Config.APIKey，无法验签。
	ErrNoAPIKey = errors.New("schedule: client api key is empty")
)

type verifyOptions struct {
	tolerance    time.Duration
	toleranceSet bool
	maxBodySize  int64
	now          func() time.Time
	namespace    string
}

// VerifyOption 调整 VerifyRequest 的验签策略。缺省值见 DefaultTolerance / DefaultMaxBodySize。
type VerifyOption func(*verifyOptions)

// WithTolerance 设置时间戳容差。
//
// 传 0 或负数表示**不做**新鲜度校验（只验签名）。只有在你确信接收方另有防重放手段
// （比如 Fire-Id 去重表 + 短 TTL）时才这么做。
func WithTolerance(d time.Duration) VerifyOption {
	return func(o *verifyOptions) {
		o.tolerance = d
		o.toleranceSet = true
	}
}

// WithMaxBodySize 设置 VerifyRequest 读取 body 的上限。
// 传 0 或负数表示用 DefaultMaxBodySize。
func WithMaxBodySize(n int64) VerifyOption {
	return func(o *verifyOptions) { o.maxBodySize = n }
}

// WithNow 注入「现在」，用于测试（也是唯一需要它的场景）。
func WithNow(now func() time.Time) VerifyOption {
	return func(o *verifyOptions) {
		if now != nil {
			o.now = now
		}
	}
}

// VerifyRequest 校验一次定时回调请求，一条调用做完接收方该做的三件事：
//
//  1. 读取 body，读完还原回 r.Body，后续 handler 照常解码；
//  2. 复算 X-Flow-Signature，确认请求确实由持有该 namespace api_key 的 flowserver 发出，
//     且方法、路径、查询串、头、body 都没被改过；
//  3. 校验 X-Flow-Timestamp 在容差内，挡住旧回调重放；
//     配了 Config.Namespace 时，还会确认 X-Flow-Namespace 与它一致。
//
// 密钥与 namespace 都取自 Client 的 Config，接收方整条链路只需要持有一个 Client：
//
//	client, _ := schedule.New("", schedule.Config{Namespace: ns, APIKey: apiKey})
//
//	func handle(w http.ResponseWriter, r *http.Request) {
//		if err := client.VerifyRequest(r); err != nil {
//			http.Error(w, "unauthorized", http.StatusUnauthorized)
//			return
//		}
//		// 到这里 body 还能正常读，签名与时效都已确认
//		body, _ := io.ReadAll(r.Body)
//		w.WriteHeader(http.StatusOK)
//	}
//
// 验签失败一律返回非 nil，用 errors.Is 区分原因：
//
//	errors.Is(err, schedule.ErrStaleTimestamp)      → 过期或重放
//	errors.Is(err, schedule.ErrNoAPIKey)            → 没配 api_key
//	errors.Is(err, signature.ErrMismatch)           → 签名不符
//	errors.Is(err, signature.ErrMissingHeader)      → 请求不是 flow 回调
//	errors.Is(err, signature.ErrMalformedSignature) → 头被改坏
//	errors.Is(err, signature.ErrScopeMismatch)      → namespace 对不上
//
// fire id 去重仍然要自己做：验签只证明来源与完整性，at-least-once 下同一次 fire 会重投，
// 用 r.Header.Get(schedule.HeaderFireID) 建唯一索引即可。
func (c *Client) VerifyRequest(r *http.Request, opts ...VerifyOption) error {
	if r == nil {
		return ErrNilRequest
	}
	if err := c.checkAPIKey(); err != nil {
		return err
	}

	o := newVerifyOptions(opts)
	o.namespace = c.cfg.Namespace
	body, err := readAndRestoreBody(r, o.maxBodySize)
	if err != nil {
		return err
	}
	return verify(r, c.cfg.APIKey, body, o)
}

// VerifyRequestWithBody 用调用方已经读出来的 body 验签，不碰 r.Body。
//
// 适用场景：框架已经把 body 解成了结构体，或者 body 被缓存过只能读一次。
// body 必须与请求体逐字节相同，多一个空格都会导致签名不符。
func (c *Client) VerifyRequestWithBody(
	r *http.Request,
	body []byte,
	opts ...VerifyOption,
) error {
	if r == nil {
		return ErrNilRequest
	}
	if err := c.checkAPIKey(); err != nil {
		return err
	}

	o := newVerifyOptions(opts)
	o.namespace = c.cfg.Namespace
	return verify(r, c.cfg.APIKey, body, o)
}

// checkAPIKey 挡住「没配密钥就验签」：那种情况下任何签名都验不过，
// 早早报错比让调用方去猜 401 从哪来好。
func (c *Client) checkAPIKey() error {
	if c == nil || c.cfg.APIKey == "" {
		return ErrNoAPIKey
	}
	return nil
}

// verify 先验签、再判时效。
//
// 顺序是有意的：时效与 namespace 都来自请求头，先做密码学校验，就不会让
// 未经认证的数据影响任何判断。
func verify(r *http.Request, secret string, body []byte, o verifyOptions) error {
	if err := signature.Verify(secret, signature.Input{
		Method: r.Method,
		URL:    r.URL,
		Header: r.Header,
		Body:   body,
	}, r.Header.Get(HeaderSignature)); err != nil {
		return err
	}

	if o.tolerance > 0 {
		ts, err := signature.Timestamp(r.Header)
		if err != nil {
			return err
		}
		skew := o.now().Sub(time.Unix(ts, 0))
		if skew > o.tolerance || skew < -o.tolerance {
			return fmt.Errorf("%w: timestamp %s is %s away from now",
				ErrStaleTimestamp, time.Unix(ts, 0).UTC().Format(time.RFC3339), skew.Round(time.Second))
		}
	}

	if o.namespace != "" {
		if got := r.Header.Get(HeaderNamespace); got != o.namespace {
			return fmt.Errorf("%w: %s is %q, want %q",
				signature.ErrScopeMismatch, HeaderNamespace, got, o.namespace)
		}
	}

	return nil
}

// readAndRestoreBody 读走 body 再放回去。
//
// 还原这一步很关键：验签不该让调用方付出「body 已经被读空」的代价。
func readAndRestoreBody(r *http.Request, max int64) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}
	// Content-Length 已知时先拦一遍，避免明知超限还去读。
	if r.ContentLength > max {
		return nil, fmt.Errorf("%w: Content-Length %d > %d", ErrBodyTooLarge, r.ContentLength, max)
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("schedule: read callback body: %w", err)
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrBodyTooLarge, max)
	}

	if len(body) == 0 {
		r.Body = http.NoBody
	} else {
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	r.ContentLength = int64(len(body))
	return body, nil
}

func newVerifyOptions(opts []VerifyOption) verifyOptions {
	o := verifyOptions{
		maxBodySize: DefaultMaxBodySize,
		now:         time.Now,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if !o.toleranceSet {
		o.tolerance = DefaultTolerance
	}
	if o.maxBodySize <= 0 {
		o.maxBodySize = DefaultMaxBodySize
	}
	return o
}
