// Package scheduler 负责定时回调的到期判定、HTTP 投递与重试/死信状态推进。
package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// DefaultTimezone 是未指定时区时的缺省值。
const DefaultTimezone = "UTC"

// DefaultContentType 是未指定 content-type 时的缺省值。
const DefaultContentType = "application/octet-stream"

// cronParser 只接受标准 5 字段（分 时 日 月 周）。
//
// 不传 cron.Descriptor，@daily / @every 这类描述符会被库直接拒绝；不传 cron.Second，
// 6 字段表达式会报 "expected exactly 5 fields, found 6"。「拒绝 6 字段」「拒绝描述符」
// 因此由库在解析层强制，不需要我们自己写校验。
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// Parse 解析 cron 表达式。
//
// expr 为标准 5 字段；tz 为 IANA 时区名，空表示 UTC。
//
// 时区必须由我们注入 CRON_TZ 前缀：robfig/cron/v3 的 Parser.Parse(spec string) 没有
// location 参数，内部写死 loc = time.Local。不注入的话「缺省 UTC」会静默变成服务器
// 本地时区——这是一个不会报错、只会算错触发时刻的坑。
func Parse(expr, tz string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron expression is empty")
	}

	// 表达式内自带时区前缀会造成两个时区来源，与独立的 timezone 字段冲突，直接拒绝。
	if strings.HasPrefix(expr, "TZ=") || strings.HasPrefix(expr, "CRON_TZ=") {
		return nil, fmt.Errorf(
			"cron expression must not embed a timezone prefix; use the timezone field instead")
	}

	loc, err := ResolveTimezone(tz)
	if err != nil {
		return nil, err
	}

	sched, err := cronParser.Parse("CRON_TZ=" + loc.String() + " " + expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}

	return sched, nil
}

// ResolveTimezone 把 IANA 时区名解析为 *time.Location，空值返回 UTC。
func ResolveTimezone(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	return loc, nil
}

// Next 返回 after 之后的下一次触发时刻；无解时返回零值。
func Next(sched cron.Schedule, after time.Time) time.Time {
	return sched.Next(after)
}

// NextMillis 按毫秒时间戳计算下一次触发时刻；无解时返回 0。
//
// 只做「解析 + 试算一次 Next」的封装，供服务层校验表达式时使用。
func NextMillis(expr, tz string, afterMillis int64) (int64, error) {
	sched, err := Parse(expr, tz)
	if err != nil {
		return 0, err
	}

	next := sched.Next(time.UnixMilli(afterMillis))
	if next.IsZero() {
		return 0, fmt.Errorf("cron expression %q has no next fire time", expr)
	}
	return next.UnixMilli(), nil
}

// CountMissedMillis 按毫秒时间戳统计 (after, until] 内错过的触发次数，最多 max 次。
func CountMissedMillis(expr, tz string, afterMillis, untilMillis int64, max int) (int, error) {
	sched, err := Parse(expr, tz)
	if err != nil {
		return 0, err
	}
	return CountMissed(sched, time.UnixMilli(afterMillis), time.UnixMilli(untilMillis), max), nil
}

// CountMissed 统计 (after, until] 区间内错过的触发次数，最多数 max 次。
//
// 用途：一次多日停机后逐个回放会很久，所以跳过时直接对齐到 Next(until)，而计数用
// 这个有界循环给出（封顶 max），既保证准确到上限内、又不让一次调用跑很久。
func CountMissed(sched cron.Schedule, after, until time.Time, max int) int {
	if max <= 0 {
		return 0
	}

	n := 0
	t := after
	for n < max {
		t = sched.Next(t)
		if t.IsZero() || t.After(until) {
			break
		}
		n++
	}
	return n
}
