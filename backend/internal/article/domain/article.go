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
	SubmittedAt    *time.Time
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
		submittedAt:    p.SubmittedAt,
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
		// pending_review 是系统内部中间态：作者不能借"编辑文章"接口把
		// visibility_status 改成 visible 来提前放行、绕过模型合规判定
		// （ADR-0035 约束 3）。CanSetVisibility 只管管理员的改可见性端点，
		// 这条通用的 Update 路径必须自己再挡一次。
		// 不改原值就不报 flags.Visibility，纯改可见性的请求因此变成 no-op，
		// 不会返回"已修改"误导调用方。
		if a.visibility == VisibilityPendingReview && next == VisibilityVisible {
			next = VisibilityPendingReview
		}
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
// 允许的来源状态：草稿、已拒绝（人工驳回后改稿重投，#303）、审核中、已发布。
// 注意：**系统直接拒**（同步风险命中 / 合规自动拒）虽然也是 StatusRejected，
// 但由 app 层的 Update 提交分支按任务来源拦住，不在这里区分——domain 只认状态。
//
// 注意：票 302 新规则下 Create/Update 走的是 SubmitWithOutcome，因为"提交"
// 的落点不再只由 skipExamining 决定（还叠了同步风险扫描与闸门）。这里保留
// 给不需要新规则的调用方与历史用例，语义仍是旧的两态。
func (a *Article) Submit(now time.Time, skipExamining bool) error {
	return a.submitTo(ResolveSubmitStatus(skipExamining), VisibilityVisible, now)
}

// SubmitWithOutcome 按用例层算好的落点提交（票 302 新规则）。
//
// 落点可能是 Published + pending_review（干净文章，作者立刻拿到"已发布"
// 回执但对读者暂不可见）、Published + visible（站点免审核）、或
// Rejected（同步风险命中）。domain 不认识"风险扫描"，只认这个结论。
func (a *Article) SubmitWithOutcome(status Status, visibility Visibility, now time.Time) error {
	return a.submitTo(status, visibility, now)
}

func (a *Article) submitTo(next Status, visibility Visibility, now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	switch a.status {
	case StatusDraft, StatusExamining, StatusRejected, StatusPublished:
	default:
		return ErrInvalidTransition
	}

	if a.status == StatusPublished && next == StatusPublished && visibility == VisibilityVisible {
		// 站点免审核的幂等提交：目标态与当前态相同，且本来就是真发布。
		// 这里以前报 ErrAlreadyPublished，后果是**已发布的文章永远改不了**——
		// 用户点"更新"只会看到一句"文章已发布"，正文改动被整笔回滚。
		//
		// 注意条件里必须带 visibility == visible：票 302 新规则下"编辑已发布
		// 文章重新判定"的目标也是 Published，但可见性是 pending_review ——
		// 那不是幂等，改过的正文必须重新等对读者隐藏起来，否则编辑会绕过模型判定。
		return nil
	}

	a.status = next
	a.visibility = visibility
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
//
// 模型判 suspect 或判定失败会把文章从"已发布待可见"改回审核中（BackToReview），
// 管理员在这里做最终放行。若可见性还停在 pending_review，一并清成 visible：
// 审核通过就是最终决定，不该留一个"已发布但读者看不到"的中间态。
func (a *Article) Approve(reviewer UserID, now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status != StatusExamining {
		return ErrInvalidTransition
	}
	a.status = StatusPublished
	if a.visibility == VisibilityPendingReview {
		a.visibility = VisibilityVisible
	}
	t := now
	a.reviewedAt = &t
	by := reviewer
	a.reviewedBy = &by
	a.updatedAt = now
	return nil
}

// PublishForCompliance 模型判 clean 后把"已发布待可见"的文章真正放开。
//
// 它是异步合规判定的成功出口（票 #302 新规则）：提交时同步扫描没命中
// medium 及以上风险，文章已以 Published + pending_review 落库，作者立刻拿到
// "已发布"回执；只有模型判干净才对匿名访客可见。
//
// 幂等：已经是 visible 时直接返回 nil，outbox 重复投递不会产生额外写入。
func (a *Article) PublishForCompliance(now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status != StatusPublished || a.visibility != VisibilityPendingReview {
		return nil
	}
	a.visibility = VisibilityVisible
	a.updatedAt = now
	return nil
}

// BackToReview 把"已发布待可见"的文章改回审核中，交人工处置。
//
// 两个入口共用：模型判 suspect（存疑，误判风险高不自动拒）、以及模型判定
// 重试一次后仍失败（不能让文章永远停在"作者以为发了、读者看不到"的隐形拒稿）。
// 可见性一并清成 visible：审核中本身就不对读者可见（CanBeViewedBy），
// 状态语义保持单一，也能让管理员审核通过后直接发布。
func (a *Article) BackToReview(now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	if a.status != StatusPublished || a.visibility != VisibilityPendingReview {
		return ErrInvalidTransition
	}
	a.status = StatusExamining
	a.visibility = VisibilityVisible
	t := now
	a.submittedAt = &t
	a.updatedAt = now
	return nil
}

// Reject 审核拒绝。
// RevokeForCompliance 把一篇文章因合规判定置为「已拒绝」（票 313 / ADR-0034）。
//
// 与 Reject 的差别只有一处：允许的起点不同。Reject 是人工驳回，只从「审核中」
// 出发；这里允许从「已发布」出发——违规内容已经在公网上了，自动拒的意义就是
// 把它撤下来，而不是等它被人举报。已在审核中的文章同样适用（跳过人工）。
//
// reviewer 传系统占位（0），与人工驳回在同一字段上区分：0 表示系统自动。
func (a *Article) RevokeForCompliance(now time.Time) error {
	if a.deleted {
		return ErrArticleNotFound
	}
	switch a.status {
	case StatusExamining, StatusPublished:
	default:
		return ErrInvalidTransition
	}
	a.status = StatusRejected
	// 清掉 pending_review：已拒绝不该继续挂着一个"等放行"的中间态，
	// 否则后续任何把它改回审核中/发布的路径都得额外处理这个残留值。
	if a.visibility == VisibilityPendingReview {
		a.visibility = VisibilityVisible
	}
	t := now
	a.reviewedAt = &t
	a.reviewedBy = nil
	a.updatedAt = now
	return nil
}

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
// 项目现有语义就是软删（行结构内嵌 gorm.DeletedAt），这里保持一致：
// 不物理删除，也不提供硬删方法。
func (a *Article) SoftDelete(now time.Time) {
	a.deleted = true
	a.updatedAt = now
}

// SetCounters 用权威值覆盖当前计数（#279）。
//
// 读侧不再"各自把列与增量相加"：账本（app.CountLedger）算出权威值后整块写回，
// 聚合只负责携带，不参与合成。因此这里**没有** AddCounters —— 一旦保留"加一下"
// 的入口，第二套读口径就会重新长回来。
func (a *Article) SetCounters(c Counters) { a.counters = c }
