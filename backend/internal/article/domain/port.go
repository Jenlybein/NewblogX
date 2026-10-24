package domain

import "context"

// Repository 是文章聚合根的持久化契约。
//
// 只声明"业务需要什么"，不描述"用哪条 SQL 实现"。
// 实现放在 adapter/mysql，测试用 apptest 里的有状态 fake。
type Repository interface {
	// Find 按 ID 读取；不存在时返回 ErrArticleNotFound。
	Find(ctx context.Context, id ArticleID) (*Article, error)

	// FindMany 批量读取，缺失的 ID 直接跳过（列表页容忍脏 ID）。
	FindMany(ctx context.Context, ids []ArticleID) ([]*Article, error)

	// List 按条件分页查询，返回当页数据与总数。
	List(ctx context.Context, f Filter, p Page) ([]*Article, int64, error)

	// Create 插入新文章；ID 必须由调用方给好。
	Create(ctx context.Context, a *Article) error

	// Update 保存已有文章的变更；文章必须已存在。
	Update(ctx context.Context, a *Article) error

	// SoftDelete 标记删除（不物理删）。
	SoftDelete(ctx context.Context, id ArticleID) error
}

// Filter 是列表查询条件。
//
// 用结构体而不是变参：调用点少写一堆 Where，实现侧也只需要拼一次条件。
type Filter struct {
	// AuthorID 非 nil 时只返回该作者的文章。
	AuthorID *UserID
	// Statuses 非空时只返回这些状态。
	Statuses []Status
	// CategoryID 非 nil 时只返回该分类。
	CategoryID *CategoryID
	// TagIDs 非空时要求文章至少命中其中一个标签。
	TagIDs []TagID
	// OnlyPublic 为真时只返回"匿名可见"的文章（已发布 + visible）。
	//
	// 匿名列表页必须置真：否则草稿会泄漏到前台。
	OnlyPublic bool
	// Keyword 非空时按标题/摘要模糊匹配。
	Keyword string
}

// Page 是分页参数。
//
// 用 Offset 而不是游标：与现有接口的 page/page_size 语义保持一致，
// 换游标会改变前端契约（golden 会红）。
type Page struct {
	Offset int
	Limit  int
}

// Normalize 收敛非法分页参数，避免 limit=0 时查出全表或空集。
func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 10
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
