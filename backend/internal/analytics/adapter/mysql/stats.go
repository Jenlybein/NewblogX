package mysql

import (
	"context"
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
		out[row.Date] = row.Count
	}
	return out, nil
}
