package schedule

import (
	"testing"

	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 回调 URL 校验
//
// 注意这里刻意**没有**「拦截私网地址」「拦截云 metadata」这类用例——不是漏了，而是
// 按设计明确不做（见设计文档「已接受的残余风险」）。这些用例把「只校验格式」这条
// 契约钉住，避免以后有人顺手加黑名单却没同步改文档。
// ---------------------------------------------------------------------------

func TestValidateCallbackUrl_AcceptsHTTPAndHTTPS(t *testing.T) {
	for _, raw := range []string{
		"http://example.com/hook",
		"https://example.com/hook",
		"https://example.com:8443/a/b?c=d",
		"http://10.0.0.5:8080/internal",
		"http://localhost:3000/cb",
	} {
		t.Run(raw, func(t *testing.T) {
			assert.NoError(t, ValidateCallbackUrl(raw))
		})
	}
}

func TestValidateCallbackUrl_RejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"空":         "",
		"非 http 协议": "ftp://example.com/hook",
		"file 协议":   "file:///etc/passwd",
		"gopher 协议": "gopher://example.com",
		"没有 host":   "http:///hook",
		"内嵌凭据":      "http://user:pass@example.com/hook",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateCallbackUrl(raw))
		})
	}
}

// 私网与 metadata 地址按设计放行 —— 这是「只校验格式」的直接体现。
func TestValidateCallbackUrl_AllowsPrivateAndMetadataAddresses(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:6379/",
		"http://192.168.1.1/admin",
		"http://[fd00:ec2::254]/",
	} {
		assert.NoError(t, ValidateCallbackUrl(raw),
			"%s 按设计应当放行（私网/metadata 不拦截）", raw)
	}
}

// ---------------------------------------------------------------------------
// 幂等键归一化
//
// 这是集成测试抓出来的那个真 bug 的单元级防线：空键必须以 nil 入库，
// 否则部分唯一索引会把所有「没传键」的 schedule 当成同一个键。
// ---------------------------------------------------------------------------

func TestNormalizeIdempotencyKey(t *testing.T) {
	t.Run("nil 保持 nil", func(t *testing.T) {
		assert.Nil(t, normalizeIdempotencyKey(nil))
	})

	t.Run("空串与纯空白都归一成 nil", func(t *testing.T) {
		empty := ""
		assert.Nil(t, normalizeIdempotencyKey(&empty))

		blank := "   \t\n"
		assert.Nil(t, normalizeIdempotencyKey(&blank))
	})

	t.Run("有值时去掉首尾空白", func(t *testing.T) {
		raw := "  order-123  "
		got := normalizeIdempotencyKey(&raw)
		require.NotNil(t, got)
		assert.Equal(t, "order-123", *got)
	})
}

func TestIdempotencyKeyOf(t *testing.T) {
	assert.Equal(t, "", IdempotencyKeyOf(nil))
	assert.Equal(t, "", IdempotencyKeyOf(&schema.Schedule{}))

	key := "k1"
	assert.Equal(t, "k1", IdempotencyKeyOf(&schema.Schedule{IdempotencyKey: &key}))
}

// ---------------------------------------------------------------------------
// proto <-> 存储模型映射
// ---------------------------------------------------------------------------

func TestMapper_OneShotRoundTrip(t *testing.T) {
	fireAt := int64(1_800_000_000_000)

	proto := &schemav1.Schedule{
		Namespace:   "ns",
		Name:        "one-shot",
		Trigger:     &schemav1.Schedule_OneShot{OneShot: &schemav1.OneShotTrigger{FireAt: fireAt}},
		CallbackUrl: "https://example.com/hook",
		ContentType: "application/json",
		Payload:     []byte("body"),
	}

	domain := FromProtoSchedule(proto)
	require.NotNil(t, domain)
	assert.Equal(t, schema.ScheduleTypeOneShot, domain.ScheduleType)
	assert.Equal(t, fireAt, domain.FireAt)
	assert.Equal(t, "", domain.CronExpr, "一次性任务不该带 cron 表达式")

	// 回程：oneof 分支必须还原
	back := ToProtoSchedule(&schema.Schedule{
		Namespace:    domain.Namespace,
		ScheduleType: domain.ScheduleType,
		FireAt:       domain.FireAt,
		CallbackUrl:  domain.CallbackUrl,
		ContentType:  domain.ContentType,
		Payload:      domain.Payload,
		State:        schema.ScheduleStateActive,
	})
	require.NotNil(t, back)
	require.NotNil(t, back.GetOneShot())
	assert.Equal(t, fireAt, back.GetOneShot().FireAt)
	assert.Nil(t, back.GetCron())
	assert.Equal(t, schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE, back.State)
}

func TestMapper_CronRoundTrip(t *testing.T) {
	proto := &schemav1.Schedule{
		Namespace: "ns",
		Trigger: &schemav1.Schedule_Cron{
			Cron: &schemav1.CronTrigger{Expr: "*/5 * * * *", Timezone: "Asia/Shanghai"},
		},
		CallbackUrl: "https://example.com/hook",
	}

	domain := FromProtoSchedule(proto)
	require.NotNil(t, domain)
	assert.Equal(t, schema.ScheduleTypeCron, domain.ScheduleType)
	assert.Equal(t, "*/5 * * * *", domain.CronExpr)
	assert.Equal(t, "Asia/Shanghai", domain.Timezone)
	assert.Zero(t, domain.FireAt, "cron 不该带 fire_at")

	back := ToProtoSchedule(&schema.Schedule{
		ScheduleType: schema.ScheduleTypeCron,
		CronExpr:     domain.CronExpr,
		Timezone:     domain.Timezone,
		State:        schema.ScheduleStateStopped,
	})
	require.NotNil(t, back)
	require.NotNil(t, back.GetCron())
	assert.Equal(t, "*/5 * * * *", back.GetCron().Expr)
	assert.Equal(t, "Asia/Shanghai", back.GetCron().Timezone)
	assert.Nil(t, back.GetOneShot())
	assert.Equal(t, schemav1.ScheduleState_SCHEDULE_STATE_STOPPED, back.State)
}

// 未知枚举值必须报错，不能在查表失败时翻成空串：空串在下游是「不筛选」，
// 会把一个拼错的状态悄悄变成「查全部」。
func TestMapper_RejectsUnknownEnumValues(t *testing.T) {
	_, err := ScheduleStateFromProto(schemav1.ScheduleState(99))
	require.Error(t, err)

	_, err = ScheduleTypeFromProto(schemav1.ScheduleType(99))
	require.Error(t, err)

	_, err = DeadLetterStateFromProto(schemav1.DeadLetterState(99))
	require.Error(t, err)

	// UNSPECIFIED 是合法的「不筛选」，翻成空串
	state, err := ScheduleStateFromProto(schemav1.ScheduleState_SCHEDULE_STATE_UNSPECIFIED)
	require.NoError(t, err)
	assert.Empty(t, state)
}

// 未指定触发类型时不能猜：必须落成空类型，由 Core 的校验拒掉。
func TestMapper_NoTriggerLeavesTypeUnset(t *testing.T) {
	domain := FromProtoSchedule(&schemav1.Schedule{Namespace: "ns"})
	require.NotNil(t, domain)
	assert.Equal(t, "", domain.ScheduleType)
}

// 运行态字段不接受调用方写入。
func TestMapper_IgnoresRuntimeFields(t *testing.T) {
	proto := &schemav1.Schedule{
		Namespace:           "ns",
		Trigger:             &schemav1.Schedule_Cron{Cron: &schemav1.CronTrigger{Expr: "* * * * *"}},
		State:               schemav1.ScheduleState_SCHEDULE_STATE_FINISHED,
		NextFireAt:          12345,
		AttemptNo:           7,
		ConsecutiveFailures: 9,
		SkippedCount:        42,
		LastError:           "injected",
	}

	domain := FromProtoSchedule(proto)
	require.NotNil(t, domain)
	assert.Equal(t, "", domain.State, "state 由调度器维护，不能由调用方写入")
	assert.Zero(t, domain.NextFireAt)
	assert.Zero(t, domain.AttemptNo)
	assert.Zero(t, domain.ConsecutiveFailures)
	assert.Zero(t, domain.SkippedCount)
	assert.Equal(t, "", domain.LastError)
}

// 槽位空闲（fire_state 为 nil）时对外表达为 UNSPECIFIED。
func TestMapper_FireStateUnspecifiedWhenIdle(t *testing.T) {
	back := ToProtoSchedule(&schema.Schedule{
		ScheduleType: schema.ScheduleTypeCron,
		CronExpr:     "* * * * *",
		State:        schema.ScheduleStateActive,
	})
	require.NotNil(t, back)
	assert.Equal(t, schemav1.FireState_FIRE_STATE_UNSPECIFIED, back.FireState)

	busy := schema.FireStateDelivering
	back = ToProtoSchedule(&schema.Schedule{
		ScheduleType: schema.ScheduleTypeCron,
		CronExpr:     "* * * * *",
		State:        schema.ScheduleStateActive,
		FireState:    &busy,
	})
	assert.Equal(t, schemav1.FireState_FIRE_STATE_DELIVERING, back.FireState)
}

func TestMapper_NilSafe(t *testing.T) {
	assert.Nil(t, ToProtoSchedule(nil))
	assert.Nil(t, FromProtoSchedule(nil))
}

func TestMapper_DeadLetterStateMapping(t *testing.T) {
	pairs := map[string]schemav1.DeadLetterState{
		schema.DeadLetterStateRetrying:  schemav1.DeadLetterState_DEAD_LETTER_STATE_RETRYING,
		schema.DeadLetterStateExhausted: schemav1.DeadLetterState_DEAD_LETTER_STATE_EXHAUSTED,
		schema.DeadLetterStateReplayed:  schemav1.DeadLetterState_DEAD_LETTER_STATE_REPLAYED,
		schema.DeadLetterStateStopped:   schemav1.DeadLetterState_DEAD_LETTER_STATE_STOPPED,
	}

	for domain, proto := range pairs {
		assert.Equal(t, proto, DeadLetterStateToProto(domain))

		back, err := DeadLetterStateFromProto(proto)
		require.NoError(t, err)
		assert.Equal(t, domain, back)
	}
}

func TestMapper_ListFilterMapping(t *testing.T) {
	state, err := ScheduleStateFromProto(schemav1.ScheduleState_SCHEDULE_STATE_ACTIVE)
	require.NoError(t, err)
	assert.Equal(t, schema.ScheduleStateActive, state)

	scheduleType, err := ScheduleTypeFromProto(schemav1.ScheduleType_SCHEDULE_TYPE_CRON)
	require.NoError(t, err)
	assert.Equal(t, schema.ScheduleTypeCron, scheduleType)

	// 未指定（0）不应当变成某个具体状态，否则筛选会莫名其妙少一半数据
	state, err = ScheduleStateFromProto(schemav1.ScheduleState_SCHEDULE_STATE_UNSPECIFIED)
	require.NoError(t, err)
	assert.Equal(t, "", state)

	scheduleType, err = ScheduleTypeFromProto(schemav1.ScheduleType_SCHEDULE_TYPE_UNSPECIFIED)
	require.NoError(t, err)
	assert.Equal(t, "", scheduleType)
}

func TestPaging(t *testing.T) {
	offset, limit := paging(1, 20)
	assert.Equal(t, 0, offset)
	assert.Equal(t, 20, limit)

	offset, limit = paging(3, 10)
	assert.Equal(t, 20, offset)
	assert.Equal(t, 10, limit)

	// 非法输入归一化
	offset, limit = paging(0, 0)
	assert.Equal(t, 0, offset)
	assert.Equal(t, 20, limit)

	// 上限保护
	offset, limit = paging(1, 1000)
	assert.Equal(t, 0, offset)
	assert.Equal(t, 100, limit)
}
