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
	"myblogx/internal/article/domain/favorite"
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
}

// Service 是用例集合。
//
// 字段不导出，只能通过 New 构造；依赖全部是接口，便于替换 fake。
type Service struct {
	tx    TxManager
	clock Clock
	ids   IDGen
}

// New 构造用例服务。
func New(tx TxManager, clock Clock, ids IDGen) *Service {
	return &Service{tx: tx, clock: clock, ids: ids}
}
