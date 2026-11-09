// Package taxonomy 是 article 模块下的分类与标签子域。
//
// 为什么单独成包：Category 与 Tag 被 ai / search / cron / user 多个模块引用，
// 它们不是文章的私有数据；但它们的生命周期又跟着文章走，所以放在 article 模块内
// 而不另立顶层模块。子包依赖父包 domain 的 ID 类型，父包不得反向依赖这里。
package taxonomy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// 子域错误。文案统一在 internal/apperr 的消息表里。
var (
	// ErrTitleRequired 名称不能为空。
	ErrTitleRequired = apperr.New(apperr.CodeTaxonomyTitleRequired)
	// ErrTitleDuplicated 同一用户下名称重复。
	ErrTitleDuplicated = apperr.New(apperr.CodeTaxonomyTitleExists)
	// ErrCategoryNotFound 分类不存在。
	ErrCategoryNotFound = apperr.New(apperr.CodeCategoryNotFound)
	// ErrTagDisabled 标签不存在或已停用。
	ErrTagDisabled = apperr.New(apperr.CodeTagDisabled)
	// ErrTagNotFound 标签不存在。
	ErrTagNotFound = apperr.New(apperr.CodeTagNotFound)
)

// TagInUseError 是"标签仍被文章引用，不能删除"。
//
// 为什么不用 apperr.New 的固定文案：管理员要看到**几篇**在用，才知道这件事
// 影响多大、该不该直接停用；只说"有引用"，他还得自己去数。
// 错误码仍然只有一个（apperr.CodeTagInUse），前端按码分支、按文案展示。
type TagInUseError struct {
	count int
}

// NewTagInUseError 构造"标签被引用"错误。
func NewTagInUseError(count int) *TagInUseError { return &TagInUseError{count: count} }

// Count 返回引用该标签的文章数。
func (e *TagInUseError) Count() int { return e.count }

// Error 返回面向管理员的文案（含篇数与下一步动作）。
func (e *TagInUseError) Error() string {
	return fmt.Sprintf("该标签已有 %d 篇文章在用，请改用停用", e.count)
}

// Unwrap 让 errors.Is / apperr.CodeOf 能拿到错误码（文案由 Error 覆盖）。
func (e *TagInUseError) Unwrap() error { return apperr.New(apperr.CodeTagInUse) }

// Category 是用户自建的分类。
//
// 唯一约束为 (userID, title)，与库里的 uk_category_user_title 一致。
type Category struct {
	id     domain.CategoryID
	userID domain.UserID
	title  string

	// 读模型字段：列表页要展示文章数与作者展示信息。
	// 放在实体上而不是另建 View，是为了让仓储一次查询就返回列表所需的全部字段。
	createdAt     time.Time
	updatedAt     time.Time
	articleCount  int
	ownerNickname string
	ownerAvatar   string
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

// RestoreCategoryParams 是带读模型字段的重建参数。
type RestoreCategoryParams struct {
	ID            domain.CategoryID
	UserID        domain.UserID
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArticleCount  int
	OwnerNickname string
	OwnerAvatar   string
}

// RestoreCategoryView 从持久化数据重建（含列表页所需读模型字段）。
func RestoreCategoryView(p RestoreCategoryParams) *Category {
	return &Category{
		id:            p.ID,
		userID:        p.UserID,
		title:         p.Title,
		createdAt:     p.CreatedAt,
		updatedAt:     p.UpdatedAt,
		articleCount:  p.ArticleCount,
		ownerNickname: p.OwnerNickname,
		ownerAvatar:   p.OwnerAvatar,
	}
}

// ID 返回分类 ID。
func (c *Category) ID() domain.CategoryID { return c.id }

// UserID 返回所属用户。
func (c *Category) UserID() domain.UserID { return c.userID }

// Title 返回标题。
func (c *Category) Title() string { return c.title }

// CreatedAt 返回创建时间（读模型）。
func (c *Category) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt 返回更新时间（读模型）。
func (c *Category) UpdatedAt() time.Time { return c.updatedAt }

// ArticleCount 返回该分类下文章数（读模型）。
func (c *Category) ArticleCount() int { return c.articleCount }

// OwnerNickname 返回所属用户昵称（读模型）。
func (c *Category) OwnerNickname() string { return c.ownerNickname }

// OwnerAvatar 返回所属用户头像（读模型）。
func (c *Category) OwnerAvatar() string { return c.ownerAvatar }

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
	createdAt    time.Time
	updatedAt    time.Time
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
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
		createdAt:    p.CreatedAt,
		updatedAt:    p.UpdatedAt,
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

// CreatedBy 返回创建人。
func (t *Tag) CreatedBy() domain.UserID { return t.createdBy }

// CreatedAt 返回创建时间（读模型）。
func (t *Tag) CreatedAt() time.Time { return t.createdAt }

// UpdatedAt 返回更新时间（读模型）。
func (t *Tag) UpdatedAt() time.Time { return t.updatedAt }

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
	//
	// 新建时若存在**同名软删行**（(user_id, title) 唯一索引不含 deleted_at，
	// 删除又是软删），实现必须恢复那一行而不是插入（ADR-0036 约束 1，
	// 房规见 platform/mysql.RestoreOrCreateUnique）：因此新建后库里的 ID
	// 可能不是调用方给的那个，调用方不要依赖新建时的 ID（列表另行读取）。
	Save(ctx context.Context, c *Category) error
	// Delete 删除分类。
	//
	// 契约是**两步、顺序固定**（ADR-0036 约束 3）：先把挂在该分类下的文章
	// `category_id` 置空（文章本身不删，变成"未分类"），再软删分类本身。
	// 两步必须在调用方的事务里完成（UseCase 的 WithinTx 现场构造本仓储），
	// 中途失败不能留下"文章已解绑、分类还在"或相反的半截状态。
	Delete(ctx context.Context, id domain.CategoryID) error
}

// TagRepository 是标签的持久化契约。
type TagRepository interface {
	// Find 读取单个标签。
	Find(ctx context.Context, id domain.TagID) (*Tag, error)
	// FindMany 批量读取；缺失的 ID 直接跳过。
	FindMany(ctx context.Context, ids []domain.TagID) ([]*Tag, error)
	// List 列出标签（includeDisabled 为真时含停用项）。
	//
	// 返回的 ArticleCount 必须按关联表实时统计，不读 tag_models.article_count：
	// 那一列没有任何维护者，真实数据里恒为 0，拿它显示"文章数"等于骗人。
	List(ctx context.Context, includeDisabled bool) ([]*Tag, error)
	// ExistsTitle 判断标题是否已存在。
	ExistsTitle(ctx context.Context, title string, excludeID domain.TagID) (bool, error)
	// CountArticleRefs 统计仍引用该标签的文章数（按关联表实时统计）。
	//
	// 删除门禁用它，而不是读实体上的 ArticleCount：读模型可能来自旧快照，
	// 门禁必须看"此刻还有谁在用"。
	CountArticleRefs(ctx context.Context, id domain.TagID) (int, error)
	// Save 新建或更新。
	//
	// 新建时若存在**同名软删行**（title 唯一索引不含 deleted_at，删除又是软删），
	// 实现必须恢复那一行而不是插入（房规见 platform/mysql.RestoreOrCreateUnique）：
	// 因此新建后库里的 ID 可能不是调用方给的那个。调用方不要依赖新建时的 ID
	// （写入接口不返回它，列表另行读取）。
	Save(ctx context.Context, t *Tag) error
	// Delete 软删标签。**不清理文章关联**：删除门禁（应用层按关联表实时统计
	// 引用数）已经保证被引用的标签不会走到这里，能删的标签本来就没有关联行。
	Delete(ctx context.Context, id domain.TagID) error
	// ReplaceArticleTags 用给定标签集合替换某文章的全部标签。
	ReplaceArticleTags(ctx context.Context, articleID domain.ArticleID, tagIDs []domain.TagID) error
	// ArticleTagIDs 读取某文章当前的标签集合。
	ArticleTagIDs(ctx context.Context, articleID domain.ArticleID) ([]domain.TagID, error)
}
