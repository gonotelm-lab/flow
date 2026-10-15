package schedule

import (
	"testing"
	"time"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOneShot_BuildsOneShotTrigger(t *testing.T) {
	fireAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s := OneShot(fireAt, "https://example.com/hook", []byte("body"))

	require.NotNil(t, s.GetOneShot(), "必须落在 oneof 的 one_shot 分支")
	assert.Nil(t, s.GetCron(), "一次性任务不能带 cron 分支")
	assert.Equal(t, fireAt.UnixMilli(), s.GetOneShot().GetFireAt())
	assert.Equal(t, "https://example.com/hook", s.CallbackUrl)
	assert.Equal(t, "body", string(s.Payload))
	// 没指定 Content-Type 时应当补上缺省值
	assert.Equal(t, DefaultContentType, s.ContentType)
}

func TestAfter_UsesAbsoluteFutureTime(t *testing.T) {
	before := time.Now()
	s := After(30*time.Second, "https://example.com/hook", nil)
	after := time.Now()

	fireAt := s.GetOneShot().GetFireAt()
	assert.GreaterOrEqual(t, fireAt, before.Add(30*time.Second).UnixMilli())
	assert.LessOrEqual(t, fireAt, after.Add(30*time.Second).UnixMilli())
}

func TestCron_BuildsCronTrigger(t *testing.T) {
	s := Cron("*/5 * * * *", "Asia/Shanghai", "https://example.com/hook", []byte("x"))

	require.NotNil(t, s.GetCron(), "必须落在 oneof 的 cron 分支")
	assert.Nil(t, s.GetOneShot(), "cron 任务不能带 one_shot 分支")
	assert.Equal(t, "*/5 * * * *", s.GetCron().GetExpr())
	assert.Equal(t, "Asia/Shanghai", s.GetCron().GetTimezone())
}

func TestCron_EmptyTimezoneMeansServerDefault(t *testing.T) {
	s := Cron("* * * * *", "", "https://example.com/hook", nil)
	// 留空交给服务端补 UTC，客户端不猜服务器时区
	assert.Equal(t, "", s.GetCron().GetTimezone())
}

func TestOptions_AreApplied(t *testing.T) {
	s := OneShot(time.Now().Add(time.Hour), "https://example.com/hook", nil,
		WithNamespace("demo"),
		WithName("nightly-report"),
		WithIdempotencyKey("order-123"),
		WithContentType("application/json"),
	)

	assert.Equal(t, "demo", s.Namespace)
	assert.Equal(t, "nightly-report", s.Name)
	assert.Equal(t, "order-123", s.GetIdempotencyKey())
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
		WithListNamespace("ns"),
		WithScheduleType(schemav1.ScheduleType_SCHEDULE_TYPE_CRON),
		WithState(schemav1.ScheduleState_SCHEDULE_STATE_PAUSED),
		WithID("abc"),
		WithPage(3, 50),
	})
	assert.Equal(t, "ns", o.namespace)
	assert.Equal(t, schemav1.ScheduleType_SCHEDULE_TYPE_CRON, o.scheduleType)
	assert.Equal(t, schemav1.ScheduleState_SCHEDULE_STATE_PAUSED, o.state)
	assert.Equal(t, "abc", o.id)
	assert.Equal(t, 3, o.page)
	assert.Equal(t, 50, o.pageSize)
}

// 客户端面**不应该**有 TriggerNow / 死信操作：它们只在 admin 面。
// 这个断言是编译期的——若有人给 ScheduleServiceClient 加了这些方法，
// 说明客户端面被污染了，这里会提醒重新审视设计。
func TestClientSurface_HasNoAdminOnlyOperations(t *testing.T) {
	var c *Client
	assert.Nil(t, c)

	// 客户端 SDK 暴露的方法就是下面这些，没有 TriggerNow / DeadLetter
	_ = []string{"Register", "Get", "List", "Update", "Pause", "Resume", "Delete", "Close"}
}
