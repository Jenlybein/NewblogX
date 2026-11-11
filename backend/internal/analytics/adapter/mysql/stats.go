package mysql

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/analytics/domain"

	"gorm.io/gorm"
)

// 发布状态：与 models/enum.ArticleStatusPublished 一致（业务模块不 import models）。
const publishStatusPublished int8 = 3

// Stats 实现统计所需的只读查询。
type Stats struct {
	db *gorm.DB
}

var _ domain.StatsRepository = (*Stats)(nil)

// NewStats 构造统计仓储。
func NewStats(db *gorm.DB) *Stats { return &Stats{db: db} }

// Summary 实现 domain.StatsRepository。
//
// 一条 SQL 拿全部总数：分开查四次在这个接口上是四次往返，没有收益。
func (s *Stats) Summary(ctx context.Context, since time.Time) (domain.Summary, error) {
	var row struct {
		UserCount    int64
		ArticleCount int64
		MessageCount int64
		CommentCount int64
		NewSignCount int64
	}
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM user_models) AS user_count,
			(SELECT COUNT(*) FROM article_models WHERE publish_status = ?) AS article_count,
			(SELECT COUNT(*) FROM chat_msg_models) AS message_count,
			(SELECT COUNT(*) FROM comment_models) AS comment_count,
			(SELECT COUNT(*) FROM user_models WHERE created_at >= ?) AS new_sign_count
		`,
		publishStatusPublished,
		since,
	).Scan(&row).Error
	if err != nil {
		return domain.Summary{}, err
	}
	return domain.Summary{
		UserCount:    row.UserCount,
		ArticleCount: row.ArticleCount,
		MessageCount: row.MessageCount,
		CommentCount: row.CommentCount,
		NewSignCount: row.NewSignCount,
	}, nil
}

// PublishedCountBetween 实现 domain.StatsRepository。
func (s *Stats) PublishedCountBetween(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Table("article_models").
		Where("created_at >= ? AND created_at < ?", start, end).
		Where("publish_status = ?", publishStatusPublished).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// SignupsByDay 实现 domain.StatsRepository。
//
// 返回的键必须是 YYYY-MM-DD（domain 端口契约），调用方按日拼键取值。
// 这里之所以要在 Go 侧再归一一次：MySQL 驱动开着 parseTime=True 时
// DATE(created_at) 会被当成 time.Time 返回，GORM 再把它字符串化成
// "2026-11-11T00:00:00+08:00"；而调用方用 "2006-01-02" 查表 → 逐日全 miss，
// 注册趋势因此恒为 0 且不报错。归一化放在读侧而不是改 SQL，是为了同时兼容
// SQLite 测试口径（它本来就返回纯日期），不必为方言写分支。
func (s *Stats) SignupsByDay(ctx context.Context, start, end time.Time) (map[string]int, error) {
	var rows []struct {
		Date  string
		Count int
	}
	err := s.db.WithContext(ctx).
		Table("user_models").
		Select("DATE(created_at) AS date, COUNT(*) AS count").
		Where("created_at >= ? AND created_at < ?", start, end).
		Group("DATE(created_at)").
		Order("DATE(created_at) ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		key := normalizeDayKey(row.Date)
		if key == "" {
			continue
		}
		out[key] += row.Count
	}
	return out, nil
}

// normalizeDayKey 把日期分组结果归一成 YYYY-MM-DD。
//
// DATE() 的值是**墙钟日期**：驱动按 loc 解析成 time.Time 后，日期部分仍是
// 分桶当天的日期，所以取前 10 个字符即正确桶位。刻意不走 time.Parse 再
// Format：那会在 loc=UTC 时把"午夜 00:00 东八区"重新格式化到前一天，
// 反而把桶挪错。
func normalizeDayKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 10 && isDayShape(raw[:10]) {
		return raw[:10]
	}
	// 兜底：非预期形状（例如驱动返回了别的序列化形态）按常见布局解析。
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at.Format("2006-01-02")
		}
	}
	return ""
}

// isDayShape 判断前 10 个字符是否是 YYYY-MM-DD 形状（只校验分隔位与数字位，
// 不校验月份/日期范围 —— 那由数据库产出保证）。
func isDayShape(v string) bool {
	if len(v) != 10 || v[4] != '-' || v[7] != '-' {
		return false
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}
