// Package viewhistory 是"用户浏览文章历史"子域。
//
// 语义上是一次 upsert：同一 (userID, articleID) 只保留一条记录，
// 再次浏览刷新时间并顶到最前，而不是追加一条。与库里的
// uk_user_article_history 唯一键一致。
package viewhistory

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/article/domain"
)

// Retention 是浏览历史的保留期：超过这个期限的记录由每日清理任务软删。
//
// 90 天是**数据策略**（会真实让用户的历史消失），不是可随手调的风格参数：
// 改动它要同时更新发布说明（决策清单里写明了这条取舍）。
const Retention = 90 * 24 * time.Hour

// PurgeCutoff 返回清理的时间界限：浏览时间早于它的记录应被清理。
//
// 时间由调用方传入（domain 不取当前时间，见 archcheck R4）；生产装配处传
// time.Now()，用例传固定时刻 —— 90 天的边界必须能被钉死。
// 边界语义是**严格小于** cutoff：恰好 90 天前的记录保留到下一轮。
func PurgeCutoff(now time.Time) time.Time { return now.Add(-Retention) }

// Record 是一条浏览记录。
type Record struct {
	articleID domain.ArticleID
	userID    domain.UserID
	viewedAt  time.Time

	// 读模型字段：列表页展示的文章与用户信息（避免 HTTP 层回查）。
	articleTitle string
	articleCover string
	nickname     string
	avatar       string
}

// NewRecord 生成浏览记录。
func NewRecord(userID domain.UserID, articleID domain.ArticleID, now time.Time) *Record {
	return &Record{articleID: articleID, userID: userID, viewedAt: now}
}

// RestoreRecordParams 是带读模型字段的重建参数。
type RestoreRecordParams struct {
	ArticleID    domain.ArticleID
	UserID       domain.UserID
	ViewedAt     time.Time
	ArticleTitle string
	ArticleCover string
	Nickname     string
	Avatar       string
}

// RestoreRecord 从持久化数据重建（含列表页所需读模型字段）。
func RestoreRecord(p RestoreRecordParams) *Record {
	return &Record{
		articleID:    p.ArticleID,
		userID:       p.UserID,
		viewedAt:     p.ViewedAt,
		articleTitle: p.ArticleTitle,
		articleCover: p.ArticleCover,
		nickname:     p.Nickname,
		avatar:       p.Avatar,
	}
}

// ArticleID 返回文章。
func (r *Record) ArticleID() domain.ArticleID { return r.articleID }

// UserID 返回用户。
func (r *Record) UserID() domain.UserID { return r.userID }

// ViewedAt 返回浏览时间。
func (r *Record) ViewedAt() time.Time { return r.viewedAt }

// ArticleTitle 返回文章标题（读模型）。
func (r *Record) ArticleTitle() string { return r.articleTitle }

// ArticleCover 返回文章封面（读模型）。
func (r *Record) ArticleCover() string { return r.articleCover }

// Nickname 返回浏览者昵称（读模型）。
func (r *Record) Nickname() string { return r.nickname }

// Avatar 返回浏览者头像（读模型）。
func (r *Record) Avatar() string { return r.avatar }

// Touch 刷新浏览时间（重复浏览时调用）。
func (r *Record) Touch(now time.Time) { r.viewedAt = now }

// Cursor 是浏览历史的游标位置：(ViewedAt, ArticleID) 的倒序后缀。
//
// 为什么用游标而不是 offset：列表按天分组（今天 / 昨天 / 近一周 / 更早），
// offset 在两个时刻之间夹着删除时会让下一页整体前移——被挤掉的那条会让
// 「昨天」这个分组跨页断开；重复浏览把某条记录顶到最前时也会造成同样的位移。
// 用「严格早于上一次最后一条」的游标，删除与置顶都不会影响后续页。
//
// ArticleID 作为第二排序键而不是历史行的自增 ID：契约里已经暴露 article_id，
// 客户端不需要再拿一个隐藏的 row id 就能续页；(user_id, article_id) 唯一，
// 所以 (ViewedAt, ArticleID) 对同一用户唯一定位一条记录。
type Cursor struct {
	ViewedAt  time.Time
	ArticleID domain.ArticleID
}

// ListQuery 是浏览历史的列表查询。
type ListQuery struct {
	// UserID 是历史的主人（由 viewer 决定，不能来自请求参数）。
	UserID domain.UserID
	// Keyword 非空时按文章标题模糊匹配（历史表本身只存关联，标题取自文章表）。
	Keyword string
	// OnlyAdminAuthors 为真时只返回"文章作者是管理员"的记录（ADR-0015 决策 4：
	// 「显示非管理员发布文章」开关关闭时浏览历史同步过滤）。
	OnlyAdminAuthors bool
	// Before 为 nil 表示第一页；非 nil 表示只取严格早于该位置的记录。
	Before *Cursor
	// Limit 是期望条数。调用方需要 has_more 时应多取一条（见 List 的契约）。
	Limit int
}

// Normalize 收敛非法分页参数，避免 limit=0 时查出全表。
//
// 口径与 domain.Page.Normalize 一致：缺省 10、上限 100。关键词去首尾空白
// 但不改写大小写（MySQL 的 utf8mb4_0900_ai_ci 排序规则本身不区分大小写）。
func (q ListQuery) Normalize() ListQuery {
	q.Keyword = strings.TrimSpace(q.Keyword)
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	return q
}

// Repository 是浏览历史的持久化契约。
type Repository interface {
	// Upsert 写入或刷新一条浏览记录。
	//
	// 唯一键是 (article_id, user_id)，与 deleted_at 无关：曾经被移出/清理
	// （软删）的同一篇再次浏览时必须**复活那一行**，否则唯一键冲突。
	Upsert(ctx context.Context, r *Record) error
	// List 列出一页浏览历史，按 (ViewedAt, ArticleID) 倒序。
	//
	// 返回条数最多 q.Limit（调用方想要 has_more 就自己多取一条）。
	// onlyAdminAuthors 与收藏内容一样放在查询层，保证过滤后再分页。
	List(ctx context.Context, q ListQuery) ([]*Record, error)
	// Delete 删除某用户指定的若干条记录。
	Delete(ctx context.Context, userID domain.UserID, articleIDs []domain.ArticleID) error
	// DeleteAll 删除某用户的全部浏览历史（「清空历史」）。
	DeleteAll(ctx context.Context, userID domain.UserID) error
	// DeleteByArticle 删除某文章的全部记录（删文章级联用）。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error
}
