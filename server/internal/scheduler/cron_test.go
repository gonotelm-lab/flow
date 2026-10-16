package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_AcceptsStandard5Fields(t *testing.T) {
	for _, expr := range []string{
		"* * * * *",
		"0 9 * * *",
		"*/5 * * * *",
		"0 0 1 * *",
		"30 10-13 ? * WED,FRI",
		"0 12 * * MON-FRI",
		"0/2 8-17 1,15 jan-mar SUN",
	} {
		t.Run(expr, func(t *testing.T) {
			sched, err := Parse(expr, "")
			require.NoError(t, err)
			assert.NotZero(t, Next(sched, time.Now()))
		})
	}
}

// 秒级 cron 会把回调打爆，因此 6 字段必须被拒绝。这条保证来自不传 cron.Second。
func TestParse_Rejects6Fields(t *testing.T) {
	_, err := Parse("*/1 * * * * *", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5 fields")
}

// cron 用标准表达式的另一半：描述符也不接受，来自不传 cron.Descriptor。
func TestParse_RejectsDescriptors(t *testing.T) {
	for _, expr := range []string{"@daily", "@every 1h30m", "@hourly", "@midnight"} {
		t.Run(expr, func(t *testing.T) {
			_, err := Parse(expr, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "descriptor")
		})
	}
}

// Quartz 扩展（L / W / #）不支持，避免出现看起来能用、语义却和 Quartz 不同的表达式。
func TestParse_RejectsQuartzExtensions(t *testing.T) {
	for _, expr := range []string{
		"0 0 L * *",
		"0 0 15W * *",
		"0 0 * * 6#3",
		"0 0 * * FRIL",
	} {
		t.Run(expr, func(t *testing.T) {
			_, err := Parse(expr, "")
			require.Error(t, err)
		})
	}
}

// 表达式内自带时区会与独立的 timezone 字段冲突，必须拒绝。
func TestParse_RejectsEmbeddedTimezone(t *testing.T) {
	for _, expr := range []string{
		"CRON_TZ=Asia/Tokyo 30 04 * * *",
		"TZ=UTC 0 9 * * *",
	} {
		t.Run(expr, func(t *testing.T) {
			_, err := Parse(expr, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not embed a timezone")
		})
	}
}

func TestParse_RejectsInvalidTimezone(t *testing.T) {
	_, err := Parse("0 9 * * *", "Not/AZone")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone")
}

func TestParse_RejectsEmptyExpression(t *testing.T) {
	_, err := Parse("   ", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// 这是本文件最重要的一个测试。
//
// robfig/cron/v3 的 Parser.Parse(spec) 没有 location 参数，内部写死 time.Local。
// 不注入 CRON_TZ 前缀的话，「每天 9 点」会静默按服务器本地时区算，不报错，只算错。
//
// robfig 还有另一个契约：SpecSchedule.Next 找到时刻后会转回入参所在的时区
// （源码注释 Save the original timezone so we can convert back），所以返回值的
// .Location() 是入参的时区，不是 schedule 的时区。我们只依赖时刻（落库是 unix
// 毫秒），要按 schedule 的时区展示时由展示方自己 In(loc)。
func TestParse_TimezoneChangesNextFireTime(t *testing.T) {
	// 2026-01-05 是周一
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	shanghai, err := ResolveTimezone("Asia/Shanghai")
	require.NoError(t, err)

	t.Run("Asia/Shanghai 的 9 点等于 01:00 UTC", func(t *testing.T) {
		sched, err := Parse("0 9 * * *", "Asia/Shanghai")
		require.NoError(t, err)

		next := Next(sched, base)
		assert.Equal(t, time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC), next.UTC())
		assert.Equal(t, "09:00", next.In(shanghai).Format("15:04"),
			"按 schedule 自己的时区展示应当是 9 点")
	})

	t.Run("缺省时区是 UTC 而不是服务器本地时区", func(t *testing.T) {
		sched, err := Parse("0 9 * * *", "")
		require.NoError(t, err)

		next := Next(sched, base)
		assert.Equal(t, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), next.UTC())
		assert.Equal(t, "17:00", next.In(shanghai).Format("15:04"),
			"UTC 的 9 点在东八区是 17 点；若这里是 09:00 说明时区没生效")
	})

	t.Run("两种时区结果必须不同", func(t *testing.T) {
		withTz, err := Parse("0 9 * * *", "Asia/Shanghai")
		require.NoError(t, err)
		utc, err := Parse("0 9 * * *", "")
		require.NoError(t, err)

		assert.NotEqual(t, Next(withTz, base).UTC(), Next(utc, base).UTC())
	})

	t.Run("入参时区不影响结果时刻", func(t *testing.T) {
		sched, err := Parse("0 9 * * *", "Asia/Shanghai")
		require.NoError(t, err)

		fromUTC := Next(sched, base)
		fromShanghai := Next(sched, base.In(shanghai))
		assert.Equal(t, fromUTC.UTC(), fromShanghai.UTC(),
			"触发时刻是绝对时间，不应随调用方所在时区变化")
	})
}

// 记录一个几乎所有手写 cron 解析器都会写错的点：日 与 星期 是 OR 不是 AND。
// `0 0 1 * MON` = 每月 1 号或每周一，不是每月 1 号且是周一。
// 我们用库的语义，所以这个测试同时是对外契约的文档。
func TestNext_DomDowIsOrNotAnd(t *testing.T) {
	// 2026-01-02 是周五，2026-01-05 是周一，2026-02-01 在更后面
	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	sched, err := Parse("0 0 1 * MON", "")
	require.NoError(t, err)

	next := Next(sched, base).UTC()
	assert.Equal(t, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), next,
		"应当命中周一（OR 语义）；若命中 2 月 1 日说明被当成了 AND")
}

func TestNext_WeekdayNumberingIsSundayZero(t *testing.T) {
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // 周一

	// 0 = Sunday：从周一起算，下一个周日在 1 月 11 日
	sched, err := Parse("0 0 * * 0", "")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), Next(sched, base).UTC())

	// 名字与数字必须等价
	byName, err := Parse("0 0 * * SUN", "")
	require.NoError(t, err)
	assert.Equal(t, Next(sched, base).UTC(), Next(byName, base).UTC())
}

func TestCountMissed(t *testing.T) {
	sched, err := Parse("*/1 * * * *", "")
	require.NoError(t, err)

	after := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 1, 5, 0, 10, 30, 0, time.UTC)

	// (00:00, 00:10:30] 内每分钟一次 = 00:01 ... 00:10，共 10 次
	assert.Equal(t, 10, CountMissed(sched, after, until, 1000))

	// 封顶
	assert.Equal(t, 3, CountMissed(sched, after, until, 3))

	// max <= 0 视为不计数
	assert.Equal(t, 0, CountMissed(sched, after, until, 0))

	// 一分钟都没到
	assert.Equal(t, 0, CountMissed(sched, after,
		time.Date(2026, 1, 5, 0, 0, 30, 0, time.UTC), 100))
}

func TestResolveTimezone(t *testing.T) {
	loc, err := ResolveTimezone("")
	require.NoError(t, err)
	assert.Equal(t, time.UTC, loc)

	loc, err = ResolveTimezone("Asia/Shanghai")
	require.NoError(t, err)
	assert.Equal(t, "Asia/Shanghai", loc.String())

	_, err = ResolveTimezone("Bogus/Zone")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid timezone"))
}
