package apptest

import (
	"context"
	"sync"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
)

// Repos 把各 fake 仓储聚成一个 app.Repos。
//
// 注意：本包 import app（为了满足接口），因此 **app 包的测试必须用
// `package app_test`**，否则会形成 app → apptest → app 的循环依赖。
//
// 命名说明：`Articles()` 等方法满足 app.Repos 接口，返回接口类型；
// 测试需要调 fake 特有方法（Calls / FailNext）时用 `FakeArticles()`。
type Repos struct {
	articleRepo  *ArticleRepo
	categoryRepo *CategoryRepo
	tagRepo      *TagRepo
	favoriteRepo *FavoriteRepo
	topRepo      *TopRepo
	historyRepo  *ViewHistoryRepo
	reviewRepo   *ReviewRepo
	authorStats  *AuthorStats
	outbox       *Outbox
	counters     *Counters
	comments     *Comments
	users        *Users
	diggs        *Diggs
}

// NewRepos 构造一套全部为空的 fake 仓储。
func NewRepos() *Repos {
	return &Repos{
		articleRepo:  NewArticleRepo(),
		categoryRepo: NewCategoryRepo(),
		tagRepo:      NewTagRepo(),
		favoriteRepo: NewFavoriteRepo(),
		topRepo:      NewTopRepo(),
		historyRepo:  NewViewHistoryRepo(),
		reviewRepo:   NewReviewRepo(),
		authorStats:  NewAuthorStats(),
		outbox:       NewOutbox(),
		counters:     NewCounters(),
		comments:     NewComments(),
		users:        NewUsers(),
		diggs:        NewDiggs(),
	}
}

// Counters 返回计数端口。
func (r *Repos) Counters() app.CounterPort { return r.counters }

// Comments 返回评论端口。
func (r *Repos) Comments() app.CommentPort { return r.comments }

// Users 返回用户端口。
func (r *Repos) Users() app.UserPort { return r.users }

// Diggs 返回点赞端口。
func (r *Repos) Diggs() app.DiggPort { return r.diggs }

// FakeCounters 返回计数 fake。
func (r *Repos) FakeCounters() *Counters { return r.counters }

// FakeComments 返回评论 fake。
func (r *Repos) FakeComments() *Comments { return r.comments }

// FakeUsers 返回用户 fake。
func (r *Repos) FakeUsers() *Users { return r.users }

// FakeDiggs 返回点赞 fake。
func (r *Repos) FakeDiggs() *Diggs { return r.diggs }

// ── app.Repos 接口实现 ──

// Articles 返回文章仓储。
func (r *Repos) Articles() domain.Repository { return r.articleRepo }

// Categories 返回分类仓储。
func (r *Repos) Categories() taxonomy.CategoryRepository { return r.categoryRepo }

// Tags 返回标签仓储。
func (r *Repos) Tags() taxonomy.TagRepository { return r.tagRepo }

// Favorites 返回收藏仓储。
func (r *Repos) Favorites() favorite.Repository { return r.favoriteRepo }

// Tops 返回置顶仓储。
func (r *Repos) Tops() top.Repository { return r.topRepo }

// ViewHistory 返回浏览历史仓储。
func (r *Repos) ViewHistory() viewhistory.Repository { return r.historyRepo }

// Reviews 返回审核任务仓储。
func (r *Repos) Reviews() review.Repository { return r.reviewRepo }

// AuthorStats 返回作者统计端口。
func (r *Repos) AuthorStats() app.AuthorStatsPort { return r.authorStats }

// Outbox 返回事件出口。
func (r *Repos) Outbox() app.OutboxPort { return r.outbox }

// ── 具体类型访问（测试断言用）──

// FakeArticles 返回文章仓储 fake。
func (r *Repos) FakeArticles() *ArticleRepo { return r.articleRepo }

// FakeCategories 返回分类仓储 fake。
func (r *Repos) FakeCategories() *CategoryRepo { return r.categoryRepo }

// FakeTags 返回标签仓储 fake。
func (r *Repos) FakeTags() *TagRepo { return r.tagRepo }

// FakeFavorites 返回收藏仓储 fake。
func (r *Repos) FakeFavorites() *FavoriteRepo { return r.favoriteRepo }

// FakeTops 返回置顶仓储 fake。
func (r *Repos) FakeTops() *TopRepo { return r.topRepo }

// FakeHistory 返回浏览历史仓储 fake。
func (r *Repos) FakeHistory() *ViewHistoryRepo { return r.historyRepo }

// FakeReviews 返回审核任务仓储 fake。
func (r *Repos) FakeReviews() *ReviewRepo { return r.reviewRepo }

// FakeAuthorStats 返回作者统计 fake。
func (r *Repos) FakeAuthorStats() *AuthorStats { return r.authorStats }

// FakeOutbox 返回 outbox fake。
func (r *Repos) FakeOutbox() *Outbox { return r.outbox }

// snapshotAll 为事务回滚保存全部**有状态**依赖。
//
// 用 map 而不是切片：以后加端口不必同步改下标，少一类"加了三处漏了一处"的错。
// 只读端口（Users）不参与：它没有状态可回滚。
func (r *Repos) snapshotAll() map[string]any {
	return map[string]any{
		"articles":   r.articleRepo.Snapshot(),
		"categories": r.categoryRepo.Snapshot(),
		"tags":       r.tagRepo.Snapshot(),
		"favorites":  r.favoriteRepo.Snapshot(),
		"tops":       r.topRepo.Snapshot(),
		"history":    r.historyRepo.Snapshot(),
		"reviews":    r.reviewRepo.Snapshot(),
		"stats":      r.authorStats.Snapshot(),
		"outbox":     r.outbox.Snapshot(),
		"counters":   r.counters.Snapshot(),
		"comments":   r.comments.Snapshot(),
		"diggs":      r.diggs.Snapshot(),
	}
}

// restoreAll 用快照覆盖全部有状态依赖。
func (r *Repos) restoreAll(snaps map[string]any) {
	r.articleRepo.RestoreState(snaps["articles"])
	r.categoryRepo.RestoreState(snaps["categories"])
	r.tagRepo.RestoreState(snaps["tags"])
	r.favoriteRepo.RestoreState(snaps["favorites"])
	r.topRepo.RestoreState(snaps["tops"])
	r.historyRepo.RestoreState(snaps["history"])
	r.reviewRepo.RestoreState(snaps["reviews"])
	r.authorStats.RestoreState(snaps["stats"])
	r.outbox.RestoreState(snaps["outbox"])
	r.counters.RestoreState(snaps["counters"])
	r.comments.RestoreState(snaps["comments"])
	r.diggs.RestoreState(snaps["diggs"])
}

// Tx 是 app.TxManager 的实现。
//
// 关键能力：fn 返回错误时，把全部仓储恢复到调用前的快照——
// 这样才能测"事务回滚后仓储无副作用"，而这正是 gomock 做不到的事。
type Tx struct {
	repos *Repos

	mu        sync.Mutex
	calls     int
	commits   int
	rollbacks int
	failNext  error
}

// NewTx 构造事务替身。
func NewTx(repos *Repos) *Tx { return &Tx{repos: repos} }

// WithinTx 执行 fn；出错则回滚全部 fake 仓储。
func (t *Tx) WithinTx(ctx context.Context, fn func(ctx context.Context, r app.Repos) error) error {
	t.mu.Lock()
	t.calls++
	injected := t.failNext
	t.failNext = nil
	t.mu.Unlock()

	if injected != nil {
		return injected
	}

	snaps := t.repos.snapshotAll()
	err := fn(ctx, t.repos)
	if err != nil {
		t.repos.restoreAll(snaps)
		t.mu.Lock()
		t.rollbacks++
		t.mu.Unlock()
		return err
	}

	t.mu.Lock()
	t.commits++
	t.mu.Unlock()
	return nil
}

// FailNext 让下一次 WithinTx 直接失败（模拟"事务都没开起来"）。
func (t *Tx) FailNext(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failNext = err
}

// Calls 返回事务开启次数。
func (t *Tx) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// Commits 返回成功提交次数。
func (t *Tx) Commits() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.commits
}

// Rollbacks 返回回滚次数。
func (t *Tx) Rollbacks() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rollbacks
}

// AuthorStats 是 app.AuthorStatsPort 的可观测 fake。
type AuthorStats struct {
	mu     sync.Mutex
	deltas map[domain.UserID][2]int
	fault  fault
	count  counter
}

// NewAuthorStats 构造空的作者统计 fake。
func NewAuthorStats() *AuthorStats {
	return &AuthorStats{deltas: make(map[domain.UserID][2]int)}
}

// FailNext 注入一次性故障。
func (s *AuthorStats) FailNext(method string, err error) { s.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (s *AuthorStats) Calls(method string) int { return s.count.Calls(method) }

// ApplyArticleDelta 记录增量。
func (s *AuthorStats) ApplyArticleDelta(_ context.Context, authorID domain.UserID, articleDelta, viewDelta int) error {
	s.count.inc("ApplyArticleDelta")
	if err := s.fault.take("ApplyArticleDelta"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.deltas[authorID]
	s.deltas[authorID] = [2]int{cur[0] + articleDelta, cur[1] + viewDelta}
	return nil
}

// Delta 返回某作者的累计增量（文章数, 浏览量）。
func (s *AuthorStats) Delta(authorID domain.UserID) (articleDelta, viewDelta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.deltas[authorID]
	return d[0], d[1]
}

// Outbox 是 app.OutboxPort 的内存实现。
type Outbox struct {
	mu     sync.Mutex
	events []app.Event
	fault  fault
	count  counter
}

// NewOutbox 构造空 outbox。
func NewOutbox() *Outbox { return &Outbox{} }

// FailNext 注入一次性故障。
func (o *Outbox) FailNext(method string, err error) { o.fault.FailNext(method, err) }

// Emit 记录一条事件。
func (o *Outbox) Emit(_ context.Context, e app.Event) error {
	o.count.inc("Emit")
	if err := o.fault.take("Emit"); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
	return nil
}

// Events 返回已记录事件。
func (o *Outbox) Events() []app.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]app.Event(nil), o.events...)
}

// Names 返回已记录事件名，便于断言"发布了哪几种事件"。
func (o *Outbox) Names() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, 0, len(o.events))
	for _, e := range o.events {
		out = append(out, e.Name)
	}
	return out
}

// Calls 返回 Emit 调用次数。
func (o *Outbox) Calls() int { return o.count.Calls("Emit") }

// Clock 是 app.Clock 的可控实现。
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock 从给定时刻开始。
func NewClock(at time.Time) *Clock { return &Clock{now: at} }

// Now 返回当前时间。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance 前进 d。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set 设定时间。
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// IDGen 是 app.IDGen 的固定序列实现，保证 golden 可复现。
type IDGen struct {
	mu      sync.Mutex
	article uint64
	review  uint64
	folder  uint64
}

// NewIDGen 从 start 开始递增。
func NewIDGen(start uint64) *IDGen {
	return &IDGen{article: start, review: start, folder: start}
}

// NextArticleID 返回下一个文章 ID。
func (g *IDGen) NextArticleID() domain.ArticleID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.article++
	return domain.ArticleID(g.article)
}

// NextReviewTaskID 返回下一个审核任务 ID。
func (g *IDGen) NextReviewTaskID() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.review++
	return g.review
}

// NextFolderID 返回下一个收藏夹 ID。
func (g *IDGen) NextFolderID() favorite.ID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.folder++
	return favorite.ID(g.folder)
}
