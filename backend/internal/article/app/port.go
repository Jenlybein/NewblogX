// Package app 是 article 模块的用例层。
//
// 职责边界：
//   - 事务边界在这里（通过 TxManager），一个用例 = 一次事务；
//   - 只依赖 domain 的端口与自己定义的 Repos 聚合，不认识 gorm、gin；
//   - 时间与 ID 都从外部注入，测试可用 FakeClock / SeqID 做到完全确定。
package app

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/hottop"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
)

// Repos 是"一次事务内可用的全部仓储"。
//
// 关键设计：app 拿到的是 Repos 而不是数据库句柄。
// 类型上就写不出 SQL，也就不会把 handler 里"随手查库"的老毛病带进来。
type Repos interface {
	Articles() domain.Repository
	Categories() taxonomy.CategoryRepository
	Tags() taxonomy.TagRepository
	Favorites() favorite.Repository
	Tops() top.Repository
	// HotTops 是站点运营位「热门置顶」（表 article_hot_top_models）。
	// 与 Tops（作者置顶）是两套数据，不要互相顶替。
	HotTops() hottop.Repository
	ViewHistory() viewhistory.Repository
	Reviews() review.Repository
	// AuthorStats 是唯一的跨模块写：删文章/浏览时更新作者统计。
	AuthorStats() AuthorStatsPort
	// Outbox 是领域事件的出口（当前仅用于记录与后续订阅）。
	Outbox() OutboxPort
	// Counters 是浏览/点赞/收藏/评论的增量计数（生产里落在 Redis）。
	Counters() CounterPort
	// Comments 是评论模块的跨模块能力（删文章级联、详情页评论数）。
	Comments() CommentPort
	// Users 是用户模块的只读能力（作者展示信息）。
	Users() UserPort
	// Diggs 是点赞关系（详情页"是否已赞"、级联删除）。
	Diggs() DiggPort
	// Collections 是合集（管理员编排的有序文章集合）。
	// 与 HotTops（热门置顶运营位）是两套数据：合集有标题/封面/展示开关，且无条数上限。
	Collections() collection.Repository
	// Projects 是项目（与合集并列的独立内容实体）。
	// 与 Collections 是两套数据、两套表：项目有自己的后台维护页，不共用合集的行。
	Projects() project.Repository
	// 目录树不在 Repos 上：两个实体各有一张具名树表，读树前必须先用
	// NamedOutlineRepository 把一次事务的仓储指向对应归属（见 named_outline.go）。
}

// DiggPort 是点赞关系端口。
//
// 计数增量走 CounterPort，关系本身（谁赞了哪篇）走这里——两者职责不同：
// 前者在 Redis 里是易失的，后者必须落库才能防重复。
type DiggPort interface {
	// Exists 判断某用户是否已点赞。
	Exists(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// Add 新增点赞关系；已存在返回 false 表示无变化。
	Add(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// Remove 取消点赞；本不存在返回 false。
	Remove(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// DeleteByArticle 删除某文章的全部点赞，返回条数。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
}

// CounterPort 是文章计数增量端口。
//
// 生产实现转发到现有 Redis 计数（redis_article）；测试用内存 fake。
// 设计成"只加增量、不做绝对值"：并发点赞不能靠读-改-写。
type CounterPort interface {
	// Add 累加计数增量。
	Add(ctx context.Context, id domain.ArticleID, delta domain.Counters) error
	// Load 批量读取增量；缺失的文章返回零值。
	Load(ctx context.Context, ids []domain.ArticleID) (map[domain.ArticleID]domain.Counters, error)
}

// CommentPort 是评论模块能力。
//
// 只暴露 article 需要的两个动作：级联删除与计数。
// 不把评论实体引进来，避免两个聚合互相依赖。
type CommentPort interface {
	// DeleteByArticle 删除某文章的全部评论，返回删除条数。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
	// CountByArticle 统计某文章的评论数。
	CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
}

// UserPort 是用户模块的只读能力。
type UserPort interface {
	// Profile 取单个用户的展示信息。
	Profile(ctx context.Context, id domain.UserID) (AuthorView, error)
	// Profiles 批量取；缺失的 ID 直接跳过。
	Profiles(ctx context.Context, ids []domain.UserID) (map[domain.UserID]AuthorView, error)
}

// TxManager 划定事务边界。
//
// 实现放在 adapter（用 platform/mysql 的句柄开事务，
// 再用同一个 tx 构造 Repos）；测试用 apptest.Tx 注入失败与回滚。
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r Repos) error) error
}

// AuthorStatsPort 是跨模块写端口。
//
// 同步实现：由 adapter 把**当前事务句柄**交给装配层注入的写入函数
// （internal/user/adapter/mysql.ApplyArticleDelta），因此仍然同事务，
// 但 article 不 import user 模块的任何实现包。
// 不做异步/outbox 的原因：会改变"删完文章作者文章数立刻减 1"的既有语义。
type AuthorStatsPort interface {
	ApplyArticleDelta(ctx context.Context, authorID domain.UserID, articleDelta, viewDelta int) error
}

// OutboxPort 是领域事件出口。
type OutboxPort interface {
	Emit(ctx context.Context, e Event) error
}

// Event 是一条领域事件。
//
// Payload 用 []byte 而不是 any：与日志治理的结论一致，
// 禁止把任意结构体塞进事件，避免出现无法审计的字段。
type Event struct {
	// Name 是事件名，如 "article.published"。
	Name string
	// Payload 是已序列化的 JSON。
	Payload []byte
	// AggregateID 是事件主体的 ID（文章事件就是文章 ID）。
	//
	// 单独带一份而不是让消费者去解 payload：outbox 表要按它建索引，
	// 运维排查"这篇文章的事件卡在哪一步"时可以直接查。
	AggregateID ctype.ID
	// AggregateType 是聚合类型，当前只有 "article"。
	AggregateType string
}

// SubmitQuotaPort 是「投稿配额」端口（票 304 / ADR-0030）。
//
// 由使用方（本包）定义、装配层接 Redis 适配器；形状对齐私信发送限流的预占 / 释放
// （domain.Limiter）：一个 Reserve 方法 + 一份可释放的预占，不引入"查询剩余额度"
// 这类读方法——额度只在提交那一刻被判定，其他地方要显示剩余量应当另开只读端口。
type SubmitQuotaPort interface {
	// Reserve 预占一次当日的投稿额度。
	//
	// 额度用完返回 domain.ErrSubmitQuotaExceeded；其余错误视为系统故障
	// （失败关闭：不放行提交）。成功时调用方在提交失败后必须 Release。
	Reserve(ctx context.Context, userID domain.UserID) (SubmitQuotaReservation, error)
}

// SubmitQuotaReservation 是一次已占用的投稿额度。
//
// Release 必须幂等：调用点常写成"显式归还 + defer 兜底"，重复归还会把
// 当日额度凭空放大。
type SubmitQuotaReservation interface {
	Release(ctx context.Context) error
}

// Clock 是可注入时钟。domain 不取时间，时间由用例层传入。
type Clock interface {
	Now() time.Time
}

// IDGen 是可注入 ID 生成器。
//
// 生产用雪花，测试用固定序列；这样 golden 才能逐字节稳定。
type IDGen interface {
	NextArticleID() domain.ArticleID
	NextReviewTaskID() uint64
	NextFolderID() favorite.ID
	// NextCollectionID 生成合集 ID。
	//
	// 不借用 NextArticleID：两次新建的 ID 序列互相穿插时，日志里无法分辨
	// "这个 ID 是文章还是合集"，而合集的 ID 会出现在读者站 URL 上。
	NextCollectionID() collection.ID
	// NextProjectID 生成项目 ID。
	//
	// 与 NextCollectionID 分开：项目与合集是两套内容实体，ID 会出现在
	// 读者站 URL 上，日志里要能分辨"这是项目还是合集"。
	NextProjectID() project.ID
	// NextOutlineNodeID 生成目录树节点 ID。
	//
	// 树节点 ID 会出现在后台接口里（整树提交带回 ID 才能幂等更新），
	// 与文章 / 合集 / 项目 ID 分开，日志里能分辨"这一行是树节点"。
	NextOutlineNodeID() outline.ID
}

// Service 是用例集合。
//
// 字段不导出，只能通过 New 构造；依赖全部是接口，便于替换 fake。
type Service struct {
	tx    TxManager
	clock Clock
	ids   IDGen
	// quota 是投稿配额端口（票 304）。为 nil 表示"不设上限"——只有测试装配会这样；
	// 生产装配在 backend/app/article_wiring.go 里必然注入 Redis 适配器，
	// 而适配器在 Redis 不可用时返回错误（失败关闭，不放行提交）。
	quota SubmitQuotaPort
}

// New 构造用例服务。
//
// quota 为 nil 时提交路径不设上限（测试用）；生产装配必须给出（见 article_wiring.go）。
func New(tx TxManager, clock Clock, ids IDGen, quota SubmitQuotaPort) *Service {
	return &Service{tx: tx, clock: clock, ids: ids, quota: quota}
}
