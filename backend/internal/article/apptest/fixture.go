package apptest

import (
	"time"

	"myblogx/internal/article/domain"
)

// DefaultTime 是测试统一使用的"现在"。
//
// 固定值而不是 time.Now()：golden 与"审核时间是否被写入"这类断言
// 都依赖它可复现。
var DefaultTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// ArticleBuilder 用链式调用构造测试文章。
//
// 例：
//
//	apptest.NewArticle().ID(1).Author(100).Status(domain.StatusPublished).Build()
type ArticleBuilder struct {
	p domain.RestoreParams
}

// NewArticle 返回一个默认草稿的构造器。
func NewArticle() *ArticleBuilder {
	return &ArticleBuilder{p: domain.RestoreParams{
		ID:             1,
		AuthorID:       100,
		Title:          "测试文章",
		Abstract:       "摘要",
		Content:        "正文内容",
		Status:         domain.StatusDraft,
		Visibility:     domain.VisibilityVisible,
		CommentsToggle: true,
		CreatedAt:      DefaultTime,
		UpdatedAt:      DefaultTime,
	}}
}

// ID 设定文章 ID。
func (b *ArticleBuilder) ID(id domain.ArticleID) *ArticleBuilder { b.p.ID = id; return b }

// Author 设定作者。
func (b *ArticleBuilder) Author(id domain.UserID) *ArticleBuilder { b.p.AuthorID = id; return b }

// Title 设定标题。
func (b *ArticleBuilder) Title(t string) *ArticleBuilder { b.p.Title = t; return b }

// Abstract 设定摘要。
func (b *ArticleBuilder) Abstract(s string) *ArticleBuilder { b.p.Abstract = s; return b }

// Content 设定正文。
func (b *ArticleBuilder) Content(s string) *ArticleBuilder { b.p.Content = s; return b }

// Status 设定发布状态。
func (b *ArticleBuilder) Status(s domain.Status) *ArticleBuilder { b.p.Status = s; return b }

// Visibility 设定可见性。
func (b *ArticleBuilder) Visibility(v domain.Visibility) *ArticleBuilder {
	b.p.Visibility = v
	return b
}

// Category 设定分类。
func (b *ArticleBuilder) Category(id domain.CategoryID) *ArticleBuilder {
	b.p.CategoryID = &id
	return b
}

// Cover 设定封面。
func (b *ArticleBuilder) Cover(u string) *ArticleBuilder { b.p.Cover = u; return b }

// Tags 设定标签。
func (b *ArticleBuilder) Tags(ids ...domain.TagID) *ArticleBuilder { b.p.Tags = ids; return b }

// Deleted 标记已删除。
func (b *ArticleBuilder) Deleted() *ArticleBuilder { b.p.Deleted = true; return b }

// SubmittedAt 设定提交时间。
func (b *ArticleBuilder) SubmittedAt(t time.Time) *ArticleBuilder { b.p.SubmittedAt = &t; return b }

// CreatedAt 设定创建时间。
func (b *ArticleBuilder) CreatedAt(t time.Time) *ArticleBuilder { b.p.CreatedAt = t; return b }

// Build 产出聚合。
func (b *ArticleBuilder) Build() *domain.Article { return domain.Restore(b.p) }
