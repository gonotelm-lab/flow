package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 用标准库独立重算一遍，确保签名规则与文档一致（而不是自证）。
func TestSign_MatchesDocumentedRule(t *testing.T) {
	secret := "sk-test-secret"
	const ts int64 = 1770000000
	body := []byte(`{"hello":"world"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	assert.Equal(t, want, Sign(secret, ts, body))
	assert.Equal(t, "sha256="+want, SignatureHeader(secret, ts, body))
}

func TestSign_IsDeterministic(t *testing.T) {
	body := []byte("payload")
	assert.Equal(t, Sign("k", 1, body), Sign("k", 1, body))
}

func TestSign_ChangesWithEveryInput(t *testing.T) {
	body := []byte("payload")
	base := Sign("k", 100, body)

	assert.NotEqual(t, base, Sign("k2", 100, body), "密钥变化必须改变签名")
	assert.NotEqual(t, base, Sign("k", 101, body), "时间戳变化必须改变签名")
	assert.NotEqual(t, base, Sign("k", 100, []byte("payload2")), "body 变化必须改变签名")
	assert.NotEqual(t, base, Sign("k", 100, []byte("payloa")), "body 少一个字节也必须改变签名")
	assert.NotEqual(t, base, Sign("k", 100, nil), "空 body 与有 body 必须不同")
}

// timestamp 参与签名的意义：接收方发现时间戳过旧即可拒收，防重放。
func TestSign_TimestampIsCovered(t *testing.T) {
	body := []byte("x")
	sig := SignatureHeader("secret", 1000, body)

	assert.True(t, Verify("secret", 1000, body, sig))
	assert.False(t, Verify("secret", 1001, body, sig), "换了时间戳，原签名必须失效")
}

func TestVerify(t *testing.T) {
	body := []byte("hello")
	sig := SignatureHeader("secret", 42, body)

	assert.True(t, Verify("secret", 42, body, sig))
	assert.False(t, Verify("wrong-secret", 42, body, sig))
	assert.False(t, Verify("secret", 42, []byte("hello!"), sig))
	assert.False(t, Verify("secret", 42, body, "sha256=deadbeef"))
	assert.False(t, Verify("secret", 42, body, ""))
	assert.False(t, Verify("secret", 42, body, sig+"x"), "长度不同也必须安全返回 false")
}
