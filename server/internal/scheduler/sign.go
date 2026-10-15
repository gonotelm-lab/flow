package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// 回调请求头。接收方按这些字段做校验与去重。
const (
	// HeaderScheduleId 本次回调所属的 schedule id
	HeaderScheduleId = "X-Flow-Schedule-Id"
	// HeaderFireId 本次 fire 的 id；同一次 fire 的所有重试与接管重投都不变，
	// 接收方必须用它做幂等去重——这是 at-least-once 契约的去重依据。
	HeaderFireId = "X-Flow-Fire-Id"
	// HeaderScheduledTime 本次触发计划时刻（unix 毫秒）
	HeaderScheduledTime = "X-Flow-Scheduled-Time"
	// HeaderTimestamp 签名时间戳（unix 秒），参与签名以防重放
	HeaderTimestamp = "X-Flow-Timestamp"
	// HeaderSignature 签名值，形如 sha256=<hex>
	HeaderSignature = "X-Flow-Signature"
)

// SignaturePrefix 是 X-Flow-Signature 的前缀。
const SignaturePrefix = "sha256="

// Sign 计算 HMAC-SHA256 签名值（不含前缀）。
//
// 密钥为 namespace 的 api_key，签名串为 `timestamp + "." + rawBody`。
// timestamp 必须参与签名，否则重放攻击无法被发现。
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignatureHeader 返回 X-Flow-Signature 头的完整值。
func SignatureHeader(secret string, timestamp int64, body []byte) string {
	return SignaturePrefix + Sign(secret, timestamp, body)
}

// Verify 按同一规则校验签名头。
//
// 这个函数是给「接收方」的参考实现（也用于测试）：Flow 只负责签名，不负责校验。
func Verify(secret string, timestamp int64, body []byte, header string) bool {
	return hmac.Equal([]byte(SignatureHeader(secret, timestamp, body)), []byte(header))
}
