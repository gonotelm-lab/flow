package scheduler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gonotelm-lab/flow/api/signature"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
)

// newCallbackRequest 把一次回调组装成**已经签好名**的 HTTP 请求。
//
// 单独拆出来是因为它是整个投递链路里唯一「契约错误就静默失败」的一步：头少一个、
// 签名范围不对，接收方只会一直验签失败，而调度器看到的是「对方 4xx」。放在这里
// 之后，它可以脱离数据库被直接测试（见 callback_test.go）。
func newCallbackRequest(
	ctx context.Context,
	secret string,
	s *schema.Schedule,
	now time.Time,
) (*http.Request, error) {
	// dispatch 已经把没有 fire id 的行过滤掉了，这里只是不让 nil 继续往下走。
	if s.FireId == nil {
		return nil, errors.New("schedule has no fire id")
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, s.CallbackUrl, bytes.NewReader(s.Payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", s.ContentType)
	req.Header.Set(HeaderScheduleId, s.Id.String())
	req.Header.Set(HeaderFireId, s.FireId.String())
	req.Header.Set(HeaderScheduledTime, strconv.FormatInt(s.FireScheduledTime, 10))

	// namespace 既告诉接收方该用哪把 api_key 验签，也参与签名 scope。
	req.Header.Set(HeaderNamespace, s.Namespace)

	// 时间戳参与签名，接收方据此判重放。
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(now.Unix(), 10))

	// 签名覆盖方法、URL、上面这些头与 body：接收方改任何一处都对不上。
	signed, err := signature.Sign(secret, signature.Input{
		Method: req.Method,
		URL:    req.URL,
		Header: req.Header,
		Body:   s.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("sign callback: %w", err)
	}
	req.Header.Set(HeaderSignature, signed)

	return req, nil
}

// drainAndClose 读掉一小段响应体再关闭，让连接可以被复用。
func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}
