package clickhouse

import (
	"fmt"
	"time"
)

// 本文件负责"按周期切桶"，是首页 AI 用量图的数据形状。
//
// 为什么桶在 Go 里生成而不是用 ClickHouse 的 WITH FILL：CH 只返回**有数据**的
// 桶，图表需要连续的时间轴（今天 24 个小时、本年 12 个月都得在），
// 而 WITH FILL 在"整段区间都没数据"时的行为依赖服务端版本，也没法单测。
// 这里自己算桶键，取不到的补 0。

// cst 是日志表的固定时区。
//
// 不能用 time.Local：容器 TZ 一般是 UTC，用它生成桶键会让"今日"曲线整体
// 偏移 8 小时，而且不会报任何错 —— 表面上只是"凌晨那几个小时没有调用"。
var cst = time.FixedZone("CST", 8*3600)

// 桶键在两套格式里各写一份：ClickHouse 用 %Y-%m-%d，Go 的 time.Format 用
// 2006-01-02 那套参考时间。之前把两者混成一个常量，结果是所有桶键都变成
// 字面量 "%Y-%m-%d %H:%M:%S" —— 查得到数据也对不上，而且不报错。
const (
	aiBucketCHFormat = "%Y-%m-%d %H:%M:%S"
	aiBucketGoFormat = "2006-01-02 15:04:05"
)

// aiBuckets 是一个周期的时间轴定义。
type aiBuckets struct {
	Start time.Time
	End   time.Time
	// Granularity 是 hour / day / month，返回给前端做文案。
	Granularity string
	// Expr 是 ClickHouse 侧的分桶表达式（常量，不来自用户输入）。
	Expr string
	// Keys 与 CH 的 toString 结果同构；Labels 是给人看的短标签。
	Keys   []string
	Labels []string
}

// buildAIBuckets 按周期生成时间轴。
//
// now 由调用方注入而不是内部取 time.Now()：这样才能在单测里钉死跨年、
// 跨月、闰月、"周一正好是 1 号"这些边界。
func buildAIBuckets(rng string, now time.Time) (aiBuckets, error) {
	local := now.In(cst)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, cst)

	switch rng {
	case "today":
		buckets := aiBuckets{
			Start:       today,
			End:         today.AddDate(0, 0, 1),
			Granularity: "hour",
			Expr:        "formatDateTime(toStartOfHour(ts), '" + aiBucketCHFormat + "', 'Asia/Shanghai')",
		}
		for hour := 0; hour < 24; hour++ {
			at := buckets.Start.Add(time.Duration(hour) * time.Hour)
			buckets.Keys = append(buckets.Keys, at.Format(aiBucketGoFormat))
			buckets.Labels = append(buckets.Labels, at.Format("15:04"))
		}
		return buckets, nil

	case "week":
		// 一周从周一开始：Go 的 Weekday 里周日是 0，这里换算成周一=0。
		offset := (int(local.Weekday()) + 6) % 7
		start := today.AddDate(0, 0, -offset)
		buckets := aiBuckets{
			Start:       start,
			End:         start.AddDate(0, 0, 7),
			Granularity: "day",
			Expr:        "formatDateTime(toStartOfDay(ts), '" + aiBucketCHFormat + "', 'Asia/Shanghai')",
		}
		for day := 0; day < 7; day++ {
			at := buckets.Start.AddDate(0, 0, day)
			buckets.Keys = append(buckets.Keys, at.Format(aiBucketGoFormat))
			buckets.Labels = append(buckets.Labels, at.Format("01-02"))
		}
		return buckets, nil

	case "month":
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, cst)
		end := start.AddDate(0, 1, 0)
		buckets := aiBuckets{
			Start:       start,
			End:         end,
			Granularity: "day",
			Expr:        "formatDateTime(toStartOfDay(ts), '" + aiBucketCHFormat + "', 'Asia/Shanghai')",
		}
		// 用 AddDate 逐天推进而不是 end.Sub(start)/24h：跨夏令时的时区会差一小时，
		// 东八区虽然不跨，但这个写法不依赖时区假设。
		for at := start; at.Before(end); at = at.AddDate(0, 0, 1) {
			buckets.Keys = append(buckets.Keys, at.Format(aiBucketGoFormat))
			buckets.Labels = append(buckets.Labels, at.Format("01-02"))
		}
		return buckets, nil

	case "year":
		start := time.Date(local.Year(), time.January, 1, 0, 0, 0, 0, cst)
		end := start.AddDate(1, 0, 0)
		buckets := aiBuckets{
			Start:       start,
			End:         end,
			Granularity: "month",
			Expr:        "formatDateTime(toStartOfMonth(ts), '" + aiBucketCHFormat + "', 'Asia/Shanghai')",
		}
		for at := start; at.Before(end); at = at.AddDate(0, 1, 0) {
			buckets.Keys = append(buckets.Keys, at.Format(aiBucketGoFormat))
			buckets.Labels = append(buckets.Labels, at.Format("2006-01"))
		}
		return buckets, nil

	default:
		return aiBuckets{}, fmt.Errorf("不支持的统计周期: %q", rng)
	}
}
