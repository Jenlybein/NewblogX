package http

import (
	"time"

	"myblogx/internal/platform/timezone"
)

// 本文件是站点数据概览的"时间窗"口径（票 349）。
//
// 三条规则：
//  1. 所有时间边界都按 **Asia/Shanghai** 切 —— 站点面向中文读者，服务器跑在
//     UTC 时用本地时区切天，用户在凌晨 0–8 点看到的"今天"其实是昨天；
//  2. `range` 只有一个词表（today / week / month / year），四个端点共用；
//  3. 空值 / 不认识的值一律回落到 week：概览页默认看一周，比"报错"更符合直觉。

// statsTimezone 是统计的时间边界。
//
// 取 platform/timezone 的唯一实现：这条规则原本在四处各写一份（含各自的
// tzdata 兜底），任意一份改坏就会让页面上的日期错一天。
var statsTimezone = timezone.Shanghai()

// statsRanges 是 `range` 的合法取值。
var statsRanges = map[string]struct{}{
	"today": {},
	"week":  {},
	"month": {},
	"year":  {},
}

// normalizeStatsRange 收敛非法取值。
func normalizeStatsRange(raw string) string {
	if _, ok := statsRanges[raw]; ok {
		return raw
	}
	return "week"
}

// statsWindow 是一次统计的时间窗。
type statsWindow struct {
	// Range 是收敛后的取值（回给前端做高亮）。
	Range string
	// Start 是窗口起点（含），已按东八区对齐到自然日 / 自然月。
	Start time.Time
	// End 是窗口终点（不含）。
	End time.Time
	// Days 是窗口覆盖的自然日数（画趋势图用）。
	Days int
}

// windowOf 把 range 翻译成时间窗。
//
// now 由调用方给：测试里传固定时刻才能断言边界，生产传 time.Now()。
func windowOf(raw string, now time.Time) statsWindow {
	rng := normalizeStatsRange(raw)
	local := now.In(statsTimezone)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, statsTimezone)

	switch rng {
	case "today":
		return statsWindow{Range: rng, Start: todayStart, End: todayStart.AddDate(0, 0, 1), Days: 1}
	case "month":
		monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, statsTimezone)
		days := int(todayStart.Sub(monthStart).Hours()/24) + 1
		return statsWindow{Range: rng, Start: monthStart, End: todayStart.AddDate(0, 0, 1), Days: days}
	case "year":
		yearStart := time.Date(local.Year(), 1, 1, 0, 0, 0, 0, statsTimezone)
		days := int(todayStart.Sub(yearStart).Hours()/24) + 1
		return statsWindow{Range: rng, Start: yearStart, End: todayStart.AddDate(0, 0, 1), Days: days}
	default: // week
		// "本周"从周一起：中文语境的"这一周"就是周一到周日。
		weekday := (int(local.Weekday()) + 6) % 7 // 周一 = 0
		weekStart := todayStart.AddDate(0, 0, -weekday)
		days := weekday + 1
		return statsWindow{Range: "week", Start: weekStart, End: todayStart.AddDate(0, 0, 1), Days: days}
	}
}
