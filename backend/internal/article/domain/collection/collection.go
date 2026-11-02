// Package collection 是「合集」子域：管理员编排的一组有顺序的文章。
//
// 与「热门置顶」是两套数据、两套规则，不要混用：
//   - 热门置顶：首页运营位，上限 12 条，表 article_hot_top_models；
//   - 合集：有标题/描述/封面/展示开关的有序文章集合，
//     表 collection_models + collection_article_models。
//
// 合集是**管理员专属**的运营产物：作者不参与创建，读者只读。
// 本包只放"什么是合法合集"的纯规则与读模型，不含 SQL 也不认识 gin。
package collection

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 类型别名。
//
// ID / ArticleID 都是 ctype.ID 的别名（与 domain/id.go 同一做法）：
// 底层是 string，契约里就是 string，不需要生成后再修补。
type (
	// ID 是合集标识。
	ID = ctype.ID
	// ArticleID 是被收录文章的标识。
	ArticleID = ctype.ID
)

// 标题长度上限。
//
// 与文章标题（varchar(256)）不同：合集卡片只显示一行标题，128 足够，
// 且卡片布局在窄屏下不会被长标题撑破。上限只在 app 层校验，不靠数据库约束
// （零 DDL 口径下加不了 CHECK，超长会给 1406 而不是可读错误）。
const MaxTitleLen = 128

// 描述长度上限：卡片上就是一段两行以内的说明。
const MaxDescriptionLen = 512

// 领域错误。
//
// 与 domain/errors.go 同一套做法：只写错误码，中文文案在 internal/apperr。
var (
	// ErrTitleRequired 标题为空。
	ErrTitleRequired = apperr.New(apperr.CodeCollectionTitleRequired)
	// ErrTitleTooLong 标题超出 MaxTitleLen。
	ErrTitleTooLong = apperr.New(apperr.CodeCollectionTitleRequired)
	// ErrNotFound 合集不存在（也覆盖"未展示，对读者不可见"）。
	ErrNotFound = apperr.New(apperr.CodeCollectionNotFound)
	// ErrDuplicateArticle 同一合集内重复收录同一篇文章。
	ErrDuplicateArticle = apperr.New(apperr.CodeCollectionArticleDuplicated)
	// ErrNotIncludable 文章不是"已发布且可见"，不能收录。
	ErrNotIncludable = apperr.New(apperr.CodeCollectionArticleNotIncludable)
	// ErrOrderMismatch 顺序替换的输入与当前收录集合不一致。
	ErrOrderMismatch = apperr.New(apperr.CodeCollectionOrderMismatch)
	// ErrArticleMissing 要移除的收录不存在。
	ErrArticleMissing = apperr.New(apperr.CodeCollectionArticleMissing)
)

// Collection 是合集聚合根。
//
// 字段全部私有：外部只能通过构造与方法改变状态，"什么算合法合集"只有一份判断。
type Collection struct {
	id          ID
	title       string
	description string
	cover       string
	creatorID   domain.UserID
	displayed   bool
	sortOrder   int
	deleted     bool
	createdAt   time.Time
	updatedAt   time.Time
}

// NewParams 是创建合集所需的全部输入。
//
// 不含 ID 生成与时间获取：两者都由调用方传入，domain 不依赖 time.Now 与雪花算法。
type NewParams struct {
	ID          ID
	Title       string
	Description string
	Cover       string
	CreatorID   domain.UserID
	Displayed   bool
	SortOrder   int
	Now         time.Time
}

// New 创建一个合集聚合。
//
// 只做"创建时就必须成立"的校验：标题非空且不超长。展示开关、封面、描述
// 都可以为空——一张没有封面的合集卡片要能退回到首篇文章的封面（由读模型负责）。
func New(p NewParams) (*Collection, error) {
	if err := checkTitle(p.Title); err != nil {
		return nil, err
	}
	return &Collection{
		id:          p.ID,
		title:       strings.TrimSpace(p.Title),
		description: p.Description,
		cover:       p.Cover,
		creatorID:   p.CreatorID,
		displayed:   p.Displayed,
		sortOrder:   p.SortOrder,
		createdAt:   p.Now,
		updatedAt:   p.Now,
	}, nil
}

// RestoreParams 是从持久化数据重建聚合的输入。
type RestoreParams struct {
	ID          ID
	Title       string
	Description string
	Cover       string
	CreatorID   domain.UserID
	Displayed   bool
	SortOrder   int
	Deleted     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Restore 从持久化数据重建聚合（不做校验：库里已有的行必须能读出来）。
func Restore(p RestoreParams) *Collection {
	return &Collection{
		id:          p.ID,
		title:       p.Title,
		description: p.Description,
		cover:       p.Cover,
		creatorID:   p.CreatorID,
		displayed:   p.Displayed,
		sortOrder:   p.SortOrder,
		deleted:     p.Deleted,
		createdAt:   p.CreatedAt,
		updatedAt:   p.UpdatedAt,
	}
}

// 只读访问器。

// ID 返回合集 ID。
func (c *Collection) ID() ID { return c.id }

// Title 返回标题。
func (c *Collection) Title() string { return c.title }

// Description 返回描述。
func (c *Collection) Description() string { return c.description }

// Cover 返回封面 URL。
func (c *Collection) Cover() string { return c.cover }

// CreatorID 返回创建者（管理员）ID。
func (c *Collection) CreatorID() domain.UserID { return c.creatorID }

// Displayed 返回是否展示给读者。
func (c *Collection) Displayed() bool { return c.displayed }

// SortOrder 返回合集之间的展示顺序（从 1 开始）。
func (c *Collection) SortOrder() int { return c.sortOrder }

// Deleted 返回是否已软删。
func (c *Collection) Deleted() bool { return c.deleted }

// CreatedAt 返回创建时间。
func (c *Collection) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt 返回最后修改时间。
func (c *Collection) UpdatedAt() time.Time { return c.updatedAt }

// Retitle 改标题，走与创建同一条不变量。
func (c *Collection) Retitle(title string, now time.Time) error {
	if err := checkTitle(title); err != nil {
		return err
	}
	c.title = strings.TrimSpace(title)
	c.updatedAt = now
	return nil
}

// SetDescription 改描述（可清空）。
func (c *Collection) SetDescription(description string, now time.Time) {
	c.description = description
	c.updatedAt = now
}

// SetCover 改封面（传空串表示"退回首篇文章的封面"）。
func (c *Collection) SetCover(cover string, now time.Time) {
	c.cover = cover
	c.updatedAt = now
}

// SetDisplay 打开/关闭"是否展示给读者"。
//
// 关掉后读者侧列表与详情都不出现（详情按 404 处理），后台仍可见。
func (c *Collection) SetDisplay(displayed bool, now time.Time) {
	c.displayed = displayed
	c.updatedAt = now
}

// SetSortOrder 设置合集之间的展示顺序。
func (c *Collection) SetSortOrder(order int, now time.Time) {
	c.sortOrder = order
	c.updatedAt = now
}

// SoftDelete 标记删除（不物理删）。
//
// 删合集**不删文章**：中间表的行留着（文章本身与本合集无关），
// 读者侧因为合集已经查不到而整块消失。
func (c *Collection) SoftDelete(now time.Time) {
	c.deleted = true
	c.updatedAt = now
}

// checkTitle 是标题的唯一条不变量检查。
func checkTitle(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrTitleRequired
	}
	if len([]rune(trimmed)) > MaxTitleLen {
		return ErrTitleTooLong
	}
	return nil
}

// CanBeIncluded 判断一篇文章能否被收录进合集。
//
// 规则只有一条：它此刻必须对匿名访客可见（已发布 + visible + 未删除）。
// 为什么复用 IsPubliclyVisible 而不自己再判一次：两者一旦分叉就会出现
// "合集里有一张点进去 404 的条目"。
//
// 注意这只约束**收录动作**。已经收录的文章后来变成私有/删除时，中间行保留
// （后台标「已失效」并可清理），读者侧靠 JOIN 过滤掉——与热门置顶逐字一致。
func CanBeIncluded(a *domain.Article) bool {
	return a != nil && a.IsPubliclyVisible()
}

// ArticleState 是收录行所指文章**此刻**的状态。
//
// 与 IncludedArticle 分开是因为职责不同：后者是"还能展示的样子"，
// ArticleState 是"它怎么样了"——后台的「已失效」标记只读后者。
// nil 表示文章已被物理删除（中间行还在，join 不到文章）。
type ArticleState struct {
	// Deleted 文章已被软删（行还在，deleted_at 非空）。
	Deleted bool
	// Status 发布状态。
	Status domain.Status
	// Visibility 可见性。
	Visibility domain.Visibility
}

// IsStale 判断这条收录是否已失效。
//
// 失效 = 文章没了 / 被删 / 不再"已发布且可见"。中间行本身**保留**：
// 读者侧靠 JOIN 过滤掉，后台标「已失效」并允许一键清理。
// 方法可以在 nil 接收者上调用，所以"文章已物理删除"也走同一条判定。
func (s *ArticleState) IsStale() bool {
	if s == nil {
		return true
	}
	return s.Deleted || s.Status != domain.StatusPublished || s.Visibility != domain.VisibilityVisible
}

// Card 是读者站合集列表页的一张卡片。
//
// 只需要下面这些字段就能渲染整块卡片；只给 ID 会让列表页逐个再查详情（N+1）。
// ArticleCount 只数**仍公开**的文章：一张写着"20 篇"点进去只有 18 篇是坏体验。
type Card struct {
	ID           ID
	Title        string
	Description  string
	Cover        string
	ArticleCount int
	CreatedAt    time.Time
}

// IncludedArticle 是合集详情页里的一篇文章（按顺序排列）。
type IncludedArticle struct {
	ArticleID      ArticleID
	SortOrder      int
	Title          string
	Abstract       string
	Cover          string
	ViewCount      int
	AuthorNickname string
	CategoryTitle  string
}

// AdminCard 是后台合集列表的一行：含**未展示**的合集与收录条数。
//
// 后台要能看见未展示的合集（否则没法准备一个还没发布的专题），
// 所以它与 Card 是两份读模型。
type AdminCard struct {
	ID           ID
	Title        string
	Description  string
	Cover        string
	Displayed    bool
	ArticleCount int
	SortOrder    int
}

// AdminItem 是后台「合集收录」页的一行：顺序 + 文章摘要 + 是否失效。
type AdminItem struct {
	ArticleID ArticleID
	SortOrder int
	Title     string
	Cover     string
	// Article 是文章当前状态；文章已物理删除时为 nil。
	Article *ArticleState
}

// Stale 判断这一行是否已失效。
func (i AdminItem) Stale() bool { return i.Article.IsStale() }

// Detail 是合集详情：合集本身 + 按顺序排列的已公开文章。
//
// 为什么在 domain 而不是 app 拼：它是"读者看到的合集"的完整读模型，
// 一旦分叉（比如详情页少过滤一次失效行），入口就会多一张死链卡片。
type Detail struct {
	Collection *Collection
	Articles   []IncludedArticle
}

// CheckOrder 校验"全量顺序替换"的输入。
//
// next 必须是 current 的一个排列：同一批文章、每条恰好一次。
// 不允许"缺的删掉"这种宽松语义——PATCH 是后台排序按钮的产物，
// 一个过期的页面点一次「上移」就会把别人刚收录的那篇静默删掉。
func CheckOrder(current, next []ArticleID) error {
	if len(next) != len(current) {
		return ErrOrderMismatch
	}
	want := make(map[ArticleID]struct{}, len(current))
	for _, id := range current {
		want[id] = struct{}{}
	}
	seen := make(map[ArticleID]struct{}, len(next))
	for _, id := range next {
		if _, ok := want[id]; !ok {
			return ErrOrderMismatch
		}
		if _, dup := seen[id]; dup {
			return ErrOrderMismatch
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Repository 是合集的持久化契约。
//
// 所有写方法都要求调用方已在事务内（app 的 WithinTx），因为"收录/移除/重排"
// 都可能同时改多行（压实序号、重排）。
type Repository interface {
	// Create 插入合集；ID 必须由调用方给好。
	Create(ctx context.Context, c *Collection) error

	// Update 保存已有合集的变更（标题/描述/封面/展示开关/顺序）。
	Update(ctx context.Context, c *Collection) error

	// Find 按 ID 读取；不存在时返回 ErrNotFound。
	//
	// **包含未展示与已软删之外的合集**：展示开关的判定在 app 层（读者侧
	// 拿到未展示的合集一律按不存在处理），后台要能读到未展示的合集来编辑它。
	Find(ctx context.Context, id ID) (*Collection, error)

	// ListCards 列出**展示中**的合集卡片（按 sort_order, id 升序）。
	//
	// onlyAdminAuthors 为真时，把"一篇公开文章都没有"（或收录文章的作者
	// 全是非管理员）的合集也过滤掉——站点「显示非管理员发布文章」开关关闭时，
	// 合集是读者端公开读面，与首页列表、详情同一口径（ADR-0017 决策 4）。
	ListCards(ctx context.Context, onlyAdminAuthors bool, p domain.Page) ([]Card, int64, error)

	// ListAdminCards 列出全部合集（含未展示），后台列表用。
	ListAdminCards(ctx context.Context) ([]AdminCard, error)

	// NextSortOrder 返回下一个可用的合集顺序号（当前最大 + 1，空表为 1）。
	//
	// 与热门置顶同一做法：序号没有唯一索引（零 DDL 加不了），
	// 两个管理员同时新建可能算出同一个号，读出一律用 (sort_order, id) 兜底，
	// 同号只退化成"按新建顺序"，既不丢数据也不阻塞。
	NextSortOrder(ctx context.Context) (int, error)

	// ListItemIDs 按顺序返回合集当前收录的文章 ID（不含软删行）。
	ListItemIDs(ctx context.Context, id ID) ([]ArticleID, error)

	// AppendItem 收录一篇文章，返回是否真的新增（已存在时幂等返回 false）。
	//
	// 新条目排在末尾；曾经移除过的文章重新收录时恢复原行，不产生第二行
	// （(collection_id, article_id) 上有唯一索引）。
	AppendItem(ctx context.Context, id ID, articleID ArticleID) (created bool, err error)

	// RemoveItem 移除收录（软删）并压实剩余序号；不存在视为成功（幂等）。
	RemoveItem(ctx context.Context, id ID, articleID ArticleID) error

	// ListIncludedArticles 列出**仍公开**的收录文章，按顺序升序。
	//
	// 失效行不出现：读者侧靠这条查询过滤，不靠前端判断。
	ListIncludedArticles(ctx context.Context, id ID, onlyAdminAuthors bool) ([]IncludedArticle, error)

	// ListAdminItems 列出**全部**收录行（含已失效），按顺序升序。
	ListAdminItems(ctx context.Context, id ID) ([]AdminItem, error)

	// UpdateItemOrder 按给定顺序把收录序号改成 1..n（调用方已用 CheckOrder 校验）。
	UpdateItemOrder(ctx context.Context, id ID, articleIDs []ArticleID) error
}
