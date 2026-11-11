// Package timezone 是全站"自然日 / 自然月"边界的唯一来源。
//
// 为什么要有这个包：统计口径必须按东八区切天（站点面向中文读者，服务器跑在 UTC 时
// 用本地时区会让凌晨 0–8 点的"今天"其实是昨天）。这条规则原本散在四处各写一遍
// ——分析 HTTP 层、ClickHouse 适配器、Redis 计数、AI 用量桶——每处都带一份
// "没有 tzdata 就退回 FixedZone(+8)"的兜底。四份兜底里任意一份被改坏，
// 页面上的日期就会错一天，而这种错很难在测试里被抓到。
package timezone

import "time"

// Name 是统计口径使用的时区名。
const Name = "Asia/Shanghai"

// shanghai 是进程内唯一的那份时区对象。
//
// 失败时退回固定 +8 偏移：没有 tzdata 的镜像（scratch / distroless）里
// LoadLocation 会失败，宁可差一次历史夏令时，也不能让整页统计崩掉。
var shanghai = load()

func load() *time.Location {
	loc, err := time.LoadLocation(Name)
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// Shanghai 返回统计口径的时区。
//
// 返回的是共享对象，调用方不得修改（*time.Location 本身不可变，这里只是说明语义）。
func Shanghai() *time.Location { return shanghai }

// StartOfDay 返回 t 在统计时区里当天的零点。
func StartOfDay(t time.Time) time.Time {
	local := t.In(shanghai)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghai)
}

// StartOfMonth 返回 t 在统计时区里当月的 1 日零点。
func StartOfMonth(t time.Time) time.Time {
	local := t.In(shanghai)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, shanghai)
}

// StartOfYear 返回 t 在统计时区里当年 1 月 1 日零点。
func StartOfYear(t time.Time) time.Time {
	local := t.In(shanghai)
	return time.Date(local.Year(), 1, 1, 0, 0, 0, 0, shanghai)
}
