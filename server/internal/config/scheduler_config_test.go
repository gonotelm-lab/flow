package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const schedulerConfPath = "../../etc/conf.toml.tpl"

func TestDefaultSchedulerConfig_IsValid(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	require.NoError(t, cfg.Validate())

	assert.Equal(t, []time.Duration{
		3 * time.Second, 10 * time.Second,
	}, cfg.FastRetryIntervals)
	assert.Equal(t, []time.Duration{
		1 * time.Minute, 5 * time.Minute, 30 * time.Minute,
	}, cfg.DeadLetterIntervals)
}

// 这是配置层最重要的一条校验：租约若不长于 HTTP 超时，正常投递会先被自己的租约过期
// 判成可接管，另一个实例就会重投，凭空制造重复投递。
func TestSchedulerConfig_LeaseMustExceedHTTPTimeout(t *testing.T) {
	cfg := DefaultSchedulerConfig()

	cfg.LeaseDuration = cfg.HTTPTimeout
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be greater than")

	cfg.LeaseDuration = cfg.HTTPTimeout / 2
	require.Error(t, cfg.Validate())

	cfg.LeaseDuration = cfg.HTTPTimeout + time.Second
	require.NoError(t, cfg.Validate())
}

func TestSchedulerConfig_RejectsBadValues(t *testing.T) {
	cases := map[string]func(c *SchedulerConfig){
		"tickInterval 为零":       func(c *SchedulerConfig) { c.TickInterval = 0 },
		"tickInterval 为负":       func(c *SchedulerConfig) { c.TickInterval = -time.Second },
		"scanBatch 为零":          func(c *SchedulerConfig) { c.ScanBatch = 0 },
		"deliverConcurrency 为零": func(c *SchedulerConfig) { c.DeliverConcurrency = 0 },
		"httpTimeout 为零":        func(c *SchedulerConfig) { c.HTTPTimeout = 0 },
		"leaseDuration 为零":      func(c *SchedulerConfig) { c.LeaseDuration = 0 },
		"latenessTolerance 为零":  func(c *SchedulerConfig) { c.LatenessTolerance = 0 },
		"jitter 为负":             func(c *SchedulerConfig) { c.RetryJitterRatio = -0.1 },
		"jitter 等于 1":           func(c *SchedulerConfig) { c.RetryJitterRatio = 1 },
		"快重试序列为空":               func(c *SchedulerConfig) { c.FastRetryIntervalsRaw = "" },
		"死信序列为空":                func(c *SchedulerConfig) { c.DeadLetterIntervalsRaw = "  " },
		"快重试含非法时长":              func(c *SchedulerConfig) { c.FastRetryIntervalsRaw = "1s,abc" },
		"死信含非正时长":               func(c *SchedulerConfig) { c.DeadLetterIntervalsRaw = "1m,0s" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultSchedulerConfig()
			mutate(cfg)
			assert.Error(t, cfg.Validate())
		})
	}
}

func TestParseDurationList(t *testing.T) {
	got, err := parseDurationList(" 1s , 2m ,3h ")
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Minute, 3 * time.Hour}, got)

	got, err = parseDurationList("1s,,2s,")
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, got)

	got, err = parseDurationList("   ")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = parseDurationList("1s,oops")
	require.Error(t, err)

	_, err = parseDurationList("0s")
	require.Error(t, err)
}

// 用真实模板加载，顺带验证 conf.toml.tpl 的 [scheduler] 段语法与默认值。
func TestLoad_SchedulerSectionDefaults(t *testing.T) {
	for _, key := range []string{
		"FLOW_SCHEDULER_TICK_INTERVAL",
		"FLOW_SCHEDULER_SCAN_BATCH",
		"FLOW_SCHEDULER_DELIVER_CONCURRENCY",
		"FLOW_SCHEDULER_HTTP_TIMEOUT",
		"FLOW_SCHEDULER_LEASE_DURATION",
		"FLOW_SCHEDULER_LATENESS_TOLERANCE",
		"FLOW_SCHEDULER_RETRY_JITTER_RATIO",
		"FLOW_SCHEDULER_FAST_RETRY_INTERVALS",
		"FLOW_SCHEDULER_DEAD_LETTER_INTERVALS",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load(schedulerConfPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Scheduler, "模板里应当有 [scheduler] 段")

	s := cfg.Scheduler
	assert.Equal(t, time.Second, s.TickInterval)
	assert.Equal(t, 100, s.ScanBatch)
	assert.Equal(t, 64, s.DeliverConcurrency)
	assert.Equal(t, 10*time.Second, s.HTTPTimeout)
	assert.Equal(t, 30*time.Second, s.LeaseDuration)
	assert.Equal(t, 30*time.Second, s.LatenessTolerance)
	assert.InDelta(t, 0.2, s.RetryJitterRatio, 0.0001)
	// 快重试 2 个间隔 = 3 次尝试（含首发），死信 3 次，合计每次 fire 最多 6 次
	assert.Len(t, s.FastRetryIntervals, 2)
	assert.Len(t, s.DeadLetterIntervals, 3)
	assert.Len(t, cfg.Scheduler.FastRetryIntervals, 2)
	assert.Len(t, cfg.Scheduler.DeadLetterIntervals, 3)

	// 租约必须严格大于 HTTP 超时，否则配置本身就不合法
	assert.Greater(t, s.LeaseDuration, s.HTTPTimeout)
}

func TestLoad_SchedulerSectionEnvOverride(t *testing.T) {
	t.Setenv("FLOW_SCHEDULER_TICK_INTERVAL", "2s")
	t.Setenv("FLOW_SCHEDULER_HTTP_TIMEOUT", "3s")
	t.Setenv("FLOW_SCHEDULER_LEASE_DURATION", "9s")
	t.Setenv("FLOW_SCHEDULER_FAST_RETRY_INTERVALS", "500ms,1s")
	t.Setenv("FLOW_SCHEDULER_DEAD_LETTER_INTERVALS", "10s")

	cfg, err := Load(schedulerConfPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Scheduler)

	assert.Equal(t, 2*time.Second, cfg.Scheduler.TickInterval)
	assert.Equal(t, 3*time.Second, cfg.Scheduler.HTTPTimeout)
	assert.Equal(t, 9*time.Second, cfg.Scheduler.LeaseDuration)
	assert.Equal(t, []time.Duration{500 * time.Millisecond, time.Second}, cfg.Scheduler.FastRetryIntervals)
	assert.Equal(t, []time.Duration{10 * time.Second}, cfg.Scheduler.DeadLetterIntervals)
}

// 环境变量把租约配得比超时还短时，Load 必须直接失败，不能照常启动。
func TestLoad_SchedulerRejectsLeaseNotExceedingTimeout(t *testing.T) {
	t.Setenv("FLOW_SCHEDULER_HTTP_TIMEOUT", "10s")
	t.Setenv("FLOW_SCHEDULER_LEASE_DURATION", "5s")

	_, err := Load(schedulerConfPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler validate failed")
}
