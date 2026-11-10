// Package domain 是 AI 模块的领域类型与端口。
//
// 本模块本身不写 SQL：所有需要读库的地方都通过下面的端口拿数据，
// 具体实现在 adapter/mysql。
package domain

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	searchdomain "myblogx/internal/search/domain"
)

// ArticleSnapshot 是评分需要的文章片段。
type ArticleSnapshot struct {
	ID        ctype.ID
	AuthorID  ctype.ID
	Title     string
	Content   string
	UpdatedAt time.Time
	Published bool
}

// ScoreRecord 是一条文章质量评分记录。
type ScoreRecord struct {
	ID               ctype.ID
	ArticleID        ctype.ID
	UserID           ctype.ID
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
	ID    ctype.ID
	Title string
}

// ScoreRepository 是评分记录与文章快照的持久化端口。
type ScoreRepository interface {
	// Article 读文章快照；不存在时返回 apperr.CodeArticleNotFound。
	Article(ctx context.Context, id uint64) (ArticleSnapshot, error)
	// Latest 读该文章最近一次评分记录；没有时返回 nil。
	Latest(ctx context.Context, articleID uint64) (*ScoreRecord, error)
	// Upsert 写入一条评分记录：该文章还没有记录就新建，已有则整体覆盖。
	//
	// 同一篇文章在库里永远只有一行 —— 一行代表"最近一次评分"，不保留历史。
	// 用这一个方法而不是"查 + 写"两步，是为了把并发下的"只有一行"收进同一个事务。
	//
	// 目标文章**允许尚未落库**：质量闸门要在提交事务之外先评分，那时新建文章的
	// ID 已经生成、article_models 那一行还没插进去（ADR-0032 决策 5）。实现不得
	// 因为"查不到文章行"而返回错误 —— 那等于闸门一开，新文章一律提交不了。
	Upsert(ctx context.Context, record *ScoreRecord) error
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

// SceneSkill 是一份运行时策略 skill 的正文与版本。
//
// Version 是 SKILL.md 字节哈希（`skills.Skill.Hash()`），随 AI 调用记录落库：
// 线上跑的是哪一版口径要能回溯，而不是在代码里再写一个人工版本常量。
type SceneSkill struct {
	Name    string
	Body    string
	Version string
}

// SceneSkillReader 按名称读取运行时策略 skill。
//
// 端口在使用方定义、由装配层接到 `internal/platform/skills` 的目录加载器上；
// 业务层不 import 那个平台包，也就不会把"文件怎么读"混进用例逻辑。
//
// 读取失败时必须返回错误：配置引用了读不到的 skill 属于配置错误，
// 不能静默退化成一份空口径继续回答（不存在代码内兜底文本，ADR-0030）。
type SceneSkillReader interface {
	ReadSceneSkill(name string) (SceneSkill, error)
}
