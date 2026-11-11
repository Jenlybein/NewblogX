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

// actionAuditTimeLayout 与写入侧同一份时间格式。
const actionAuditTimeLayout = "2006-01-02 15:04:05"

// CountDailyStudioActivity 统计某用户最近 days 天里每天的操作审计条数。
//
// 只数**成功**的 create / update：失败的动作没有产出内容，把它们算进"创作活跃度"
// 会让热力图变成"我试了多少次"。
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
	sqlText := fmt.Sprintf(`
SELECT toDate(ts, ?) AS day, count() AS c
FROM %s
WHERE user_id = ? AND success = 1 AND ts >= ?
  AND (action_name LIKE 'create%%' OR action_name LIKE 'update%%')
GROUP BY day
ORDER BY day ASC`, ActionAuditLogTableName)

	rows, err := opts.ClickHouse.QueryContext(ctx, sqlText,
		timezone.Name, userID, since.Format(actionAuditTimeLayout))
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
