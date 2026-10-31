package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/hottop"
)

// HotTopRepo 是 hottop.Repository 的内存实现。
//
// 只模拟"关系 + 序号"这套薄状态：卡片与后台行所需的文章字段来自真实 SQL join，
// 内存里没有文章表可 join，所以读模型靠 SeedCards 预置（见 ListCards 的注释）。
type HotTopRepo struct {
	mu sync.Mutex
	// orders 是 articleID → 序号（从 1 开始连续）。
	orders map[domain.ArticleID]int
	// states 是文章当前状态，用于后台「已失效」判定；缺省视为"文章已不存在"。
	states map[domain.ArticleID]*hottop.ArticleState
	// cards 预置的读模型卡片，同时充当后台行的标题来源。
	cards []hottop.Card
	// lastOnlyAdminAuthors 记录最近一次 ListCards 收到的过滤开关。
	//
	// 内存里没有 user_models 可 join，"作者是否管理员"没法真的过滤（那是
	// adapter/mysql 集成测试的事）；这里只用来证明 app 把开关传下去了。
	lastOnlyAdminAuthors bool
	fault                fault
	count                counter
}

// NewHotTopRepo 创建空仓储。
func NewHotTopRepo() *HotTopRepo {
	return &HotTopRepo{
		orders: make(map[domain.ArticleID]int),
		states: make(map[domain.ArticleID]*hottop.ArticleState),
	}
}

// SeedEntry 预置一条置顶（序号按调用顺序 1..n）。
//
// state 传 nil 表示"文章已经查不到了"（物理删除），后台应把它标成失效。
func (r *HotTopRepo) SeedEntry(articleID domain.ArticleID, state *hottop.ArticleState) *HotTopRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.orders[articleID]; !ok {
		r.orders[articleID] = len(r.orders) + 1
	}
	r.states[articleID] = state
	return r
}

// SeedCards 预置读模型卡片（公开列表与后台行的标题）。
func (r *HotTopRepo) SeedCards(cards ...hottop.Card) *HotTopRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cards = append(r.cards, cards...)
	return r
}

// FailNext 注入一次性故障。
func (r *HotTopRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *HotTopRepo) Calls(method string) int { return r.count.Calls(method) }

// Save 新增置顶；已存在时返回 false（幂等）。
func (r *HotTopRepo) Save(_ context.Context, articleID domain.ArticleID) (bool, error) {
	r.count.inc("Save")
	if err := r.fault.take("Save"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.orders[articleID]; ok {
		return false, nil
	}
	r.orders[articleID] = len(r.orders) + 1
	return true, nil
}

// Delete 取消置顶并压实序号；不存在时什么也不做（幂等）。
func (r *HotTopRepo) Delete(_ context.Context, articleID domain.ArticleID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.orders, articleID)
	r.compactLocked()
	return nil
}

// Exists 判断是否已置顶。
func (r *HotTopRepo) Exists(_ context.Context, articleID domain.ArticleID) (bool, error) {
	r.count.inc("Exists")
	if err := r.fault.take("Exists"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.orders[articleID]
	return ok, nil
}

// Count 统计置顶条数。
func (r *HotTopRepo) Count(_ context.Context) (int, error) {
	r.count.inc("Count")
	if err := r.fault.take("Count"); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.orders), nil
}

// ListIDs 按序号返回置顶文章 ID。
func (r *HotTopRepo) ListIDs(_ context.Context) ([]domain.ArticleID, error) {
	r.count.inc("ListIDs")
	if err := r.fault.take("ListIDs"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sortedIDsLocked(), nil
}

// ListCards 返回预置的读模型卡片。
//
// 真实实现的"只返回仍公开的文章"是 SQL join 的事，内存 fake 无法复现；
// 集成测试（adapter/mysql）覆盖那部分。这里只记录收到的过滤开关。
func (r *HotTopRepo) ListCards(_ context.Context, onlyAdminAuthors bool) ([]hottop.Card, error) {
	r.count.inc("ListCards")
	if err := r.fault.take("ListCards"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors
	return append([]hottop.Card(nil), r.cards...), nil
}

// ListCardsOnlyAdminAuthors 返回最近一次 ListCards 收到的过滤开关。
func (r *HotTopRepo) ListCardsOnlyAdminAuthors() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastOnlyAdminAuthors
}

// ListAdminItems 由 orders + states + cards 拼出后台行。
func (r *HotTopRepo) ListAdminItems(_ context.Context) ([]hottop.AdminItem, error) {
	r.count.inc("ListAdminItems")
	if err := r.fault.take("ListAdminItems"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]hottop.AdminItem, 0, len(r.orders))
	for _, id := range r.sortedIDsLocked() {
		item := hottop.AdminItem{
			ArticleID: id,
			SortOrder: r.orders[id],
			Article:   r.states[id],
		}
		for _, c := range r.cards {
			if c.ArticleID == id {
				item.Title = c.Title
				item.Cover = c.Cover
				break
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateSortOrder 按给定顺序重排。
func (r *HotTopRepo) UpdateSortOrder(_ context.Context, ids []domain.ArticleID) error {
	r.count.inc("UpdateSortOrder")
	if err := r.fault.take("UpdateSortOrder"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, id := range ids {
		r.orders[id] = i + 1
	}
	return nil
}

// compactLocked 把序号压实成 1..n（调用方必须已持锁）。
func (r *HotTopRepo) compactLocked() {
	for i, id := range r.sortedIDsLocked() {
		r.orders[id] = i + 1
	}
}

// sortedIDsLocked 按序号升序返回 ID（同序号时按 ID 数值兜底，保证稳定）。
func (r *HotTopRepo) sortedIDsLocked() []domain.ArticleID {
	ids := make([]domain.ArticleID, 0, len(r.orders))
	for id := range r.orders {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if r.orders[ids[i]] != r.orders[ids[j]] {
			return r.orders[ids[i]] < r.orders[ids[j]]
		}
		return ids[i].Uint64() < ids[j].Uint64()
	})
	return ids
}

// Snapshot 参与事务回滚。
func (r *HotTopRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	orders := make(map[domain.ArticleID]int, len(r.orders))
	for k, v := range r.orders {
		orders[k] = v
	}
	states := make(map[domain.ArticleID]*hottop.ArticleState, len(r.states))
	for k, v := range r.states {
		states[k] = v
	}
	return hotTopState{orders: orders, states: states, cards: append([]hottop.Card(nil), r.cards...)}
}

// RestoreState 覆盖回快照。
func (r *HotTopRepo) RestoreState(s any) {
	state, ok := s.(hotTopState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders = state.orders
	r.states = state.states
	r.cards = state.cards
}

type hotTopState struct {
	orders map[domain.ArticleID]int
	states map[domain.ArticleID]*hottop.ArticleState
	cards  []hottop.Card
}
