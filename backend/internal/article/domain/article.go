package domain

import (
	"strings"
	"time"
)

// Counters 是文章的各类计数。
//
// 真实值 = 库内快照 + 缓存增量；增量合并由 app 层负责，domain 只承载结果。
type Counters struct {
	View    int
	Digg    int
	Comment int
	Favor   int
}

// Article 是文章聚合根。
//
// 字段全部私有：外部只能通过构造、方法和只读访问器改变状态，
// 因此"状态能不能改成这样"的判断只有一份，不会散落在 55 个 handler 里。
type Article struct {
	id             ArticleID
	authorID       UserID
	title          string
	abstract       string
	content        string
	categoryID     *CategoryID
	cover          string
	commentsToggle bool
	status         Status
	visibility     Visibility
	submittedAt    *time.Time
	reviewedAt     *time.Time
	reviewedBy     *UserID
	deleted        bool
	counters       Counters
	tags           []TagID
	createdAt      time.Time
	updatedAt      time.Time
}

// NewArticleParams 是创建文章所需的全部输入。
//
// 不含 ID 生成与时间获取：两者都由调用方传入，domain 不依赖 time.Now 与雪花算法。
type NewArticleParams struct {
	ID             ArticleID
	AuthorID       UserID
	Title          string
	Abstract       string
	Content        string
	CategoryID     *CategoryID
	Cover          string
	CommentsToggle bool
	Status         Status
	Visibility     Visibility
	Now            time.Time
}

// NewArticle 创建一个文章聚合。
//
// 只做"创建时就必须成立"的校验（作者、标题、状态合法性）；
// 可见性走 NormalizeVisibility 归一，与旧实现一致。
func NewArticle(p NewArticleParams) (*Article, error) {
	if p.AuthorID.IsZero() {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(p.Title) == "" {
		return nil, ErrTitleRequired
	}
	if !p.Status.Valid() {
		return nil, ErrInvalidTransition
	}
	return &Article{
		id:             p.ID,
		authorID:       p.AuthorID,
		title:          p.Title,
		abstract:       p.Abstract,
		content:        p.Content,
		categoryID:     p.CategoryID,
		cover:          p.Cover,
		commentsToggle: p.CommentsToggle,
		status:         p.Status,
		visibility:     NormalizeVisibility(p.Visibility),
		createdAt:      p.Now,
		updatedAt:      p.Now,
	}, nil
}

// 只读访问器。写操作必须走方法。

// ID 返回文章 ID。
func (a *Article) ID() ArticleID { return a.id }

// AuthorID 返回作者 ID。
func (a *Article) AuthorID() UserID { return a.authorID }

// Title 返回标题。
func (a *Article) Title() string { return a.title }

// Abstract 返回摘要。
func (a *Article) Abstract() string { return a.abstract }

// Content 返回正文。
func (a *Article) Content() string { return a.content }

// CategoryID 返回分类；未设置时为 nil。
func (a *Article) CategoryID() *CategoryID { return a.categoryID }

// Cover 返回封面地址。
func (a *Article) Cover() string { return a.cover }

// CommentsToggle 返回是否允许评论。
func (a *Article) CommentsToggle() bool { return a.commentsToggle }

// Status 返回发布状态。
func (a *Article) Status() Status { return a.status }

// Visibility 返回可见性。
func (a *Article) Visibility() Visibility { return a.visibility }

// SubmittedAt 返回提交审核时间。
func (a *Article) SubmittedAt() *time.Time { return a.submittedAt }

// ReviewedAt 返回审核时间。
func (a *Article) ReviewedAt() *time.Time { return a.reviewedAt }

// ReviewedBy 返回审核人。
func (a *Article) ReviewedBy() *UserID { return a.reviewedBy }

// Deleted 返回是否已软删。
func (a *Article) Deleted() bool { return a.deleted }

// Counters 返回计数快照。
func (a *Article) Counters() Counters { return a.counters }

// CreatedAt 返回创建时间。
func (a *Article) CreatedAt() time.Time { return a.createdAt }

// UpdatedAt 返回更新时间。
func (a *Article) UpdatedAt() time.Time { return a.updatedAt }

// IsPubliclyVisible 判断是否"对匿名访客可见"。
//
// 等价于旧实现 IsPublicVisible()：已发布 且 可见性为 visible。
func (a *Article) IsPubliclyVisible() bool {
	return !a.deleted && a.status == StatusPublished && a.visibility == VisibilityVisible
}

// Tags 返回标签 ID 副本，防止调用方改到内部切片。
func (a *Article) Tags() []TagID {
	out := make([]TagID, len(a.tags))
	copy(out, a.tags)
	return out
}

// SetTags 替换标签集合。
//
// 只存 ID 不存 Tag 实体：标签的标题与排序属于 taxonomy 子域，
// 放进聚合根会让两个聚合互相依赖。
func (a *Article) SetTags(ids []TagID, now time.Time) {
	out := make([]TagID, len(ids))
	copy(out, ids)
	a.tags = out
	a.updatedAt = now
}

// EditFlags 记录一次编辑动了哪些字段。
//
// 用布尔而不是比对新旧值：是否"真的变了"由 app 结合数据库现状判断，
// domain 只关心"哪些字段参与了编辑"，因为重新审核的判定正是按字段类别算的。
type EditFlags struct {
	Title          bool
	Abstract       bool
	Content        bool
	Category       bool
	Tags           bool
	Cover          bool
	CommentsToggle bool
	Visibility     bool
}

// HasSubstantive 判断是否改了"实质性内容"。
//
// 注意 Visibility 不算：旧实现里只改可见性不会触发重新审核。
func (f EditFlags) HasSubstantive() bool {
	return f.Title || f.Abstract || f.Content || f.Category || f.Tags || f.Cover || f.CommentsToggle
}

// EditPatch 是一次编辑的输入；nil 表示该字段未提供。
type EditPatch struct {
	Title          *string
	Abstract       *string
	Content        *string
	CategoryID     *CategoryID // 非 nil 时写入，nil 且 CategoryCleared 为真时清空
	CategorySet    bool
	Cover          *string
	CommentsToggle *bool
	Visibility     *Visibility
}

// ApplyEdit 应用内容字段的修改，返回本次实际改动的字段标记。
//
// 只改字段，不改状态；状态迁移由调用方根据 Flags 决定调 Submit / BackToDraft。
// 这样"编辑"与"状态机"两件事各自可测。
func (a *Article) ApplyEdit(p EditPatch, now time.Time) (EditFlags, error) {
	var flags EditFlags

	if p.Title != nil {
		if strings.TrimSpace(*p.Title) == "" {
			return flags, ErrTitleRequired
		}
		if a.title != *p.Title {
			a.title = *p.Title
			flags.Title = true
		}
	}
	if p.Abstract != nil && a.abstract != *p.Abstract {
		a.abstract = *p.Abstract
		flags.Abstract = true
	}
	if p.Content != nil {
		if strings.TrimSpace(*p.Content) == "" {
			return flags, ErrContentRequired
		}
		if a.content != *p.Content {
			a.content = *p.Content
			flags.Content = true
		}
	}
	if p.CategorySet {
		if p.CategoryID == nil {
			if a.categoryID != nil {
				a.categoryID = nil
				flags.Category = true
			}
		} else if a.categoryID == nil || *a.categoryID != *p.CategoryID {
			id := *p.CategoryID
			a.categoryID = &id
			flags.Category = true
		}
	}
	if p.Cover != nil && a.cover != *p.Cover {
		a.cover = *p.Cover
		flags.Cover = true
	}
	if p.CommentsToggle != nil && a.commentsToggle != *p.CommentsToggle {
		a.commentsToggle = *p.CommentsToggle
		flags.CommentsToggle = true
	}
	if p.Visibility != nil {
		next := NormalizeVisibility(*p.Visibility)
		if a.visibility != next {
			a.visibility = next
			flags.Visibility = true
		}
	}

	if flags != (EditFlags{}) {
		a.updatedAt = now
	}
	return flags, nil
}

// MarkTagsChanged 记录标签变更（标签写入由 app 调 taxonomy 仓储完成）。
func (a *Article) MarkTagsChanged(now time.Time) {
	a.updatedAt = now
}

// RestoreParams 是从持久化层重建聚合所需的全部字段。
type RestoreParams struct {
	ID             ArticleID
	AuthorID       UserID
	Title          string
	Abstract       string
	Content        string
	CategoryID     *CategoryID
	Cover          string
	CommentsToggle bool
	Status         Status
	Visibility     Visibility
	SubmittedAt    *time.Time
	ReviewedAt     *time.Time
	ReviewedBy     *UserID
	Deleted        bool
	Counters       Counters
	Tags           []TagID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Restore 从持久化数据重建聚合。
//
// 不做业务校验：库里已有的数据即使不合新规则也必须能读出来，
// 否则一条历史脏数据就能让整页 500。校验只发生在"写入路径"。
func Restore(p RestoreParams) *Article {
	tags := make([]TagID, len(p.Tags))
	copy(tags, p.Tags)
	return &Article{
		id:             p.ID,
		authorID:       p.AuthorID,
		title:          p.Title,
		abstract:       p.Abstract,
		content:        p.Content,
		categoryID:     p.CategoryID,
		cover:          p.Cover,
		commentsToggle: p.CommentsToggle,
		status:         p.Status,
		visibility:     NormalizeVisibility(p.Visibility),
		submittedAt:    p.SubmittedAt,
		reviewedAt:     p.ReviewedAt,
		reviewedBy:     p.ReviewedBy,
		deleted:        p.Deleted,
		counters:       p.Counters,
		tags:           tags,
		createdAt:      p.CreatedAt,
		updatedAt:      p.UpdatedAt,
	}
}

// NeedsReReview 判断一次编辑后是否应重新进入审核。
//
// 与旧实现 shouldReSubmitAfterEdit 等价：
//   - 站点未跳过审核；
//   - 改的是实质内容（可见性不算）；
//   - 文章当前已是"已发布"或"审核中"。
func (a *Article) NeedsReReview(flags EditFlags, skipExamining bool) bool {
	if skipExamining {
		return false
	}
	if !flags.HasSubstantive() {
		return false
	}
	return a.status == StatusPublished || a.status == StatusExamining
}

// Submit 提交审核（或按站点配置直接发布）。
//
// 允许的来源状态：草稿、已拒绝、审核中（重复提交即重新排审）、已发布（编辑后重审）。
func (a *Article) Submit(now time.Time, skipExamining bool) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	switch a.status {
	case StatusDraft, StatusExamining, StatusRejected, StatusPublished:
	default:
		return ErrInvalidTransition
	}

	next := ResolveSubmitStatus(skipExamining)
	if a.status == StatusPublished && next == StatusPublished {
		return ErrAlreadyPublished
	}

	a.status = next
	t := now
	a.submittedAt = &t
	a.reviewedAt = nil
	a.reviewedBy = nil
	a.updatedAt = now
	return nil
}

// BackToDraft 作者把文章退回草稿，并清空审核痕迹。
func (a *Article) BackToDraft(now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status == StatusDraft {
		return nil // 幂等：已经是草稿不算错误
	}
	a.status = StatusDraft
	a.submittedAt = nil
	a.reviewedAt = nil
	a.reviewedBy = nil
	a.updatedAt = now
	return nil
}

// Approve 审核通过。
func (a *Article) Approve(reviewer UserID, now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status != StatusExamining {
		return ErrInvalidTransition
	}
	a.status = StatusPublished
	t := now
	a.reviewedAt = &t
	by := reviewer
	a.reviewedBy = &by
	a.updatedAt = now
	return nil
}

// Reject 审核拒绝。
func (a *Article) Reject(reviewer UserID, now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status != StatusExamining {
		return ErrInvalidTransition
	}
	a.status = StatusRejected
	t := now
	a.reviewedAt = &t
	by := reviewer
	a.reviewedBy = &by
	a.updatedAt = now
	return nil
}

// SoftDelete 标记删除。
//
// 项目现有语义就是软删（models.Model 内嵌 gorm.DeletedAt），这里保持一致：
// 不物理删除，也不提供硬删方法。
func (a *Article) SoftDelete(now time.Time) {
	a.deleted = true
	a.updatedAt = now
}

// AddCounters 用缓存增量修正计数。
func (a *Article) AddCounters(delta Counters) {
	a.counters.View += delta.View
	a.counters.Digg += delta.Digg
	a.counters.Comment += delta.Comment
	a.counters.Favor += delta.Favor
}
