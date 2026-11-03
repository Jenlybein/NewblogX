package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// CollectionRepo 是 collection.Repository 的内存实现。
//
// 卡片读模型的篇数与封面回退都从目录树派生（与生产同一口径）：它持有文章 /
// 目录树 / 用户 fake 的引用，所以"文章变私有后篇数跟着变"可以在用例层被
// 断言，不必等到 adapter/mysql 的集成测试。
type CollectionRepo struct {
	mus  sync.Mutex
	arts *ArticleRepo
	// outs / users 让卡片读模型与生产一样从目录树与作者角色派生：
	// 收录关系只有目录树这一份真相，没有第二份成员表可读。
	outs  *OutlineRepo
	users *Users

	collections map[collection.ID]*collection.Collection
	// lastOnlyAdminAuthors 记录最近一次读面收到的过滤开关。
	//
	// 内存里没有 user_models 可 join，"作者是否管理员"没法真的过滤
	// （那是 adapter/mysql 集成测试的事）；这里只用来证明 app 把开关传下去了。
	lastOnlyAdminAuthors bool

	fault fault
	count counter
}

// NewCollectionRepo 创建空仓储；arts / outs / users 用于读模型派生（可为 nil）。
func NewCollectionRepo(arts *ArticleRepo, outs *OutlineRepo, users *Users) *CollectionRepo {
	return &CollectionRepo{
		arts:        arts,
		outs:        outs,
		users:       users,
		collections: make(map[collection.ID]*collection.Collection),
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
//
// 不检查"已软删"：删合集本身就是 SoftDelete + Update 这一条写路径，
// 存储里放的是同一个聚合指针，此处无法区分"正在软删"与"改一个已删的合集"。
// 「删掉后读不到」由 Find / 列表的 Deleted 过滤保证。
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

// Find 按 ID 读取合集（含未展示的，不含已软删的）。
func (r *CollectionRepo) Find(_ context.Context, id collection.ID) (*collection.Collection, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	c, ok := r.collections[id]
	if !ok || c.Deleted() {
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
		if !c.Displayed() || c.Deleted() {
			continue
		}
		// 卡片读面只认目录树：收录关系没有第二份真相。
		count, fallback, err := outlineCardStats(context.Background(), r.outs, r.arts, r.users,
			outline.OwnerCollection, ctype.ID(c.ID()), onlyAdminAuthors)
		if err != nil {
			return nil, 0, err
		}
		// 开关关闭时，一篇公开文章都凑不出的合集不出现。
		if onlyAdminAuthors && count == 0 {
			continue
		}
		cover := c.Cover()
		if cover == "" {
			// 没上传封面时用 DFS 第一个公开文章节点的封面；它为空就是空。
			cover = fallback
		}
		out = append(out, collection.Card{
			ID:           c.ID(),
			Title:        c.Title(),
			Description:  c.Description(),
			Cover:        cover,
			ArticleCount: count,
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
		if c.Deleted() {
			continue
		}
		// 篇数与回退封面与读者卡片同一口径：只数树里仍公开的文章，
		// 封面取 DFS 第一个公开文章节点。
		count, fallback, err := outlineCardStats(context.Background(), r.outs, r.arts, r.users,
			outline.OwnerCollection, ctype.ID(c.ID()), false)
		if err != nil {
			return nil, err
		}
		out = append(out, collection.AdminCard{
			ID:            c.ID(),
			Title:         c.Title(),
			Description:   c.Description(),
			Cover:         c.Cover(),
			Displayed:     c.Displayed(),
			ArticleCount:  count,
			SortOrder:     c.SortOrder(),
			FallbackCover: fallback,
		})
	}
	return out, nil
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
	return collectionState{collections: cols, onlyAdmin: r.lastOnlyAdminAuthors}
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
	r.lastOnlyAdminAuthors = state.onlyAdmin
}

type collectionState struct {
	collections map[collection.ID]*collection.Collection
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
