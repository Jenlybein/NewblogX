package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	redis_service "myblogx/internal/platform/redis"
)

// errNilDB 表示装配期漏传了数据库连接。
//
// 不 panic：启动期错误应该尽早暴露，但不该在装配阶段直接杀死进程难以定位。
// 文案用英文：这是运维/开发者看的错误，不面向终端用户（R8 的目标是后者）。
var errNilDB = errors.New("article/mysql: database not initialized")

// TxManager 是 app.TxManager 的实现。
//
// 关键点：事务句柄只在本包内流转。app 拿到的是 repos（一组仓储接口），
// 因此 app 里不可能出现 SQL，也不可能绕过事务直接落库。
type TxManager struct {
	db             *gorm.DB
	counterOptions redis_service.Options
}

// NewTxManager 构造事务管理器。
//
// counterOptions 允许零值（无 Redis 时计数退化为 no-op）。
//
// 不再接收 outbox：事件出口现在是**事务内的 outbox 表**，
// 由 repos 用当前事务句柄现场构造（见 repos.Outbox），
// 因此事件落库必然与业务写入同事务，不可能绕过事务。
func NewTxManager(db *gorm.DB, counterOptions redis_service.Options) *TxManager {
	return &TxManager{db: db, counterOptions: counterOptions}
}

// WithinTx 在一个事务里执行 fn，并用同一个句柄构造全部仓储。
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context, r app.Repos) error) error {
	if m == nil || m.db == nil {
		return errNilDB
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, m.repos(tx))
	})
}

// repos 用指定事务句柄构造仓储集合。
func (m *TxManager) repos(tx *gorm.DB) *repos {
	return &repos{
		db:      tx,
		counter: NewCounterPort(m.counterOptions),
	}
}

// repos 实现 app.Repos。
//
// 每个 Getter 现场构造一个轻量仓储（只是把 db 包起来），不做缓存：
// 这样"同一个 repos 实例 == 同一个事务句柄"的语义不会被破坏。
type repos struct {
	db      *gorm.DB
	counter app.CounterPort
}

func (r *repos) Articles() domain.Repository             { return NewArticleRepo(r.db) }
func (r *repos) Categories() taxonomy.CategoryRepository { return NewCategoryRepo(r.db) }
func (r *repos) Tags() taxonomy.TagRepository            { return NewTagRepo(r.db) }
func (r *repos) Favorites() favorite.Repository          { return NewFavoriteRepo(r.db) }
func (r *repos) Tops() top.Repository                    { return NewTopRepo(r.db) }
func (r *repos) ViewHistory() viewhistory.Repository     { return NewViewHistoryRepo(r.db) }
func (r *repos) Reviews() review.Repository              { return NewReviewRepo(r.db) }
func (r *repos) AuthorStats() app.AuthorStatsPort        { return &authorStats{db: r.db} }

// Outbox 用当前事务句柄构造写入器：事件行写进本事务，随业务一起提交/回滚。
func (r *repos) Outbox() app.OutboxPort    { return NewOutboxWriter(r.db) }
func (r *repos) Counters() app.CounterPort { return r.counter }
func (r *repos) Comments() app.CommentPort { return &commentPort{db: r.db} }
func (r *repos) Users() app.UserPort       { return &userPort{db: r.db} }
func (r *repos) Diggs() app.DiggPort       { return &diggPort{db: r.db} }
