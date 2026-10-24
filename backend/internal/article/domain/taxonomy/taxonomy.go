// Package taxonomy 是 article 模块下的分类与标签子域。
//
// 为什么单独成包：Category 与 Tag 被 ai / search / cron / user 多个模块引用，
// 它们不是文章的私有数据；但它们的生命周期又跟着文章走，所以放在 article 模块内
// 而不另立顶层模块。子包依赖父包 domain 的 ID 类型，父包不得反向依赖这里。
package taxonomy

import (
	"context"
	"errors"
	"strings"

	"myblogx/internal/article/domain"
)

// 子域错误。
var (
	// ErrTitleRequired 名称不能为空。
	ErrTitleRequired = errors.New("名称不能为空")
	// ErrTitleDuplicated 同一用户下名称重复。
	ErrTitleDuplicated = errors.New("名称重复")
	// ErrCategoryNotFound 分类不存在。
	ErrCategoryNotFound = errors.New("分类不存在")
	// ErrTagDisabled 标签不存在或已停用。
	ErrTagDisabled = errors.New("标签不存在或已停用")
	// ErrTagNotFound 标签不存在。
	ErrTagNotFound = errors.New("标签不存在")
)

// Category 是用户自建的分类。
//
// 唯一约束为 (userID, title)，与库里的 uk_category_user_title 一致。
type Category struct {
	id     domain.CategoryID
	userID domain.UserID
	title  string
}

// NewCategory 创建分类。标题去空白后不能为空。
func NewCategory(id domain.CategoryID, userID domain.UserID, title string) (*Category, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return nil, ErrTitleRequired
	}
	if userID.IsZero() {
		return nil, ErrTitleRequired
	}
	return &Category{id: id, userID: userID, title: trimmed}, nil
}

// RestoreCategory 从持久化数据重建，不做校验。
func RestoreCategory(id domain.CategoryID, userID domain.UserID, title string) *Category {
	return &Category{id: id, userID: userID, title: title}
}

// ID 返回分类 ID。
func (c *Category) ID() domain.CategoryID { return c.id }

// UserID 返回所属用户。
func (c *Category) UserID() domain.UserID { return c.userID }

// Title 返回标题。
func (c *Category) Title() string { return c.title }

// Rename 改名。
func (c *Category) Rename(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrTitleRequired
	}
	c.title = trimmed
	return nil
}

// BelongsTo 判断分类是否属于某个用户（删除/改名前的归属校验）。
func (c *Category) BelongsTo(userID domain.UserID) bool { return c.userID == userID }

// Tag 是公共标签词库中的一项。
type Tag struct {
	id           domain.TagID
	title        string
	sort         int
	description  string
	articleCount int
	enabled      bool
	createdBy    domain.UserID
}

// NewTag 创建标签。
func NewTag(id domain.TagID, title string, createdBy domain.UserID) (*Tag, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return nil, ErrTitleRequired
	}
	return &Tag{id: id, title: trimmed, enabled: true, createdBy: createdBy}, nil
}

// RestoreTagParams 是重建标签的输入。
type RestoreTagParams struct {
	ID           domain.TagID
	Title        string
	Sort         int
	Description  string
	ArticleCount int
	Enabled      bool
	CreatedBy    domain.UserID
}

// RestoreTag 从持久化数据重建。
func RestoreTag(p RestoreTagParams) *Tag {
	return &Tag{
		id:           p.ID,
		title:        p.Title,
		sort:         p.Sort,
		description:  p.Description,
		articleCount: p.ArticleCount,
		enabled:      p.Enabled,
		createdBy:    p.CreatedBy,
	}
}

// ID 返回标签 ID。
func (t *Tag) ID() domain.TagID { return t.id }

// Title 返回标题。
func (t *Tag) Title() string { return t.title }

// Sort 返回排序值。
func (t *Tag) Sort() int { return t.sort }

// Description 返回描述。
func (t *Tag) Description() string { return t.description }

// ArticleCount 返回文章数。
func (t *Tag) ArticleCount() int { return t.articleCount }

// Enabled 返回是否启用。
func (t *Tag) Enabled() bool { return t.enabled }

// Usable 判断标签能否被文章使用。
func (t *Tag) Usable() bool { return t.enabled }

// Disable 停用标签。停用后不能再被新文章引用，已引用的不自动摘除。
func (t *Tag) Disable() { t.enabled = false }

// Enable 启用标签。
func (t *Tag) Enable() { t.enabled = true }

// Rename 改名并重置排序与描述。
func (t *Tag) Rename(title string, sort int, description string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrTitleRequired
	}
	t.title = trimmed
	t.sort = sort
	t.description = description
	return nil
}

// FilterUsable 过滤出可用标签；遇到停用的标签直接报错。
//
// 与旧实现 loadEnabledTagsByIDs 等价：请求里带了停用/不存在的标签就整单失败，
// 而不是静默丢弃——否则用户以为标签加上了。
func FilterUsable(tags []*Tag) ([]*Tag, error) {
	out := make([]*Tag, 0, len(tags))
	for _, t := range tags {
		if t == nil || !t.Usable() {
			return nil, ErrTagDisabled
		}
		out = append(out, t)
	}
	return out, nil
}

// CategoryRepository 是分类的持久化契约。
type CategoryRepository interface {
	// Find 读取单个分类；不存在返回 ErrCategoryNotFound。
	Find(ctx context.Context, id domain.CategoryID) (*Category, error)
	// ListByUser 列出某用户的分类。
	ListByUser(ctx context.Context, userID domain.UserID) ([]*Category, error)
	// ExistsTitle 判断同用户下标题是否已存在（excludeID 用于更新时排除自身）。
	ExistsTitle(ctx context.Context, userID domain.UserID, title string, excludeID domain.CategoryID) (bool, error)
	// Save 新建或更新。
	Save(ctx context.Context, c *Category) error
	// Delete 删除。
	Delete(ctx context.Context, id domain.CategoryID) error
}

// TagRepository 是标签的持久化契约。
type TagRepository interface {
	// Find 读取单个标签。
	Find(ctx context.Context, id domain.TagID) (*Tag, error)
	// FindMany 批量读取；缺失的 ID 直接跳过。
	FindMany(ctx context.Context, ids []domain.TagID) ([]*Tag, error)
	// List 列出标签（includeDisabled 为真时含停用项）。
	List(ctx context.Context, includeDisabled bool) ([]*Tag, error)
	// ExistsTitle 判断标题是否已存在。
	ExistsTitle(ctx context.Context, title string, excludeID domain.TagID) (bool, error)
	// Save 新建或更新。
	Save(ctx context.Context, t *Tag) error
	// Delete 删除标签及其文章关联。
	Delete(ctx context.Context, id domain.TagID) error
	// ReplaceArticleTags 用给定标签集合替换某文章的全部标签。
	ReplaceArticleTags(ctx context.Context, articleID domain.ArticleID, tagIDs []domain.TagID) error
	// ArticleTagIDs 读取某文章当前的标签集合。
	ArticleTagIDs(ctx context.Context, articleID domain.ArticleID) ([]domain.TagID, error)
}
