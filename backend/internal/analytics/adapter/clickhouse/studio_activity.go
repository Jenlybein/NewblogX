package clickhouse

import (
	"context"
	"fmt"
	"time"

	"myblogx/internal/platform/timezone"
)

// DailyActivityCount 是"某一天有几次创作动作"。
type DailyActivityCount struct {
	// Day 是 `YYYY-MM-DD`（按 Asia/Shanghai 切天）。
	Day   string
	Count int
}

// studioActivitySQL 是热力图的取数语句。
//
// `toString(toDate(...))` 里的 toString **不能省**：clickhouse-go 的 Date 列只支持
// 扫描进 time.Time / sql.Scanner，扫进 *string 会直接报 ColumnConverterError。
// 2026-11-14 之前这就是热力图永远是 0 的真因 —— SQL 在 clickhouse-client 里跑得出数据，
// 数据也一直在表里，是 Go 这一侧的 Scan 一直在失败，而失败又被上层吞掉了。
const studioActivitySQL = `
SELECT toString(toDate(ts, ?)) AS day, count() AS c
FROM %s
WHERE user_id = ? AND success = 1 AND ts >= ?
  AND action_name IN ('article_create', 'article_update')
GROUP BY day
ORDER BY day ASC`

// CountDailyStudioActivity 统计某用户最近 days 天里每天的操作审计条数。
//
// 只数**成功**的 create / update：失败的动作没有产出内容，把它们算进"创作活跃度"
// 会让热力图变成"我试了多少次"。
//
// 2026-11-12 修正：过滤条件原来写的是 `action_name LIKE 'create%'` / `'update%'`，
// 但审计里的动作名是**带实体前缀**的（`article_create`、`article_update`、
// `outline_save`…）。前缀匹配一行都匹配不到。现在按确切动作名判断；将来要把
// "项目 / 合集也算创作"时改这个集合，而不是放宽成前缀。
//
// 用一条 GROUP BY 而不是把 120 天的记录拉回来在内存里数：审计表一天可能有几千行，
// 拉回来只为了求和是浪费。
func CountDailyStudioActivity(opts Options, userID uint64, days int) ([]DailyActivityCount, error) {
	if opts.ClickHouse == nil || userID == 0 || days <= 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 起点取统计时区里"今天零点"往前 days-1 天：与热力图的格子一一对应。
	// 用 timezone.StartOfDay 而不是 time.Now().AddDate(...)：后者按服务器本地时区
	// 算，跑在 UTC 时会让首尾两天错位。
	today := timezone.StartOfDay(time.Now())
	since := today.AddDate(0, 0, -(days - 1))
	sqlText := fmt.Sprintf(studioActivitySQL, ActionAuditLogTableName)

	rows, err := opts.ClickHouse.QueryContext(ctx, sqlText,
		// since 用 time.Time 传：ClickHouse 按列的时区做比较。格式化成字符串再比，
		// 会被当成 UTC 字面量，最早那天会白白少掉 8 小时（上海时区）。
		timezone.Name, userID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DailyActivityCount, 0, days)
	for rows.Next() {
		var item DailyActivityCount
		if err := rows.Scan(&item.Day, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
