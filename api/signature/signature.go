// Package signature 定义 Flow 定时回调（Schedule）的请求签名：FLOW-HMAC-SHA256。
//
// 这套签名由四个部件组成，思路是「把请求本身规范化后，再用一条派生密钥签它」：
//
//	CanonicalRequest  = 方法 + 路径 + 查询串 + 规范化头 + 已签名头清单 + body 哈希
//	StringToSign      = "FLOW-HMAC-SHA256" + 时刻 + scope + CanonicalRequest 哈希
//	SigningKey        = HMAC 链("FLOW"+secret, 日期, namespace, "flow_request")
//	Signature         = hex(HMAC(SigningKey, StringToSign))
//
// 签名结果放在 X-Flow-Signature 头里，形如：
//
//	FLOW-HMAC-SHA256 Scope=20261015/demo/flow_request, \
//	  Signed=content-type;x-flow-fire-id;..., Signature=<hex>
//
// 为什么不用「HMAC(secret, timestamp + body)」那种一行式签名：
//
//   - **密钥不是直接用 api_key**，而是按「日期 → namespace」两级派生。派生的意义是
//     隔离：某一天的派生密钥泄露不影响其他日期，改 namespace 也不会让旧签名继续有效。
//   - **签名覆盖请求的形状**（方法、路径、查询串、头、body），而不是只覆盖 body。
//     这样 body 被改、头被改、请求被挪到另一个路径重放，都会验签失败。
//   - **已签名头清单是闭合的**：签名方全签，验签方要求清单里必须有那几个必需头，
//     否则签名方可以「少签 timestamp 再重放」而不被察觉。清单本身也在签名范围内，
//     所以它无法被篡改。
//
// 有意不签 host：接收方普遍部署在反向代理后，nginx 默认就会用
// `proxy_set_header Host $proxy_host` 改写 Host，签了它等于要求所有用户改代理配置，
// 否则验签必然失败。而 host 在这里也不提供额外保护——密钥是 namespace 级的，
// 签名已覆盖 schedule id、路径与查询串。
//
// 本包同时被服务端（签名）与客户端 SDK（验签）引用，是这套算法的**唯一实现**，
// 两边不可能漂移。改这里就等于同时改了两端。
package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Algorithm 是 X-Flow-Signature 的算法标识。
const Algorithm = "FLOW-HMAC-SHA256"

// 回调请求头。接收方按这些字段做校验与去重。
const (
	// HeaderScheduleID 本次回调所属的 schedule id
	HeaderScheduleID = "X-Flow-Schedule-Id"
	// HeaderFireID 本次 fire 的 id；同一次 fire 的所有重试与接管重投都不变，
	// 接收方必须用它做幂等去重——这是 at-least-once 契约的去重依据。
	HeaderFireID = "X-Flow-Fire-Id"
	// HeaderScheduledTime 本次触发计划时刻（unix 毫秒）
	HeaderScheduledTime = "X-Flow-Scheduled-Time"
	// HeaderTimestamp 签名时间戳（unix 秒）。它参与签名，接收方据此判重放。
	HeaderTimestamp = "X-Flow-Timestamp"
	// HeaderNamespace 本次回调所属的 namespace。它同时是签名 scope 的组成部分，
	// 让接收方不必猜「该用哪把 api_key 验签」。
	HeaderNamespace = "X-Flow-Namespace"
	// HeaderSignature 签名值，形如
	// "FLOW-HMAC-SHA256 Scope=<date>/<namespace>/flow_request, Signed=<...>, Signature=<hex>"
	HeaderSignature = "X-Flow-Signature"
)

// 派生链上的固定成分：密钥前缀、scope 的终止分量。
const (
	keyPrefix  = "FLOW"
	scopeFinal = "flow_request"
)

// SignedHeaderNames 是签名必须覆盖的头，升序、小写。
//
// 名单是**闭合**的：签名方全签，验签方要求这些头都存在且都出现在 Signed 清单里。
// 否则签名方可以少签一个（比如 timestamp）而不被察觉，防重放就落空了。
var SignedHeaderNames = []string{
	"content-type",
	"x-flow-fire-id",
	"x-flow-namespace",
	"x-flow-schedule-id",
	"x-flow-scheduled-time",
	"x-flow-timestamp",
}

// 验签失败的原因分类。调用方用 errors.Is 判断该怎么响应（缺头 400 / 不匹配 401）。
var (
	// ErrMissingHeader 缺少必需的请求头（含 HeaderSignature 本身为空）。
	ErrMissingHeader = errors.New("signature: missing required header")
	// ErrMalformedSignature X-Flow-Signature 的值不符合约定格式。
	ErrMalformedSignature = errors.New("signature: malformed signature header")
	// ErrUnsupportedAlgorithm 算法标识不是 Algorithm。
	ErrUnsupportedAlgorithm = errors.New("signature: unsupported algorithm")
	// ErrScopeMismatch 凭证 scope 与请求本身对不上（日期、namespace、已签名头清单）。
	ErrScopeMismatch = errors.New("signature: credential scope mismatch")
	// ErrMismatch 签名值不匹配。密钥不对、body 被改、头被改都会落到这里。
	ErrMismatch = errors.New("signature: signature mismatch")
)

// Input 是一次回调请求中参与签名的全部输入。
//
// 时间戳与 namespace 不单独传参：它们就在 Header 里，而且本身就是被签名的头，
// 从 header 取能保证「算签名用的值」与「发出去的值」不可能不一致。
type Input struct {
	// Method HTTP 方法，通常是 POST。
	Method string
	// URL 回调地址。只有路径与查询串参与签名，scheme / host 不参与（见包注释）。
	// 接收方传 *http.Request.URL 即可。
	URL *url.URL
	// Header 请求头，必须含 SignedHeaderNames 列出的全部头。
	Header http.Header
	// Body 回调 body 的原始字节。空 body 合法，哈希为空串的 sha256。
	Body []byte
}

// Sign 计算 X-Flow-Signature 头的完整值。
//
// secret 是 namespace 的 api_key。in.Header 里必须已经有时间戳、namespace、
// schedule id、fire id、scheduled time 与 content-type——签名只是把它们算进去。
func Sign(secret string, in Input) (string, error) {
	ts, err := Timestamp(in.Header)
	if err != nil {
		return "", err
	}

	namespace, err := requiredHeader(in.Header, HeaderNamespace)
	if err != nil {
		return "", err
	}
	if err := validateNamespace(namespace); err != nil {
		return "", err
	}

	canonical, signed, err := canonicalRequest(in, SignedHeaderNames)
	if err != nil {
		return "", err
	}

	date := timestampDate(ts)
	scope := credential{date: date, namespace: namespace}.String()
	sig := hmacSHA256Hex(signingKey(secret, date, namespace), stringToSign(ts, scope, canonical))

	return Algorithm +
		" Scope=" + scope +
		", Signed=" + signed +
		", Signature=" + sig, nil
}

// Verify 校验 X-Flow-Signature 的值。
//
// 它只回答「签名是否由持有 secret 的一方对这份请求生成」。时效性（重放窗口）是
// 调用方的策略，本函数不做——`Timestamp` 已经帮忙把时间戳解析出来了。
//
// 返回的 error 一律可以用 errors.Is 对上上面那组哨兵错误。
func Verify(secret string, in Input, headerValue string) error {
	if strings.TrimSpace(headerValue) == "" {
		return fmt.Errorf("%w: %s is empty", ErrMissingHeader, HeaderSignature)
	}

	scope, signed, want, err := parseHeader(headerValue)
	if err != nil {
		return err
	}

	ts, err := Timestamp(in.Header)
	if err != nil {
		return err
	}

	// scope 的日期必须与时间戳同一天。不对的话，同一份请求可以拿另一个日期的
	// 派生密钥重复使用，scope 里的日期就形同虚设。
	if got := timestampDate(ts); got != scope.date {
		return fmt.Errorf("%w: scope date %s, timestamp date %s", ErrScopeMismatch, scope.date, got)
	}

	namespace, err := requiredHeader(in.Header, HeaderNamespace)
	if err != nil {
		return err
	}
	if err := validateNamespace(namespace); err != nil {
		return err
	}
	if namespace != scope.namespace {
		return fmt.Errorf("%w: scope namespace %q, header namespace %q",
			ErrScopeMismatch, scope.namespace, namespace)
	}

	canonical, _, err := canonicalRequest(in, signed)
	if err != nil {
		return err
	}

	expected := hmacSHA256Hex(signingKey(secret, scope.date, scope.namespace),
		stringToSign(ts, scope.String(), canonical))

	// 定长比较，避免用比较耗时泄露前缀信息。
	if !hmac.Equal([]byte(expected), []byte(want)) {
		return ErrMismatch
	}
	return nil
}

// Timestamp 读取并解析 X-Flow-Timestamp（unix 秒）。
func Timestamp(h http.Header) (int64, error) {
	raw := strings.TrimSpace(h.Get(HeaderTimestamp))
	if raw == "" {
		return 0, fmt.Errorf("%w: %s", ErrMissingHeader, HeaderTimestamp)
	}
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ts <= 0 {
		return 0, fmt.Errorf("%w: %s=%q is not a unix timestamp", ErrMalformedSignature, HeaderTimestamp, raw)
	}
	return ts, nil
}

// ---------------------------------------------------------------------------
// CanonicalRequest / StringToSign
// ---------------------------------------------------------------------------

// canonicalRequest 拼出 CanonicalRequest，并返回其中的 Signed 清单行。
//
// 形状（注意规范化头块自身以 \n 结尾，Join 之后再补一个 \n，于是头块与清单之间
// 恰好隔一个空行）：
//
//	POST\n
//	/hook\n
//	a=1&b=2\n
//	content-type:application/json\n
//	...\n
//	\n
//	content-type;x-flow-fire-id;...\n
//	<hex sha256(body)>
func canonicalRequest(in Input, signedHeaders []string) (string, string, error) {
	if err := validateSignedHeaders(signedHeaders); err != nil {
		return "", "", err
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		return "", "", fmt.Errorf("%w: HTTP method is empty", ErrMalformedSignature)
	}

	var canonicalHeaders strings.Builder
	for _, name := range signedHeaders {
		values := in.Header.Values(name)
		if len(values) == 0 {
			return "", "", fmt.Errorf("%w: %s", ErrMissingHeader, name)
		}
		// 同名多头用逗号连接。
		value := normalizeHeaderValue(strings.Join(values, ","))
		if value == "" {
			return "", "", fmt.Errorf("%w: %s is empty", ErrMissingHeader, name)
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(value)
		canonicalHeaders.WriteByte('\n')
	}

	signed := strings.Join(signedHeaders, ";")
	canonical := strings.Join([]string{
		method,
		canonicalURI(in.URL),
		canonicalQueryString(in.URL),
		canonicalHeaders.String(),
		signed,
		sha256Hex(in.Body),
	}, "\n")

	return canonical, signed, nil
}

// stringToSign 拼出 StringToSign。时刻用完整的 ISO8601，它和 scope 里的日期
// 由同一个 X-Flow-Timestamp 推出；timestamp 头本身也在 SignedHeaderNames 里，
// 所以「换个时间戳重放」必然破坏签名。
func stringToSign(ts int64, scope, canonicalRequest string) string {
	return strings.Join([]string{
		Algorithm,
		timestampISO8601(ts),
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
}

// ---------------------------------------------------------------------------
// 头与 URL 的规范化
// ---------------------------------------------------------------------------

func validateSignedHeaders(signed []string) error {
	if len(signed) == 0 {
		return fmt.Errorf("%w: Signed list is empty", ErrMalformedSignature)
	}

	prev := ""
	for _, name := range signed {
		if !isHeaderName(name) || name != strings.ToLower(name) {
			return fmt.Errorf("%w: illegal signed header %q", ErrMalformedSignature, name)
		}
		if prev != "" && name <= prev {
			return fmt.Errorf("%w: Signed list must be sorted and unique", ErrMalformedSignature)
		}
		prev = name
	}

	// 闭合名单：少签任何一个必需头都拒绝，否则「少签 timestamp 再重放」就无法发现。
	for _, required := range SignedHeaderNames {
		if !slices.Contains(signed, required) {
			return fmt.Errorf("%w: %s is not in the Signed list", ErrScopeMismatch, required)
		}
	}
	return nil
}

func isHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
}

// normalizeHeaderValue 去掉首尾空白并把连续空白压成一个空格。
// 在代理重写 Content-Type 之类场景下，两侧按同一规则折叠，结论仍然一致。
func normalizeHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	if !strings.ContainsAny(v, " \t") {
		return v
	}
	return strings.Join(strings.FieldsFunc(v, func(r rune) bool {
		return r == ' ' || r == '\t'
	}), " ")
}

// canonicalURI 取路径的百分号编码形式。
//
// 用 EscapedPath 而不是 Path：前者保留发送方实际写在 URL 里的编码，接收方解析请求行
// 后拿到的也是同一串，两边天然对齐；Path 是解码后的，重新编码可能改变 %2F 这类语义。
func canonicalURI(u *url.URL) string {
	if u == nil {
		return "/"
	}
	if p := u.EscapedPath(); p != "" {
		return p
	}
	return "/"
}

// canonicalQueryString 规范化查询串：先解码再按 RFC3986 编码，
// 然后按「编码后的名、编码后的值」排序，最后用 & 连接。
func canonicalQueryString(u *url.URL) string {
	if u == nil || u.RawQuery == "" {
		return ""
	}

	type pair struct{ name, value string }
	pairs := make([]pair, 0, strings.Count(u.RawQuery, "&")+1)
	for _, kv := range strings.Split(u.RawQuery, "&") {
		if kv == "" {
			continue
		}
		name, value, _ := strings.Cut(kv, "=")
		pairs = append(pairs, pair{
			name:  uriEncode(queryUnescape(name)),
			value: uriEncode(queryUnescape(value)),
		})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].name != pairs[j].name {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].value < pairs[j].value
	})

	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.name)
		b.WriteByte('=')
		b.WriteString(p.value)
	}
	return b.String()
}

// queryUnescape 尽力解码；解不开（非法 % 序列）就原样用，保证两边规则一致。
func queryUnescape(s string) string {
	if v, err := url.QueryUnescape(s); err == nil {
		return v
	}
	return s
}

// uriEncode 实现 RFC3986：非保留字符原样，其余 %XX（大写十六进制）。
// 与 url.QueryEscape 的区别是空格编成 %20 而不是 +，且不放过 '/'。
func uriEncode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// 密钥派生与哈希
// ---------------------------------------------------------------------------

// signingKey 是两级派生链：前缀密钥 → 日期 → namespace → 终止分量。
// 每一层都是 HMAC-SHA256，任何一层变了签名都变。
func signingKey(secret, date, namespace string) []byte {
	k := hmacSHA256Bytes([]byte(keyPrefix+secret), date)
	k = hmacSHA256Bytes(k, namespace)
	return hmacSHA256Bytes(k, scopeFinal)
}

func hmacSHA256Bytes(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

func hmacSHA256Hex(key []byte, msg string) string {
	return hex.EncodeToString(hmacSHA256Bytes(key, msg))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// X-Flow-Signature 的解析
// ---------------------------------------------------------------------------

// credential 是签名值里的 Scope 段：<date>/<namespace>/flow_request。
//
// namespace 只校验非空（见 Namespace proto），所以它可能带逗号、空格甚至 %。
// 直接写进 header 会把「逗号分参数、空格分算法」的语法撑破，因此这一段按 RFC3986
// 编码后再写；解码回来的原始值才是参与密钥派生的那个。常见 namespace（demo 之类）
// 全是非保留字符，编码后原样不变。
type credential struct {
	date      string
	namespace string
}

func (c credential) String() string {
	return strings.Join([]string{c.date, uriEncode(c.namespace), scopeFinal}, "/")
}

// decodeNamespace 把 Scope 里的 namespace 段解回原始值。
//
// 用 PathUnescape 而不是 QueryUnescape：后者会把 '+' 当空格，而我们的编码器从不产出
// '+'（'+' 会被编成 %2B），两种规则混用会制造不必要的歧义。
func decodeNamespace(encoded string) (string, error) {
	namespace, err := url.PathUnescape(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: Scope namespace %q", ErrMalformedSignature, encoded)
	}
	if err := validateNamespace(namespace); err != nil {
		return "", err
	}
	return namespace, nil
}

// validateNamespace 拦住无法放进 HTTP 头的 namespace（控制字符）。
// 其余字符（逗号、空格、%）由 Scope 的编码负责，不在这里限制。
func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("%w: %s is empty", ErrMissingHeader, HeaderNamespace)
	}
	for i := 0; i < len(namespace); i++ {
		if c := namespace[i]; c < 0x20 || c == 0x7f {
			return fmt.Errorf("%w: %s contains control character", ErrMalformedSignature, HeaderNamespace)
		}
	}
	return nil
}

func parseHeader(value string) (credential, []string, string, error) {
	var (
		cred   credential
		signed []string
		sig    string
	)

	value = strings.TrimSpace(value)
	algorithm, rest, ok := strings.Cut(value, " ")
	if !ok {
		return cred, nil, "", fmt.Errorf("%w: expected %q and parameters", ErrMalformedSignature, Algorithm)
	}
	if algorithm != Algorithm {
		return cred, nil, "", fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, algorithm)
	}

	// 参数不允许重复。
	//
	// 如果按「后者覆盖前者」解析，同一个 header 在不同实现里（代理、WAF、日志、业务）
	// 就可能被读出不同的 scope —— 签名本身没被伪造，但「看到的是哪一份」变成了实现细节。
	// 直接拒绝重复，header 与语义就一一对应了。
	seen := make(map[string]bool, 3)

	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		name, val, ok := strings.Cut(part, "=")
		if !ok {
			return cred, nil, "", fmt.Errorf("%w: parameter %q", ErrMalformedSignature, part)
		}
		if seen[name] {
			return cred, nil, "", fmt.Errorf("%w: duplicate parameter %q", ErrMalformedSignature, name)
		}
		seen[name] = true

		switch name {
		case "Scope":
			fields := strings.Split(val, "/")
			if len(fields) != 3 || fields[2] != scopeFinal {
				return cred, nil, "", fmt.Errorf("%w: Scope %q", ErrMalformedSignature, val)
			}
			if !isDate(fields[0]) {
				return cred, nil, "", fmt.Errorf("%w: Scope date %q", ErrMalformedSignature, fields[0])
			}
			namespace, err := decodeNamespace(fields[1])
			if err != nil {
				return cred, nil, "", err
			}
			cred = credential{date: fields[0], namespace: namespace}
			// 只认规范编码形式：同一个请求只应有一种合法写法。非规范写法（比如把空格
			// 原样塞进来）虽然能解出同样的 namespace，但会让 header 出现两种等价形态。
			if cred.String() != val {
				return cred, nil, "", fmt.Errorf("%w: Scope %q is not canonical", ErrMalformedSignature, val)
			}
		case "Signed":
			if val == "" {
				return cred, nil, "", fmt.Errorf("%w: Signed list is empty", ErrMalformedSignature)
			}
			signed = strings.Split(val, ";")
		case "Signature":
			if !isHexSignature(val) {
				return cred, nil, "", fmt.Errorf("%w: Signature %q", ErrMalformedSignature, val)
			}
			sig = val
		default:
			return cred, nil, "", fmt.Errorf("%w: unknown parameter %q", ErrMalformedSignature, name)
		}
	}

	if cred.date == "" {
		return cred, nil, "", fmt.Errorf("%w: Scope is missing", ErrMalformedSignature)
	}
	if len(signed) == 0 {
		return cred, nil, "", fmt.Errorf("%w: Signed list is missing", ErrMalformedSignature)
	}
	if sig == "" {
		return cred, nil, "", fmt.Errorf("%w: Signature is missing", ErrMalformedSignature)
	}
	return cred, signed, sig, nil
}

func isDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isHexSignature 要求完整长度的 sha256 十六进制（64 位小写）。
// 长度先卡死，比较才是定长的。
func isHexSignature(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func requiredHeader(h http.Header, name string) (string, error) {
	v := strings.TrimSpace(h.Get(name))
	if v == "" {
		return "", fmt.Errorf("%w: %s", ErrMissingHeader, name)
	}
	return v, nil
}

func timestampDate(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("20060102")
}

func timestampISO8601(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("20060102T150405Z")
}
