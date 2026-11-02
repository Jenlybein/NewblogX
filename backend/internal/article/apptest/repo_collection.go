package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
)

// CollectionRepo 是 collection.Repository 的内存实现。
//
// 与 HotTopRepo 的读模型 fake 不同，这里**能真的按公开可见性过滤**：它持有
// 文章 fake 的引用，详情与列表的读模型都从真实（fake）文章状态派生，
// 所以"文章变私有后读者侧看不到、后台仍占位"可以在用例层被断言，
// 不必等到 adapter/mysql 的集成测试。
type CollectionRepo struct {
	mus  sync.Mutex
	arts *ArticleRepo

	collections map[collection.ID]*collection.Collection
	// items 是每个合集的收录行，顺序即插入顺序，序号单独记账。
	items map[collection.ID][]*item
	// lastOnlyAdminAuthors 记录最近一次读面收到的过滤开关。
	//
	// 内存里没有 user_models 可 join，"作者是否管理员"没法真的过滤
	// （那是 adapter/mysql 集成测试的事）；这里只用来证明 app 把开关传下去了。
	lastOnlyAdminAuthors bool

	fault fault
	count counter
}

// item 是一条收录行。
type item struct {
	articleID collection.ArticleID
	sortOrder int
	// deleted 为真表示这行被软删（移除收录），而行仍占着唯一键。
	deleted bool
}

// NewCollectionRepo 创建空仓储；arts 用于读模型查文章状态（可为 nil）。
func NewCollectionRepo(arts *ArticleRepo) *CollectionRepo {
	return &CollectionRepo{
		arts:        arts,
		collections: make(map[collection.ID]*collection.Collection),
		items:       make(map[collection.ID][]*item),
	}
}

// SeedCollection 直接预置一个合集（跳过用例层的权限与校验）。
func (r *CollectionRepo) SeedCollection(c *collection.Collection) *CollectionRepo {
	r.mus.Lock()
	defer r.mus.Unlock()
	r.collections[c.ID()] = c
	return r
}

// FailNext 注入一次性故障。
func (r *CollectionRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *CollectionRepo) Calls(method string) int { return r.count.Calls(method) }

// Create 插入合集。
func (r *CollectionRepo) Create(_ context.Context, c *collection.Collection) error {
	r.count.inc("Create")
	if err := r.fault.take("Create"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.collections[c.ID()]; ok {
		return ErrConflict
	}
	r.collections[c.ID()] = c
	return nil
}

// Update 保存已存在的合集；不存在返回错误。
func (r *CollectionRepo) Update(_ context.Context, c *collection.Collection) error {
	r.count.inc("Update")
	if err := r.fault.take("Update"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.collections[c.ID()]; !ok {
		return collection.ErrNotFound
	}
	r.collections[c.ID()] = c
	return nil
}

// Find 按 ID 读取合集（含未展示的）。
func (r *CollectionRepo) Find(_ context.Context, id collection.ID) (*collection.Collection, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	c, ok := r.collections[id]
	if !ok {
		return nil, collection.ErrNotFound
	}
	return c, nil
}

// NextSortOrder 返回当前最大序号 + 1。
func (r *CollectionRepo) NextSortOrder(_ context.Context) (int, error) {
	r.count.inc("NextSortOrder")
	if err := r.fault.take("NextSortOrder"); err != nil {
		return 0, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	maxOrder := 0
	for _, c := range r.collections {
		if c.SortOrder() > maxOrder {
			maxOrder = c.SortOrder()
		}
	}
	return maxOrder + 1, nil
}

// ListCards 列出展示中的合集卡片（按序号升序）。
func (r *CollectionRepo) ListCards(
	_ context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]collection.Card, int64, error) {
	r.count.inc("ListCards")
	if err := r.fault.take("ListCards"); err != nil {
		return nil, 0, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors

	displayed := r.sortedCollectionsLocked()
	out := make([]collection.Card, 0, len(displayed))
	for _, c := range displayed {
		if !c.Displayed() {
			continue
		}
		articles := r.publicArticlesLocked(c.ID())
		// 开关关闭时，一篇公开文章都凑不出的合集不出现。
		if onlyAdminAuthors && len(articles) == 0 {
			continue
		}
		cover := c.Cover()
		if cover == "" && len(articles) > 0 {
			// 没有封面时退回首篇文章的封面：卡片不能是破的。
			cover = articles[0].Cover
		}
		out = append(out, collection.Card{
			ID:           c.ID(),
			Title:        c.Title(),
			Description:  c.Description(),
			Cover:        cover,
			ArticleCount: len(articles),
			CreatedAt:    c.CreatedAt(),
		})
	}

	total := int64(len(out))
	return pageSlice(out, p), total, nil
}

// ListAdminCards 列出全部合集（含未展示）。
func (r *CollectionRepo) ListAdminCards(_ context.Context) ([]collection.AdminCard, error) {
	r.count.inc("ListAdminCards")
	if err := r.fault.take("ListAdminCards"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	all := r.sortedCollectionsLocked()
	out := make([]collection.AdminCard, 0, len(all))
	for _, c := range all {
		out = append(out, collection.AdminCard{
			ID:           c.ID(),
			Title:        c.Title(),
			Description:  c.Description(),
			Cover:        c.Cover(),
			Displayed:    c.Displayed(),
			ArticleCount: len(r.liveItemsLocked(c.ID())),
			SortOrder:    c.SortOrder(),
		})
	}
	return out, nil
}

// ListItemIDs 按序号返回当前收录的文章 ID。
func (r *CollectionRepo) ListItemIDs(_ context.Context, id collection.ID) ([]collection.ArticleID, error) {
	r.count.inc("ListItemIDs")
	if err := r.fault.take("ListItemIDs"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	rows := r.liveItemsLocked(id)
	out := make([]collection.ArticleID, 0, len(rows))
	for _, it := range rows {
		out = append(out, it.articleID)
	}
	return out, nil
}

// AppendItem 收录一篇文章，返回是否真的新增；移除过的恢复原行。
func (r *CollectionRepo) AppendItem(
	_ context.Context, id collection.ID, articleID collection.ArticleID,
) (bool, error) {
	r.count.inc("AppendItem")
	if err := r.fault.take("AppendItem"); err != nil {
		return false, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.collections[id]; !ok {
		return false, collection.ErrNotFound
	}

	for _, it := range r.items[id] {
		if it.articleID != articleID {
			continue
		}
		if !it.deleted {
			// 活行已存在：幂等返回 false（不挪到末尾）。
			return false, nil
		}
		// 软删行恢复：按新序号排到末尾。
		it.deleted = false
		it.sortOrder = len(r.liveItemsLocked(id)) + 1
		return true, nil
	}

	r.items[id] = append(r.items[id], &item{
		articleID: articleID,
		sortOrder: len(r.liveItemsLocked(id)) + 1,
	})
	return true, nil
}

// RemoveItem 移除收录（软删）并压实序号；不存在时不做任何事。
func (r *CollectionRepo) RemoveItem(
	_ context.Context, id collection.ID, articleID collection.ArticleID,
) error {
	r.count.inc("RemoveItem")
	if err := r.fault.take("RemoveItem"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	for _, it := range r.items[id] {
		if it.articleID == articleID && !it.deleted {
			it.deleted = true
			break
		}
	}
	r.compactLocked(id)
	return nil
}

// ListIncludedArticles 列出仍公开的收录文章（按序号升序）。
func (r *CollectionRepo) ListIncludedArticles(
	_ context.Context, id collection.ID, onlyAdminAuthors bool,
) ([]collection.IncludedArticle, error) {
	r.count.inc("ListIncludedArticles")
	if err := r.fault.take("ListIncludedArticles"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors

	rows := r.publicArticlesLocked(id)
	out := make([]collection.IncludedArticle, 0, len(rows))
	for _, a := range rows {
		out = append(out, collection.IncludedArticle{
			ArticleID: a.ArticleID,
			SortOrder: a.SortOrder,
			Title:     a.Title,
			Abstract:  a.Abstract,
			Cover:     a.Cover,
			ViewCount: a.ViewCount,
		})
	}
	return out, nil
}

// ListAdminItems 列出全部收录行（含已失效）。
func (r *CollectionRepo) ListAdminItems(_ context.Context, id collection.ID) ([]collection.AdminItem, error) {
	r.count.inc("ListAdminItems")
	if err := r.fault.take("ListAdminItems"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	out := make([]collection.AdminItem, 0, len(r.items[id]))
	for _, it := range r.items[id] {
		if it.deleted {
			continue
		}
		row := collection.AdminItem{ArticleID: it.articleID, SortOrder: it.sortOrder}
		if a, err := r.arts.Find(context.Background(), it.articleID); err == nil {
			row.Title = a.Title()
			row.Cover = a.Cover()
			row.Article = &collection.ArticleState{
				Deleted:    isDeleted(a),
				Status:     a.Status(),
				Visibility: a.Visibility(),
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// UpdateItemOrder 按给定顺序把序号改成 1..n。
func (r *CollectionRepo) UpdateItemOrder(
	_ context.Context, id collection.ID, articleIDs []collection.ArticleID,
) error {
	r.count.inc("UpdateItemOrder")
	if err := r.fault.take("UpdateItemOrder"); err != nil {
	return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	for i, articleID := range articleIDs {
		for _, it := range r.items[id] {
			if it.articleID == articleID {
				it.sortOrder = i + 1
			}
		}
	}
	return nil
}

// ListCardsOnlyAdminAuthors 返回最近一次读面收到的过滤开关。
func (r *CollectionRepo) ListCardsOnlyAdminAuthors() bool {
	r.mus.Lock()
	defer r.mus.Unlock()
	return r.lastOnlyAdminAuthors
}

// Snapshot 参与事务回滚。
func (r *CollectionRepo) Snapshot() any {
	r.mus.Lock()
	defer r.mus.Unlock()
	cols := make(map[collection.ID]*collection.Collection, len(r.collections))
	for k, v := range r.collections {
		cols[k] = v
	}
	items := make(map[collection.ID][]*item, len(r.items))
	for k, rows := range r.items {
		cp := make([]*item, 0, len(rows))
		for _, it := range rows {
			c := *it
			cp = append(cp, &c)
		}
		items[k] = cp
	}
	return collectionState{collections: cols, items: items, onlyAdmin: r.lastOnlyAdminAuthors}
}

// RestoreState 覆盖回快照。
func (r *CollectionRepo) RestoreState(s any) {
	state, ok := s.(collectionState)
	if !ok {
		return
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.collections = state.collections
	r.items = state.items
	r.lastOnlyAdminAuthors = state.onlyAdmin
}

type collectionState struct {
	collections map[collection.ID]*collection.Collection
	items       map[collection.ID][]*item
	onlyAdmin   bool
}

// sortedCollectionsLocked 按序号升序返回合集（同序号按 ID 兜底，保证稳定）。
func (r *CollectionRepo) sortedCollectionsLocked() []*collection.Collection {
	out := make([]*collection.Collection, 0, len(r.collections))
	for _, c := range r.collections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder() != out[j].SortOrder() {
			return out[i].SortOrder() < out[j].SortOrder()
		}
		return out[i].ID().Uint64() < out[j].ID().Uint64()
	})
	return out
}

// liveItemsLocked 按序号返回未软删的收录行（调用方必须已持锁）。
func (r *CollectionRepo) liveItemsLocked(id collection.ID) []*item {
	out := make([]*item, 0, len(r.items[id]))
	for _, it := range r.items[id] {
		if it.deleted {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].sortOrder != out[j].sortOrder {
			return out[i].sortOrder < out[j].sortOrder
		}
		return out[i].articleID.Uint64() < out[j].articleID.Uint64()
	})
	return out
}

// publicArticlesLocked 返回仍公开的收录文章（调用方必须已持锁）。
//
// 这就是真实 SQL join 的等价物：文章被删/变私有 → 条目消失，但行还在。
func (r *CollectionRepo) publicArticlesLocked(id collection.ID) []collection.IncludedArticle {
	rows := r.liveItemsLocked(id)
	out := make([]collection.IncludedArticle, 0, len(rows))
	for _, it := range rows {
		if r.arts == nil {
			out = append(out, collection.IncludedArticle{ArticleID: it.articleID, SortOrder: it.sortOrder})
			continue
		}
		a, err := r.arts.Find(context.Background(), it.articleID)
		if err != nil || !a.IsPubliclyVisible() {
			continue
		}
		out = append(out, collection.IncludedArticle{
			ArticleID: it.articleID,
			SortOrder: it.sortOrder,
			Title:     a.Title(),
			Abstract:  a.Abstract(),
			Cover:     a.Cover(),
			ViewCount: a.Counters().View,
		})
	}
	return out
}

// compactLocked 把序号压实成 1..n（调用方必须已持锁）。
func (r *CollectionRepo) compactLocked(id collection.ID) {
	for i, it := range r.liveItemsLocked(id) {
		it.sortOrder = i + 1
	}
}

// pageSlice 按 Offset/Limit 截取。
func pageSlice[T any](rows []T, p domain.Page) []T {
	start := p.Offset
	if start > len(rows) {
		start = len(rows)
	}
	end := start + p.Limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

// isDeleted 读文章 fake 的删除标记。
func isDeleted(a *domain.Article) bool {
	return !a.IsPubliclyVisible() && a.Status() == domain.StatusPublished && a.Visibility() == domain.VisibilityVisible
}
