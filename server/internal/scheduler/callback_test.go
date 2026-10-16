package scheduler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gonotelm-lab/flow/api/signature"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这一组测试不碰数据库：调度器「怎么组请求、怎么签」是回调契约的发送侧，
// 必须能在没有 Postgres 的环境里跑（端到端用例在没配 FLOW_DB_* 时会 skip）。

func testSchedule() *schema.Schedule {
	fireID := uuid.MustParse("018f2f1e-0000-7000-8000-0000000000f1")
	return &schema.Schedule{
		Id:                uuid.MustParse("018f2f1e-0000-7000-8000-0000000000a1"),
		Namespace:         "demo",
		CallbackUrl:       "https://receiver.example.com/hook?a=1&b=2",
		ContentType:       "application/json",
		Payload:           []byte(`{"hello":"world"}`),
		FireId:            &fireID,
		FireScheduledTime: 1770000000000,
	}
}

func TestNewCallbackRequest_IsVerifiableByReceiver(t *testing.T) {
	now := time.Unix(1770000000, 0).UTC()
	req, err := newCallbackRequest(context.Background(), "sk-test-secret", testSchedule(), now)
	require.NoError(t, err)

	// 接收方要看的头一个都不能少。
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "018f2f1e-0000-7000-8000-0000000000a1", req.Header.Get(HeaderScheduleId))
	assert.Equal(t, "018f2f1e-0000-7000-8000-0000000000f1", req.Header.Get(HeaderFireId))
	assert.Equal(t, "1770000000000", req.Header.Get(HeaderScheduledTime))
	assert.Equal(t, "demo", req.Header.Get(HeaderNamespace))
	assert.Equal(t, "1770000000", req.Header.Get(HeaderTimestamp))
	assert.Contains(t, req.Header.Get(HeaderSignature), signature.Algorithm+" Scope=")

	// body 必须是 payload 原样。
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"hello":"world"}`, string(body))

	// 接收方复算必须通过——用的是 SDK 那份实现。
	assert.NoError(t, signature.Verify("sk-test-secret", signature.Input{
		Method: req.Method,
		URL:    req.URL,
		Header: req.Header,
		Body:   []byte(`{"hello":"world"}`),
	}, req.Header.Get(HeaderSignature)))
}

func TestNewCallbackRequest_SignatureCoversTheRequestShape(t *testing.T) {
	req, err := newCallbackRequest(context.Background(), "sk-test-secret", testSchedule(), time.Unix(1770000000, 0))
	require.NoError(t, err)
	signed := req.Header.Get(HeaderSignature)

	base := signature.Input{
		Method: req.Method,
		URL:    req.URL,
		Header: req.Header,
		Body:   []byte(`{"hello":"world"}`),
	}

	t.Run("secret 不对", func(t *testing.T) {
		assert.ErrorIs(t, signature.Verify("sk-other", base, signed), signature.ErrMismatch)
	})
	t.Run("body 被改", func(t *testing.T) {
		evil := base
		evil.Body = []byte(`{"hello":"evil"}`)
		assert.ErrorIs(t, signature.Verify("sk-test-secret", evil, signed), signature.ErrMismatch)
	})
	t.Run("路径被改", func(t *testing.T) {
		evil := base
		evil.URL, _ = url.Parse("https://receiver.example.com/evil?a=1&b=2")
		assert.ErrorIs(t, signature.Verify("sk-test-secret", evil, signed), signature.ErrMismatch)
	})
	t.Run("查询串被改", func(t *testing.T) {
		evil := base
		evil.URL, _ = url.Parse("https://receiver.example.com/hook?a=1&b=3")
		assert.ErrorIs(t, signature.Verify("sk-test-secret", evil, signed), signature.ErrMismatch)
	})
	t.Run("换 fire id", func(t *testing.T) {
		evil := base
		evil.Header = req.Header.Clone()
		evil.Header.Set(HeaderFireId, uuid.NewString())
		assert.ErrorIs(t, signature.Verify("sk-test-secret", evil, signed), signature.ErrMismatch)
	})
}

// 没有 fire id 是编程错误：必须报错，而不是签一个没有去重依据的请求出去。
func TestNewCallbackRequest_RequiresFireID(t *testing.T) {
	s := testSchedule()
	s.FireId = nil

	_, err := newCallbackRequest(context.Background(), "sk", s, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fire id")
}

// namespace 缺失时签名会失败（它是 scope 的一部分），投递侧记成失败重试。
func TestNewCallbackRequest_RequiresNamespace(t *testing.T) {
	s := testSchedule()
	s.Namespace = ""

	_, err := newCallbackRequest(context.Background(), "sk", s, time.Now())
	assert.ErrorIs(t, err, signature.ErrMissingHeader, "外层包装必须保留错误链")
}
