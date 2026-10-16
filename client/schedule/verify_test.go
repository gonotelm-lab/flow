package schedule

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/api/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testApiKey     = "sk-test-secret"
	testNamespace  = "demo"
	testScheduleID = "018f2f1e-0000-7000-8000-000000000001"
	testFireID     = "018f2f1e-0000-7000-8000-000000000002"
	testBody       = `{"hello":"world"}`
)

// 固定时间戳便于断言，避免测试依赖「跑的时候正好几点」。
var testTimestamp = time.Date(2026, 2, 2, 2, 40, 0, 0, time.UTC)

// testClient 造一个只用于验签的客户端：不接连接，只带 api_key。
func testClient(apiKey string) *Client {
	return NewWithConn(nil, Config{APIKey: apiKey})
}

// testClientNS 带 namespace 的验签客户端：验签时会核对回调的 X-Flow-Namespace。
func testClientNS(namespace, apiKey string) *Client {
	return NewWithConn(nil, Config{Namespace: namespace, APIKey: apiKey})
}

// 接收方的文档用法必须真的可用：addr 留空（gRPC 懒连接，不会因为没服务端而失败）。
func TestNew_VerifyOnlyClientWithEmptyAddr(t *testing.T) {
	c, err := New("", Config{APIKey: testApiKey})
	require.NoError(t, err)
	require.NoError(t, c.Close())

	// 没配密钥时给出明确错误，而不是让调用方去猜 401 从哪来
	empty := testClient("")
	assert.ErrorIs(t, empty.VerifyRequest(&http.Request{}), ErrNoAPIKey)
}

// signCallback 按 flowserver 的做法把回调请求填齐并签名。
// 它直接调 api/signature.Sign——也就是说：测试里「服务端侧」的签名与
// 生产代码走的是同一份实现，这里验证的是「SDK 能不能验」。
func signCallback(t *testing.T, req *http.Request, secret, namespace string, body []byte, ts time.Time) {
	t.Helper()

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderScheduleID, testScheduleID)
	req.Header.Set(HeaderFireID, testFireID)
	req.Header.Set(HeaderScheduledTime, "1770000000000")
	req.Header.Set(HeaderNamespace, namespace)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))

	header, err := signature.Sign(secret, signature.Input{
		Method: req.Method,
		URL:    req.URL,
		Header: req.Header,
		Body:   body,
	})
	require.NoError(t, err)
	req.Header.Set(HeaderSignature, header)
}

// newCallback 造一个「接收方视角」的请求：body 可读、Content-Length 正确。
func newCallback(t *testing.T, target string, body []byte, secret string, ts time.Time) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	signCallback(t, req, secret, testNamespace, body, ts)
	return req
}

// atTime 把「现在」钉在回调时间戳上，让新鲜度校验可预期。
func atTime(ts time.Time) VerifyOption {
	return WithNow(func() time.Time { return ts })
}

// ---------------------------------------------------------------------------
// 主路径：真 HTTP 一跳，接收方一行调用完成验签
// ---------------------------------------------------------------------------

type hopResult struct {
	verifyErr error
	body      []byte
	bodyErr   error
}

func TestVerifyRequest_RealHTTPHop(t *testing.T) {
	results := make(chan hopResult, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := testClientNS(testNamespace, testApiKey).VerifyRequest(r, atTime(testTimestamp))

		// 验签之后 body 必须还能正常读——这是「便捷」的前提。
		body, bodyErr := io.ReadAll(r.Body)
		results <- hopResult{verifyErr: err, body: body, bodyErr: bodyErr}

		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body := []byte(testBody)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/hook?a=1&b=2", bytes.NewReader(body))
	require.NoError(t, err)
	signCallback(t, req, testApiKey, testNamespace, body, testTimestamp)

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	got := <-results
	require.NoError(t, got.verifyErr, "服务端签出来的回调必须验得过")
	assert.Equal(t, body, got.body, "验签后 body 必须被还原")
	require.NoError(t, got.bodyErr)
}

// ---------------------------------------------------------------------------
// 反例：改任何一处都必须失败
// ---------------------------------------------------------------------------

func TestVerifyRequest_RejectsTampering(t *testing.T) {
	now := testTimestamp
	base := func(t *testing.T) *http.Request {
		return newCallback(t, "/hook?a=1&b=2", []byte(testBody), testApiKey, testTimestamp)
	}

	t.Run("body 被改", func(t *testing.T) {
		req := base(t)
		req.Body = io.NopCloser(strings.NewReader(`{"hello":"evil"}`))
		req.ContentLength = int64(len(`{"hello":"evil"}`))

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("路径被改", func(t *testing.T) {
		req := base(t)
		req.URL.Path = "/evil"

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("查询串被改", func(t *testing.T) {
		req := base(t)
		req.URL.RawQuery = "a=1&b=3"

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("方法被改", func(t *testing.T) {
		req := base(t)
		req.Method = http.MethodGet

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("fire id 被改", func(t *testing.T) {
		req := base(t)
		req.Header.Set(HeaderFireID, "018f2f1e-0000-7000-8000-000000000009")

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("时间戳被改", func(t *testing.T) {
		req := base(t)
		req.Header.Set(HeaderTimestamp, strconv.FormatInt(now.Add(time.Second).Unix(), 10))

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("secret 不对", func(t *testing.T) {
		req := base(t)

		assert.ErrorIs(t, testClient("sk-wrong").VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})

	t.Run("空 body 与有 body 不等价", func(t *testing.T) {
		req := base(t)
		req.Body = http.NoBody
		req.ContentLength = 0

		assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(now)), signature.ErrMismatch)
	})
}

// 旧的一行式签名（HMAC(secret, ts+"."+body)，形如 sha256=<hex>）必须被明确拒绝，
// 不能因为「碰巧像」而被接受。
func TestVerifyRequest_RejectsLegacyScheme(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)
	req.Header.Set(HeaderSignature, "sha256=deadbeef")

	err := testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp))
	assert.ErrorIs(t, err, signature.ErrMalformedSignature)
}

func TestVerifyRequest_RejectsMissingSignature(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)
	req.Header.Del(HeaderSignature)

	assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp)), signature.ErrMissingHeader)
}

func TestVerifyRequest_NilRequest(t *testing.T) {
	assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(nil), ErrNilRequest)
	assert.ErrorIs(t, testClient(testApiKey).VerifyRequestWithBody(nil, nil), ErrNilRequest)
}

// ---------------------------------------------------------------------------
// 时效性
// ---------------------------------------------------------------------------

func TestVerifyRequest_TimestampTolerance(t *testing.T) {
	ts := testTimestamp
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, ts)

	tests := map[string]struct {
		now  time.Time
		want error
	}{
		"同一时刻":           {ts, nil},
		"过去 4 分钟（缺省容差内）": {ts.Add(4 * time.Minute), nil},
		"未来 4 分钟（缺省容差内）": {ts.Add(-4 * time.Minute), nil},
		"过去 6 分钟":        {ts.Add(6 * time.Minute), ErrStaleTimestamp},
		"未来 6 分钟":        {ts.Add(-6 * time.Minute), ErrStaleTimestamp},
		"一天前的重放":         {ts.Add(24 * time.Hour), ErrStaleTimestamp},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := testClient(testApiKey).VerifyRequest(req, atTime(tc.now))
			if tc.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestVerifyRequest_ToleranceCanBeNarrowedOrDisabled(t *testing.T) {
	ts := testTimestamp
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, ts)

	// 放宽到 10 分钟：原本会过期的请求变成可接受。
	assert.NoError(t, testClient(testApiKey).VerifyRequest(req,
		atTime(ts.Add(6*time.Minute)), WithTolerance(10*time.Minute)))

	// 收紧到 1 分钟：原本可接受的变成过期。
	assert.ErrorIs(t, testClient(testApiKey).VerifyRequest(req,
		atTime(ts.Add(2*time.Minute)), WithTolerance(time.Minute)), ErrStaleTimestamp)

	// 传 0 表示关掉新鲜度校验，只验签名。
	assert.NoError(t, testClient(testApiKey).VerifyRequest(req,
		atTime(ts.Add(24*time.Hour)), WithTolerance(0)))
}

// 时间戳非法时验签必须失败（fail closed），不能因为「算不出时间」就放行。
func TestVerifyRequest_UnparseableTimestampFailsClosed(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)
	req.Header.Set(HeaderTimestamp, "not-a-number")

	err := testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp))
	assert.ErrorIs(t, err, signature.ErrMalformedSignature)
}

// ---------------------------------------------------------------------------
// namespace 断言
// ---------------------------------------------------------------------------

// namespace 断言来自 Config，不是每次调用的选项。
func TestVerifyRequest_ConfigNamespaceMustMatch(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)

	assert.NoError(t, testClientNS(testNamespace, testApiKey).VerifyRequest(req, atTime(testTimestamp)))

	err := testClientNS("other", testApiKey).VerifyRequest(req, atTime(testTimestamp))
	assert.ErrorIs(t, err, signature.ErrScopeMismatch)

	// 没配 namespace 的客户端不做这项断言
	assert.NoError(t, testClient(testApiKey).VerifyRequest(
		newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp), atTime(testTimestamp)))
}

// namespace 只校验非空，带逗号/空格的也必须能签能验（scope 会编码）。
func TestVerifyRequest_NamespaceWithSeparatorCharacters(t *testing.T) {
	const namespace = "demo, prod"

	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader([]byte(testBody)))
	signCallback(t, req, testApiKey, namespace, []byte(testBody), testTimestamp)

	assert.NoError(t, testClientNS(namespace, testApiKey).VerifyRequest(req, atTime(testTimestamp)))
	assert.ErrorIs(t,
		testClientNS("demo", testApiKey).VerifyRequest(req, atTime(testTimestamp)),
		signature.ErrScopeMismatch)
}

// ---------------------------------------------------------------------------
// body 处理
// ---------------------------------------------------------------------------

func TestVerifyRequest_BodyTooLarge(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 2048)

	t.Run("Content-Length 已知时直接拒绝", func(t *testing.T) {
		req := newCallback(t, "/hook", body, testApiKey, testTimestamp)

		err := testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp), WithMaxBodySize(1024))
		assert.ErrorIs(t, err, ErrBodyTooLarge)
		assert.Equal(t, int64(len(body)), req.ContentLength, "不该消费掉 body")
	})

	t.Run("Content-Length 未知时读一半就停", func(t *testing.T) {
		req := newCallback(t, "/hook", body, testApiKey, testTimestamp)
		req.ContentLength = -1 // 分块传输的形态

		err := testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp), WithMaxBodySize(1024))
		assert.ErrorIs(t, err, ErrBodyTooLarge)
	})

	t.Run("上限内的 body 正常通过", func(t *testing.T) {
		req := newCallback(t, "/hook", body, testApiKey, testTimestamp)

		assert.NoError(t, testClient(testApiKey).VerifyRequest(req,
			atTime(testTimestamp), WithMaxBodySize(int64(len(body)))))
	})
}

func TestVerifyRequest_EmptyBody(t *testing.T) {
	req := newCallback(t, "/hook", nil, testApiKey, testTimestamp)

	assert.NoError(t, testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp)))
	assert.Equal(t, http.NoBody, req.Body)
}

// 调用方自己读的 body 走 WithBody 版本，且这个版本不能动 r.Body。
func TestVerifyRequestWithBody_DoesNotTouchRequestBody(t *testing.T) {
	body := []byte(testBody)
	req := newCallback(t, "/hook", body, testApiKey, testTimestamp)

	assert.NoError(t, testClient(testApiKey).VerifyRequestWithBody(req, body, atTime(testTimestamp)))

	rest, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, body, rest, "WithBody 版本必须原样保留请求体")

	// body 与请求体不一致时同样要失败。
	assert.ErrorIs(t,
		testClient(testApiKey).VerifyRequestWithBody(req, []byte("other"), atTime(testTimestamp)),
		signature.ErrMismatch)
}

// 验签失败时也要把 body 放回去：调用方可能想把原始报文记进日志再排查。
func TestVerifyRequest_RestoresBodyEvenOnFailure(t *testing.T) {
	body := []byte(testBody)
	req := newCallback(t, "/hook", body, testApiKey, testTimestamp)

	require.Error(t, testClient("sk-wrong").VerifyRequest(req, atTime(testTimestamp)))

	rest, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, body, rest)
}

// 便捷入口不该依赖调用方先建好 Content-Length / Content-Type。
func TestVerifyRequest_WorksWithoutContentLengthHint(t *testing.T) {
	body := []byte(testBody)
	req := newCallback(t, "/hook", body, testApiKey, testTimestamp)
	req.ContentLength = -1

	assert.NoError(t, testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp)))
}

func TestVerifyRequest_BadOptionsAreIgnored(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)

	// 0/负数 body 上限回落到缺省值；nil 的 now 回落到 time.Now。
	assert.NoError(t, testClient(testApiKey).VerifyRequest(req,
		atTime(testTimestamp), WithMaxBodySize(0), WithNow(nil)))
}

func TestNewVerifyOptions_Defaults(t *testing.T) {
	o := newVerifyOptions(nil)
	assert.Equal(t, DefaultTolerance, o.tolerance)
	assert.Equal(t, int64(DefaultMaxBodySize), o.maxBodySize)
	require.NotNil(t, o.now)

	o = newVerifyOptions([]VerifyOption{WithTolerance(-1), WithMaxBodySize(64)})
	assert.Equal(t, -time.Nanosecond, o.tolerance)
	assert.Equal(t, int64(64), o.maxBodySize)
}

func TestVerifyRequest_ErrorsAreClassifiable(t *testing.T) {
	req := newCallback(t, "/hook", []byte(testBody), testApiKey, testTimestamp)
	req.Header.Del(HeaderSignature)

	err := testClient(testApiKey).VerifyRequest(req, atTime(testTimestamp))
	require.Error(t, err)
	assert.True(t, errors.Is(err, signature.ErrMissingHeader))
	assert.False(t, errors.Is(err, signature.ErrMismatch))
}
