package interceptor

import (
	"testing"

	schedulev1 "github.com/gonotelm-lab/flow/api/schedule/v1"
	schemav1 "github.com/gonotelm-lab/flow/api/schema/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// 这一组测试守的是服务端 buf.validate 拦截器这一层。
//
// 起因是一个真实事故：Schedule.id 原本只有 `string.uuid = true`，而 proto3 的非 optional
// 字符串默认值就是 ""，protovalidate 对空值也会跑 uuid 校验，于是每一次注册都被拦截器
// 判成 InvalidArgument，客户端根本没法注册。
//
// 仓储测试绕过了 service 层，dispatcher 端到端测试绕过了 gRPC 层，两边都测不到它。
// 所以这里直接对着请求形状做校验，作为回归防线。

func registerShapedSchedule() *schemav1.Schedule {
	return &schemav1.Schedule{
		Namespace:   "demo",
		Name:        "nightly",
		CallbackUrl: "https://example.com/hook",
		ContentType: "application/json",
		Payload:     []byte(`{"a":1}`),
		Trigger: &schemav1.Schedule_Cron{
			Cron: &schemav1.CronTrigger{Expr: "*/5 * * * *", Timezone: "Asia/Shanghai"},
		},
	}
}

// 回归：客户端注册时 id 必然为空，必须放行。
func TestValidate_ScheduleWithoutIDIsAccepted(t *testing.T) {
	s := registerShapedSchedule()
	require.Empty(t, s.GetId(), "前提：注册请求里 id 为空")

	require.NoError(t, validator.Validate(s),
		"id 为空是注册的正常形状，不能被 uuid 校验拦下")
}

// 套上请求外壳再校验一遍，确认嵌套校验路径也放行。
func TestValidate_RegisterAndCreateRequestShapes(t *testing.T) {
	s := registerShapedSchedule()

	t.Run("api.schedule.v1.RegisterRequest", func(t *testing.T) {
		req := &schedulev1.RegisterRequest{Schedule: s}
		assert.NoError(t, validator.Validate(proto.Clone(req).(*schedulev1.RegisterRequest)))
	})
}

// 留空跳过，但一旦提供了就必须是合法 UUID。
func TestValidate_ScheduleIDWhenProvidedMustBeUUID(t *testing.T) {
	t.Run("合法 UUID 放行", func(t *testing.T) {
		s := registerShapedSchedule()
		s.Id = uuid.NewString()
		assert.NoError(t, validator.Validate(s))
	})

	t.Run("乱填的 id 被拒", func(t *testing.T) {
		s := registerShapedSchedule()
		s.Id = "not-a-uuid"
		assert.Error(t, validator.Validate(s))
	})
}

// 必填和边界约束仍然要生效，别把放行空 id 做成整体不校验。
func TestValidate_ScheduleRequiredFieldsStillEnforced(t *testing.T) {
	cases := map[string]func(s *schemav1.Schedule){
		"namespace 为空":    func(s *schemav1.Schedule) { s.Namespace = "" },
		"callback_url 为空": func(s *schemav1.Schedule) { s.CallbackUrl = "" },
		"cron 表达式为空":      func(s *schemav1.Schedule) { s.GetCron().Expr = "" },
		"cron 表达式过短":      func(s *schemav1.Schedule) { s.GetCron().Expr = "x" },
		"没有指定触发类型":        func(s *schemav1.Schedule) { s.Trigger = nil },
		"一次性 fire_at 为 0": func(s *schemav1.Schedule) {
			s.Trigger = &schemav1.Schedule_OneShot{OneShot: &schemav1.OneShotTrigger{FireAt: 0}}
		},
		"一次性 fire_at 为负": func(s *schemav1.Schedule) {
			s.Trigger = &schemav1.Schedule_OneShot{OneShot: &schemav1.OneShotTrigger{FireAt: -1}}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := registerShapedSchedule()
			mutate(s)
			assert.Error(t, validator.Validate(s))
		})
	}
}

// 一次性触发给一个正数 fire_at 应当放行。
func TestValidate_OneShotWithPositiveFireAt(t *testing.T) {
	s := registerShapedSchedule()
	s.Trigger = &schemav1.Schedule_OneShot{
		OneShot: &schemav1.OneShotTrigger{FireAt: 1_800_000_000_000},
	}
	assert.NoError(t, validator.Validate(s))
}
