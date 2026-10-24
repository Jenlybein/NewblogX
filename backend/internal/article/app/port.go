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
)

// Repos 是"一次事务内可用的全部仓储"。
//
// 关键设计：app 拿到的是 Repos 而不是 *gorm.DB。
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
}

// TxManager 划定事务边界。
//
// 实现放在 adapter（用 platform/mysql 的 *gorm.DB 开事务，
// 再用同一个 tx 构造 Repos）；测试用 apptest.Tx 注入失败与回滚。
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r Repos) error) error
}

// AuthorStatsPort 是跨模块写端口。
//
// 同步实现：adapter 转发给 user_service.StatApplyArticleDelta，复用同一事务。
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
