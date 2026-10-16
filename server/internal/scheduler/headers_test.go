package scheduler

import (
	"net/http"
	"testing"

	"github.com/gonotelm-lab/flow/api/signature"
	"github.com/stretchr/testify/assert"
)

// 这些字符串是接收方要照着实现的 wire contract：改名等于破坏兼容。
// 用字面量钉住，防止「顺手重命名」把契约改掉——算法本身的测试在 api/signature。
func TestHeaderNames_MatchWireContract(t *testing.T) {
	assert.Equal(t, "X-Flow-Schedule-Id", HeaderScheduleId)
	assert.Equal(t, "X-Flow-Fire-Id", HeaderFireId)
	assert.Equal(t, "X-Flow-Scheduled-Time", HeaderScheduledTime)
	assert.Equal(t, "X-Flow-Timestamp", HeaderTimestamp)
	assert.Equal(t, "X-Flow-Namespace", HeaderNamespace)
	assert.Equal(t, "X-Flow-Signature", HeaderSignature)
}

// 服务端要求接收方复算的签名范围必须与签名实现一致，不多不少。
func TestSignedHeaderNames_AreTheDocumentedSet(t *testing.T) {
	assert.Equal(t, []string{
		"content-type",
		"x-flow-fire-id",
		"x-flow-namespace",
		"x-flow-schedule-id",
		"x-flow-scheduled-time",
		"x-flow-timestamp",
	}, signature.SignedHeaderNames)
}

// 头名常量与 http.Header 的取值路径必须对得上（大小写不敏感查找）。
func TestHeaderNames_AreLookupSafe(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderNamespace, "demo")

	assert.Equal(t, "demo", h.Get(HeaderNamespace))
	assert.Equal(t, "demo", h.Get(signature.HeaderNamespace))
}
