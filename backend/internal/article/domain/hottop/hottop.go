// Package hottop 是「热门置顶」运营位子域。
//
// 与 domain/top（作者置顶）是**两套数据、两套规则**，不要混用：
//   - 作者置顶：作者把自己的一篇文章钉在个人主页，表 user_top_article_models；
//   - 热门置顶：管理员把任意「已发布且可见」的文章放到首页运营位，
//     表 article_hot_top_models，最多 MaxItems 条、可整体排序。
//
// 本包只放"什么是合法置顶"的纯规则与读模型，不含 SQL 也不认识 gin。
// 取舍见 docs/adr/0017-publish-display-switches-and-hot-top-operator-slot.md 决策 5。
package hottop

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// MaxItems 是热门置顶的条数上限。
//
// 12 来自首页版式（ADR-0017 决策 5）。上限只在 app 层用，不靠数据库约束：
// 零 DDL 口径下加不了 CHECK，而"第 13 条"要给出可读错误而不是 1062。
const MaxItems = 12

// 领域错误。
//
// 与 domain/errors.go 同一套做法：只写错误码，中文文案在 internal/apperr。
var (
	// ErrLimitExceeded 置顶条数已达上限。
	ErrLimitExceeded = apperr.New(apperr.CodeHotTopLimitExceeded)
	// ErrNotPinnable 文章不是"已发布且可见"，不能置顶。
	ErrNotPinnable = apperr.New(apperr.CodeHotTopNotPinnable)
	// ErrOrderMismatch 排序输入与当前置顶集合不一致。
	ErrOrderMismatch = apperr.New(apperr.CodeHotTopOrderMismatch)
)

// CanBePinned 判断一篇文章能否设为热门置顶。
//
// 规则只有一条：它此刻必须对匿名访客可见（已发布 + visible + 未删除）。
// 为什么复用 IsPubliclyVisible 而不是自己再判一次：两者一旦分叉就会出现
// "首页有一张点进去 404 的置顶卡片"。
func CanBePinned(a *domain.Article) bool {
	return a != nil && a.IsPubliclyVisible()
}

// Entry 是一条热门置顶关系：哪篇文章、排第几。
//
// 唯一约束是 article_id（一篇文章最多出现一次），与 uk_article_hot_top 一致。
// SortOrder 从 1 开始连续编号；取消一条后由仓储压实，避免序号越用越大。
type Entry struct {
	articleID domain.ArticleID
	sortOrder int
}

// NewEntry 建立置顶关系。
func NewEntry(articleID domain.ArticleID, sortOrder int) *Entry {
	return &Entry{articleID: articleID, sortOrder: sortOrder}
}

// RestoreEntry 从持久化数据恢复。
func RestoreEntry(articleID domain.ArticleID, sortOrder int) *Entry {
	return &Entry{articleID: articleID, sortOrder: sortOrder}
}

// ArticleID 返回文章。
func (e *Entry) ArticleID() domain.ArticleID { return e.articleID }

// SortOrder 返回序号（从 1 开始）。
func (e *Entry) SortOrder() int { return e.sortOrder }

// Card 是读者站「热门置顶」区块的一张卡片。
//
// 只需要下面几个字段就能渲染整块区域；只给 ID 会让首页逐篇再查详情（N+1）。
// 字段与 ArticleHotTopCardResponse 一一对应。
type Card struct {
	ArticleID      domain.ArticleID
	Title          string
	Abstract       string
	Cover          string
	ViewCount      int
	AuthorNickname string
	CategoryTitle  string
}

// ArticleState 是置顶行所指文章**此刻**的状态。
//
// 与 Card 分开是因为职责不同：Card 是"还能展示的样子"，
// ArticleState 是"它怎么样了"——后台的「已失效」标记只读后者。
// nil 表示文章已被物理删除（置顶行还在，join 不到文章）。
type ArticleState struct {
	// Deleted 文章已被软删（行还在，deleted_at 非空）。
	Deleted bool
	// Status 发布状态。
	Status domain.Status
	// Visibility 可见性。
	Visibility domain.Visibility
}

// IsStale 判断这条置顶是否已失效。
//
// 失效 = 文章没了 / 被删 / 不再"已发布且可见"。置顶行本身**保留**：
// 首页靠 JOIN 过滤掉，后台标「已失效」并允许一键清理（ADR-0017 决策 5）。
// 方法可以在 nil 接收者上调用，所以"文章已物理删除"也走同一条判定。
func (s *ArticleState) IsStale() bool {
	if s == nil {
		return true
	}
	return s.Deleted || s.Status != domain.StatusPublished || s.Visibility != domain.VisibilityVisible
}

// AdminItem 是后台「热门置顶」页的一行：序号 + 文章摘要 + 是否失效。
//
// 后台要能看见失效行（否则没法清理），所以它与 Card 是两份读模型：
// Card 只含在架文章，AdminItem 含全部置顶行。
type AdminItem struct {
	ArticleID domain.ArticleID
	SortOrder int
	Title     string
	Cover     string
	// Article 是文章当前状态；文章已物理删除时为 nil。
	Article *ArticleState
}

// Stale 判断这一行是否已失效。
func (i AdminItem) Stale() bool { return i.Article.IsStale() }

// CheckOrder 校验"全量顺序替换"的输入。
//
// next 必须是 current 的一个排列：同一批文章、每条恰好一次。
// 不允许"缺的删掉"这种宽松语义——PATCH 是后台排序按钮的产物，
// 一个过期的页面点一次「上移」就会把别人刚加的那条置顶静默删掉。
func CheckOrder(current, next []domain.ArticleID) error {
	if len(next) > MaxItems {
		return ErrLimitExceeded
	}
	if len(next) != len(current) {
		return ErrOrderMismatch
	}

	want := make(map[domain.ArticleID]struct{}, len(current))
	for _, id := range current {
		want[id] = struct{}{}
	}
	seen := make(map[domain.ArticleID]struct{}, len(next))
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

// Repository 是热门置顶的持久化契约。
//
// 所有写方法都要求调用方已在事务内（app 的 WithinTx），
// 因为"置顶/取消/重排"都可能同时改多行（压实序号、重排）。
type Repository interface {
	// Save 新增置顶，返回是否真的新增（已存在时返回 false，幂等）。
	//
	// 新条目排在末尾；曾经软删过的文章重新置顶时恢复原行，不产生第二行
	// （article_id 上有唯一索引）。
	Save(ctx context.Context, articleID domain.ArticleID) (created bool, err error)
	// Delete 取消置顶并压实剩余序号；不存在视为成功（幂等）。
	Delete(ctx context.Context, articleID domain.ArticleID) error
	// Exists 判断是否已置顶（行存在且未软删）。
	Exists(ctx context.Context, articleID domain.ArticleID) (bool, error)
	// Count 统计当前置顶条数（不含软删行）。
	Count(ctx context.Context) (int, error)
	// ListIDs 按序号返回当前置顶的文章 ID，用于校验全量顺序替换。
	ListIDs(ctx context.Context) ([]domain.ArticleID, error)
	// ListCards 列出**当前仍公开**的置顶卡片（按序号升序）。
	//
	// 失效的置顶行不出现：首页容器靠这条查询过滤，不靠前端判断。
	//
	// onlyAdminAuthors 为真时只返回"作者是管理员"的卡片（ADR-0017 决策 4：
	// 热门置顶属于读者端公开读面，与收藏内容 / 浏览历史同一口径）。
	ListCards(ctx context.Context, onlyAdminAuthors bool) ([]Card, error)
	// ListAdminItems 列出**全部**置顶行（含已失效），按序号升序。
	ListAdminItems(ctx context.Context) ([]AdminItem, error)
	// UpdateSortOrder 按给定顺序把序号改成 1..n（调用方已用 CheckOrder 校验）。
	UpdateSortOrder(ctx context.Context, ids []domain.ArticleID) error
}
