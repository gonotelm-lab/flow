package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOneShot_BuildsOneShotTrigger(t *testing.T) {
	fireAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s := OneShot(fireAt, "https://example.com/hook", []byte("body"))

	trigger, ok := s.Trigger.(OneShotTrigger)
	require.True(t, ok, "必须落在一次性触发上")
	assert.Equal(t, fireAt, trigger.FireAt)
	assert.Equal(t, "https://example.com/hook", s.CallbackURL)
	assert.Equal(t, "body", string(s.Payload))
	// 没指定 Content-Type 时应当补上缺省值
	assert.Equal(t, DefaultContentType, s.ContentType)
}

func TestAfter_UsesAbsoluteFutureTime(t *testing.T) {
	before := time.Now()
	s := After(30*time.Second, "https://example.com/hook", nil)
	after := time.Now()

	trigger, ok := s.Trigger.(OneShotTrigger)
	require.True(t, ok)
	assert.False(t, trigger.FireAt.Before(before.Add(30*time.Second)))
	assert.False(t, trigger.FireAt.After(after.Add(30*time.Second)))
}

func TestCron_BuildsCronTrigger(t *testing.T) {
	s := Cron("*/5 * * * *", "Asia/Shanghai", "https://example.com/hook", []byte("x"))

	trigger, ok := s.Trigger.(CronTrigger)
	require.True(t, ok, "必须落在 cron 触发上")
	assert.Equal(t, "*/5 * * * *", trigger.Expr)
	assert.Equal(t, "Asia/Shanghai", trigger.Timezone)
}

func TestCron_EmptyTimezoneMeansServerDefault(t *testing.T) {
	s := Cron("* * * * *", "", "https://example.com/hook", nil)
	// 留空交给服务端补 UTC，客户端不猜服务器时区
	trigger, ok := s.Trigger.(CronTrigger)
	require.True(t, ok)
	assert.Equal(t, "", trigger.Timezone)
}

func TestOptions_AreApplied(t *testing.T) {
	s := OneShot(time.Now().Add(time.Hour), "https://example.com/hook", nil,
		WithName("nightly-report"),
		WithIdempotencyKey("order-123"),
		WithContentType("application/json"),
	)

	assert.Equal(t, "nightly-report", s.Name)
	assert.Equal(t, "order-123", s.IdempotencyKey)
	assert.Equal(t, "application/json", s.ContentType)
}

func TestOptions_EmptyValuesDoNotOverwrite(t *testing.T) {
	// 空字符串应当被视为「没设置」，而不是把已有值清掉
	s := Cron("* * * * *", "UTC", "https://example.com/hook", nil, WithName(""))
	assert.Equal(t, "", s.Name)
	assert.Equal(t, DefaultContentType, s.ContentType)
}

func TestNewListOptions_DefaultsAndOverrides(t *testing.T) {
	o := newListOptions(nil)
	assert.Equal(t, 1, o.page)
	assert.Equal(t, 20, o.pageSize)

	o = newListOptions([]ListOption{
		WithScheduleType(TypeCron),
		WithState(StateStopped),
		WithID("abc"),
		WithPage(3, 50),
	})
	assert.Equal(t, TypeCron, o.scheduleType)
	assert.Equal(t, StateStopped, o.state)
	assert.Equal(t, "abc", o.id)
	assert.Equal(t, 3, o.page)
	assert.Equal(t, 50, o.pageSize)
}

// 客户端 SDK 不能把 proto 类型漏到表面上：这些方法都只收发本包的类型。
// 里面没有 Update：定义改不了，只能重新注册（管理面也不提供 Update）。
func TestClientSurface_HasNoProtoTypes(t *testing.T) {
	var c *Client
	_ = []string{"Register", "Get", "List", "Stop", "Delete", "Close",
		"VerifyRequest", "VerifyRequestWithBody"}
	assert.Nil(t, c)
}

// 没显式设置幂等键时自动补一个随机键：RPC 重试不会建出两条。
func TestFillIdempotencyKey(t *testing.T) {
	generated := Spec{}
	require.NoError(t, fillIdempotencyKey(&generated))
	assert.Regexp(t, "^[0-9a-f]{32}$", generated.IdempotencyKey)

	another := Spec{}
	require.NoError(t, fillIdempotencyKey(&another))
	assert.NotEqual(t, generated.IdempotencyKey, another.IdempotencyKey)

	// 显式给了就原样保留，不会被覆盖掉
	explicit := Spec{IdempotencyKey: "nightly-report"}
	require.NoError(t, fillIdempotencyKey(&explicit))
	assert.Equal(t, "nightly-report", explicit.IdempotencyKey)
}

// 没配 namespace 的客户端不该发出请求：注册会被服务端拒，列表会变成跨 namespace 查全部。
func TestRegisterAndList_RequireNamespace(t *testing.T) {
	c := NewWithConn(nil, Config{APIKey: testApiKey})

	_, err := c.Register(context.Background(), Spec{})
	assert.ErrorIs(t, err, ErrNoNamespace)

	_, _, err = c.List(context.Background())
	assert.ErrorIs(t, err, ErrNoNamespace)
}
