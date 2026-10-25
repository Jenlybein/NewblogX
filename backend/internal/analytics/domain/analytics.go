// Package domain 是统计模块的领域类型与端口。
package domain

import (
	"context"
	"time"
)

// FlowPoint 是某一天的访问量。
type FlowPoint struct {
	Date  string
	Count int
}

// FlowCounter 读站点访问量。
//
// 访问量存在 Redis（按天计数），属于平台缓存能力，这里只声明需要什么。
type FlowCounter interface {
	// Current 返回今天截至目前的总访问量。
	Current() int
	// Recent 返回最近 days 天的访问量，按日期升序。
	Recent(days int) []FlowPoint
}

// Summary 是站点汇总数字。
type Summary struct {
	UserCount    int64
	ArticleCount int64
	MessageCount int64
	CommentCount int64
	NewSignCount int64
}

// StatsRepository 是统计所需的 MySQL 只读查询。
type StatsRepository interface {
	// Summary 统计用户/文章/私信/评论总数，以及 since 之后的新注册数。
	Summary(ctx context.Context, since time.Time) (Summary, error)
	// PublishedCountBetween 统计 [start, end) 内发布且已发布的文章数。
	PublishedCountBetween(ctx context.Context, start, end time.Time) (int, error)
	// SignupsByDay 统计 [start, end) 内每天的注册数，key 是 YYYY-MM-DD。
	SignupsByDay(ctx context.Context, start, end time.Time) (map[string]int, error)
}

// DeadLetter 是一条死信记录。
type DeadLetter struct {
	ID          uint64
	Stream      string
	CdcJobID    string
	SourceTable string
	Action      string
	TargetKey   string
	RetryCount  int
	Status      string
	ErrorCode   string
	ErrorMsg    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DeadLetterQuery 是死信检索条件。
type DeadLetterQuery struct {
	Stream         string
	Status         string
	CdcJobID       string
	CdcJobIDPrefix bool
	CdcJobIDs      []string
	Limit          int
}

// DeadLetterReader 查死信队列。
type DeadLetterReader interface {
	Query(ctx context.Context, q DeadLetterQuery) ([]DeadLetter, error)
}
