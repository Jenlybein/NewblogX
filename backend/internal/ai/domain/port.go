// Package domain 是 AI 模块的领域类型与端口。
//
// 本模块本身不写 SQL：所有需要读库的地方都通过下面的端口拿数据，
// 具体实现在 adapter/mysql。
package domain

import (
	"context"
	"time"

	searchdomain "myblogx/internal/search/domain"
)

// ArticleSnapshot 是评分需要的文章片段。
type ArticleSnapshot struct {
	ID        uint64
	AuthorID  uint64
	Title     string
	Content   string
	UpdatedAt time.Time
	Published bool
}

// ScoreRecord 是一条文章质量评分记录。
type ScoreRecord struct {
	ID               uint64
	ArticleID        uint64
	UserID           uint64
	TitleSnapshot    string
	ContentHash      string
	ContentLength    int
	ArticleUpdatedAt *time.Time
	AITotalScore     int
	TotalScore       int
	ScoreLevel       string
	ArticleType      string
	DimensionsJSON   string
	MainIssuesJSON   string
	OverallComment   string
	Provider         string
	ModelName        string
	PromptVersion    string
	CreatedAt        time.Time
}

// Option 是分类或标签候选项。
type Option struct {
	ID    uint64
	Title string
}

// ScoreRepository 是评分记录与文章快照的持久化端口。
type ScoreRepository interface {
	// Article 读文章快照；不存在时返回 apperr.CodeArticleNotFound。
	Article(ctx context.Context, id uint64) (ArticleSnapshot, error)
	// Latest 读最近一条评分记录；没有时返回 nil。
	Latest(ctx context.Context, articleID uint64) (*ScoreRecord, error)
	// CreateIfAbsent 只在还没有评分记录时写入，返回是否真的写入。
	//
	// 用一个方法而不是"查 + 写"两步，是为了把并发下的重复判断收进同一个事务。
	CreateIfAbsent(ctx context.Context, record *ScoreRecord) (bool, error)
}

// Catalog 提供文章元信息推荐用的分类与标签候选。
type Catalog interface {
	// Categories 返回该用户的分类。
	Categories(ctx context.Context, userID uint64) ([]Option, error)
	// Tags 返回全部启用中的标签。
	Tags(ctx context.Context) ([]Option, error)
}

// TagReader 返回启用中的标签标题（AI 搜索改写用）。
type TagReader interface {
	TagTitles(ctx context.Context) ([]string, error)
}

// Searcher 执行文章搜索。
//
// 直接复用 search 模块的请求/响应类型：AI 搜索页和普通搜索页返回的是同一个结构，
// 复制一份只会在两边字段漂移。
type Searcher interface {
	Search(ctx context.Context, req searchdomain.Request) (searchdomain.Response, error)
}
