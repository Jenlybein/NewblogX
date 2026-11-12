package http

import (
	"time"

	"myblogx/internal/platform/timezone"
)

// 本文件是站点数据概览的"时间窗"口径（票 349，#367 改为滚动窗口）。
//
// 三条规则：
//  1. 所有时间边界都按 **Asia/Shanghai** 切 —— 站点面向中文读者，服务器跑在
//     UTC 时用本地时区切天，用户在凌晨 0–8 点看到的"今天"其实是昨天；
//  2. `range` 只有一个词表（today / week / month / year），四个端点共用；
//  3. 空值 / 不认识的值一律回落到 week：概览页默认看一周，比"报错"更符合直觉。
//
// #367 起 week / month / year 都是**向前取**的滚动窗口，不再对齐自然周 / 自然月：
// 「本周一到现在」在周一早上只有一个数据点，图上等于什么都没画；而运营真正想问的
// 永远是"最近 N 天怎么样"。year 的粒度是**月**（12 个月），不是 365 天——
// 一年按天铺出来是一张没人看得懂的图。

// statsTimezone 是统计的时间边界。
//
// 取 platform/timezone 的唯一实现：这条规则原本在四处各写一份（含各自的
// tzdata 兜底），任意一份改坏就会让页面上的日期错一天。
var statsTimezone = timezone.Shanghai()

// statsRanges 是 `range` 的合法取值。
//
// today 仍被接受（1 天），但后台不再提供这个按钮：站点是周更节奏，
// "今日"在趋势图上只有一个点，看不出任何趋势。
var statsRanges = map[string]struct{}{
	"today": {},
	"week":  {},
	"month": {},
	"year":  {},
}

// 滚动窗口的长度（按桶计）。
const (
	weekDays   = 7
	monthDays  = 30
	yearMonths = 12
)

// normalizeStatsRange 收敛非法取值。
func normalizeStatsRange(raw string) string {
	if _, ok := statsRanges[raw]; ok {
		return raw
	}
	return "week"
}

// statsUnit 是趋势的桶粒度。
type statsUnit int

const (
	// unitDay 一天一个桶（today / week / month）。
	unitDay statsUnit = iota
	// unitMonth 一个月一个桶（year）。
	unitMonth
)

// statsWindow 是一次统计的时间窗。
type statsWindow struct {
	// Range 是收敛后的取值（回给前端做高亮）。
	Range string
	// Start 是窗口起点（含），已按东八区对齐到自然日 / 自然月。
	Start time.Time
	// End 是窗口终点（不含）。
	End time.Time
	// Buckets 是桶的数量：日粒度是天数，月粒度是月数（趋势图按它铺点）。
	Buckets int
	// unit 决定桶按天还是按月推进。
	unit statsUnit
}

// BucketStart 返回第 i 个桶的起点（含）。
func (w statsWindow) BucketStart(i int) time.Time {
	if w.unit == unitMonth {
		return w.Start.AddDate(0, i, 0)
	}
	return w.Start.AddDate(0, 0, i)
}

// BucketEnd 返回第 i 个桶的终点（不含）。
func (w statsWindow) BucketEnd(i int) time.Time {
	if w.unit == unitMonth {
		return w.Start.AddDate(0, i+1, 0)
	}
	return w.Start.AddDate(0, 0, i+1)
}

// BucketLabel 是第 i 个桶在前端的显示键（日 = 2006-01-02，月 = 2006-01）。
func (w statsWindow) BucketLabel(i int) string {
	if w.unit == unitMonth {
		return w.BucketStart(i).Format("2006-01")
	}
	return w.BucketStart(i).Format("2006-01-02")
}

// SpanDays 是窗口覆盖的自然日数。
//
// 与 Buckets 分开：月粒度的桶数只有 12，但"窗口有多长"要按天算，
// 概览页拿它去和 Redis 的按天保留期取较小值。
func (w statsWindow) SpanDays() int {
	return int(w.End.Sub(w.Start).Hours() / 24)
}

// windowOf 把 range 翻译成时间窗。
//
// now 由调用方给：测试里传固定时刻才能断言边界，生产传 time.Now()。
func windowOf(raw string, now time.Time) statsWindow {
	rng := normalizeStatsRange(raw)
	local := now.In(statsTimezone)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, statsTimezone)
	// 窗口右端固定是"明天 0 点"：今天还没过完，但今天的量要算进来。
	end := todayStart.AddDate(0, 0, 1)

	switch rng {
	case "today":
		return statsWindow{Range: rng, Start: todayStart, End: end, Buckets: 1}
	case "month":
		return statsWindow{
			Range:   rng,
			Start:   todayStart.AddDate(0, 0, -(monthDays - 1)),
			End:     end,
			Buckets: monthDays,
		}
	case "year":
		// 含本月在内的最近 12 个自然月，按整月切。
		thisMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, statsTimezone)
		return statsWindow{
			Range:   rng,
			Start:   thisMonth.AddDate(0, -(yearMonths - 1), 0),
			End:     thisMonth.AddDate(0, 1, 0),
			Buckets: yearMonths,
			unit:    unitMonth,
		}
	default: // week
		return statsWindow{
			Range:   "week",
			Start:   todayStart.AddDate(0, 0, -(weekDays - 1)),
			End:     end,
			Buckets: weekDays,
		}
	}
}
