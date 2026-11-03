package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// ProjectRepo 是 project.Repository 的内存实现（读者侧公开读 + 管理端读写）。
//
// 与 CollectionRepo 同一做法：它持有文章 fake 的引用，读模型都从**真实的**
// fake 文章状态派生，所以"文章不再公开后读者侧看不到、关联行仍在"可以在
// 用例层被断言，不必等到 adapter/mysql 的集成测试。
type ProjectRepo struct {
	mus  sync.Mutex
	arts *ArticleRepo
	// outs / users 让卡片读模型与生产一样从目录树与作者角色派生。
	outs  *OutlineRepo
	users *Users

	projects map[project.ID]*project.Project
	// items 是每个项目的收录行，顺序即写入顺序，序号单独记账。
	items map[project.ID][]projectItem

	// lastOnlyAdminAuthors 记录最近一次读面收到的过滤开关。
	// 内存里没有 user_models 可 join，这里只证明 app 把开关传下去了。
	lastOnlyAdminAuthors bool

	fault fault
	count counter
}

// projectItem 是一条收录行。
type projectItem struct {
	articleID project.ArticleID
	sortOrder int
	deleted   bool
}

// NewProjectRepo 创建空仓储；arts / outs / users 用于读模型派生（可为 nil）。
func NewProjectRepo(arts *ArticleRepo, outs *OutlineRepo, users *Users) *ProjectRepo {
	return &ProjectRepo{
		arts:     arts,
		outs:     outs,
		users:    users,
		projects: make(map[project.ID]*project.Project),
		items:    make(map[project.ID][]projectItem),
	}
}

// SeedProject 直接预置一个项目（跳过用例层的权限与校验）。
func (r *ProjectRepo) SeedProject(p *project.Project) *ProjectRepo {
	r.mus.Lock()
	defer r.mus.Unlock()
	r.projects[p.ID()] = p
	return r
}

// SeedItem 直接预置一条收录行。
func (r *ProjectRepo) SeedItem(id project.ID, articleID project.ArticleID, sortOrder int, deleted bool) *ProjectRepo {
	r.mus.Lock()
	defer r.mus.Unlock()
	r.items[id] = append(r.items[id], projectItem{
		articleID: articleID,
		sortOrder: sortOrder,
		deleted:   deleted,
	})
	return r
}

// FailNext 注入一次性故障。
func (r *ProjectRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *ProjectRepo) Calls(method string) int { return r.count.Calls(method) }

// Create 插入项目。
func (r *ProjectRepo) Create(_ context.Context, p *project.Project) error {
	r.count.inc("Create")
	if err := r.fault.take("Create"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.projects[p.ID()]; ok {
		return ErrConflict
	}
	r.projects[p.ID()] = p
	return nil
}

// Update 保存已存在的项目；不存在返回错误。
//
// 不检查"已软删"：删项目本身就是 SoftDelete + Update 这一条写路径。
// 「删掉后读不到」由 Find / 列表的 Deleted 过滤保证。
func (r *ProjectRepo) Update(_ context.Context, p *project.Project) error {
	r.count.inc("Update")
	if err := r.fault.take("Update"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.projects[p.ID()]; !ok {
		return project.ErrNotFound
	}
	r.projects[p.ID()] = p
	return nil
}

// NextSortOrder 返回当前最大序号 + 1。
func (r *ProjectRepo) NextSortOrder(_ context.Context) (int, error) {
	r.count.inc("NextSortOrder")
	if err := r.fault.take("NextSortOrder"); err != nil {
		return 0, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	maxOrder := 0
	for _, p := range r.projects {
		if p.SortOrder() > maxOrder {
			maxOrder = p.SortOrder()
		}
	}
	return maxOrder + 1, nil
}

// Find 按 ID 读取项目（含未展示的，不含已软删的）。
func (r *ProjectRepo) Find(_ context.Context, id project.ID) (*project.Project, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	p, ok := r.projects[id]
	if !ok || p.Deleted() {
		return nil, project.ErrNotFound
	}
	return p, nil
}

// ListCards 列出展示中的项目卡片（按序号升序）。
func (r *ProjectRepo) ListCards(
	_ context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]project.Card, int64, error) {
	r.count.inc("ListCards")
	if err := r.fault.take("ListCards"); err != nil {
		return nil, 0, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors

	out := make([]project.Card, 0, len(r.projects))
	for _, pj := range r.sortedProjectsLocked() {
		if !pj.Displayed() || pj.Deleted() {
			continue
		}
		// 卡片读面只认目录树：旧扁平收录表已冻结，不再参与。
		count, fallback, err := outlineCardStats(context.Background(), r.outs, r.arts, r.users,
			outline.OwnerProject, ctype.ID(pj.ID()), onlyAdminAuthors)
		if err != nil {
			return nil, 0, err
		}
		// 开关关闭时，一篇公开文章都凑不出的项目不出现。
		if onlyAdminAuthors && count == 0 {
			continue
		}
		cover := pj.Cover()
		if cover == "" {
			// 没上传封面时用 DFS 第一个公开文章节点的封面；它为空就是空。
			cover = fallback
		}
		out = append(out, project.Card{
			ID:           pj.ID(),
			Title:        pj.Title(),
			Description:  pj.Description(),
			Cover:        cover,
			ArticleCount: count,
			CreatedAt:    pj.CreatedAt(),
		})
	}

	total := int64(len(out))
	return pageSlice(out, p), total, nil
}

// ListAdminCards 列出全部项目（含未展示）。
func (r *ProjectRepo) ListAdminCards(_ context.Context) ([]project.AdminCard, error) {
	r.count.inc("ListAdminCards")
	if err := r.fault.take("ListAdminCards"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	all := r.sortedProjectsLocked()
	out := make([]project.AdminCard, 0, len(all))
	for _, pj := range all {
		if pj.Deleted() {
			continue
		}
		// 篇数与回退封面与读者卡片同一口径：只数树里仍公开的文章，
		// 封面取 DFS 第一个公开文章节点。
		count, fallback, err := outlineCardStats(context.Background(), r.outs, r.arts, r.users,
			outline.OwnerProject, ctype.ID(pj.ID()), false)
		if err != nil {
			return nil, err
		}
		out = append(out, project.AdminCard{
			ID:            pj.ID(),
			Title:         pj.Title(),
			Description:   pj.Description(),
			Cover:         pj.Cover(),
			Displayed:     pj.Displayed(),
			ArticleCount:  count,
			SortOrder:     pj.SortOrder(),
			FallbackCover: fallback,
		})
	}
	return out, nil
}

// ListIDs 按序号返回全部未软删项目的 ID。
func (r *ProjectRepo) ListIDs(_ context.Context) ([]project.ID, error) {
	r.count.inc("ListIDs")
	if err := r.fault.take("ListIDs"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	all := r.sortedProjectsLocked()
	out := make([]project.ID, 0, len(all))
	for _, pj := range all {
		if pj.Deleted() {
			continue
		}
		out = append(out, pj.ID())
	}
	return out, nil
}

// UpdateSortOrders 按给定顺序把项目自身的序号改成 1..n。
func (r *ProjectRepo) UpdateSortOrders(_ context.Context, ids []project.ID) error {
	r.count.inc("UpdateSortOrders")
	if err := r.fault.take("UpdateSortOrders"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	for i, id := range ids {
		pj, ok := r.projects[id]
		if !ok {
			return project.ErrNotFound
		}
		pj.SetSortOrder(i+1, pj.UpdatedAt())
	}
	return nil
}

// ListItemIDs 按序号返回当前收录的文章 ID。
func (r *ProjectRepo) ListItemIDs(_ context.Context, id project.ID) ([]project.ArticleID, error) {
	r.count.inc("ListItemIDs")
	if err := r.fault.take("ListItemIDs"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	rows := r.liveItemsLocked(id)
	out := make([]project.ArticleID, 0, len(rows))
	for _, it := range rows {
		out = append(out, it.articleID)
	}
	return out, nil
}

// AppendItem 收录一篇文章，返回是否真的新增；移除过的恢复原行。
func (r *ProjectRepo) AppendItem(
	_ context.Context, id project.ID, articleID project.ArticleID,
) (bool, error) {
	r.count.inc("AppendItem")
	if err := r.fault.take("AppendItem"); err != nil {
		return false, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	if _, ok := r.projects[id]; !ok {
		return false, project.ErrNotFound
	}

	for i := range r.items[id] {
		it := &r.items[id][i]
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

	r.items[id] = append(r.items[id], projectItem{
		articleID: articleID,
		sortOrder: len(r.liveItemsLocked(id)) + 1,
	})
	return true, nil
}

// RemoveItem 移除收录（软删）并压实序号；不存在时不做任何事。
func (r *ProjectRepo) RemoveItem(
	_ context.Context, id project.ID, articleID project.ArticleID,
) error {
	r.count.inc("RemoveItem")
	if err := r.fault.take("RemoveItem"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	for i := range r.items[id] {
		it := &r.items[id][i]
		if it.articleID == articleID && !it.deleted {
			it.deleted = true
			break
		}
	}
	r.compactLocked(id)
	return nil
}

// ListIncludedArticles 列出仍公开的收录文章（按序号升序）。
func (r *ProjectRepo) ListIncludedArticles(
	_ context.Context, id project.ID, onlyAdminAuthors bool,
) ([]project.IncludedArticle, error) {
	r.count.inc("ListIncludedArticles")
	if err := r.fault.take("ListIncludedArticles"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors

	rows := r.publicArticlesLocked(id)
	out := make([]project.IncludedArticle, 0, len(rows))
	for _, a := range rows {
		out = append(out, project.IncludedArticle{
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
func (r *ProjectRepo) ListAdminItems(_ context.Context, id project.ID) ([]project.AdminItem, error) {
	r.count.inc("ListAdminItems")
	if err := r.fault.take("ListAdminItems"); err != nil {
		return nil, err
	}
	r.mus.Lock()
	defer r.mus.Unlock()

	out := make([]project.AdminItem, 0, len(r.items[id]))
	for _, it := range r.liveItemsLocked(id) {
		row := project.AdminItem{ArticleID: it.articleID, SortOrder: it.sortOrder}
		// 与 MySQL ListAdminItems 的 LEFT JOIN 对齐：软删的文章仍能读到标题与
		// "已删除"状态；只有文章行真的不存在时才留空并由 domain 判为 missing。
		if a, err := r.arts.FindIncludingDeleted(context.Background(), it.articleID); err == nil {
			row.Title = a.Title()
			row.Cover = a.Cover()
			row.Article = &project.ArticleState{
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
func (r *ProjectRepo) UpdateItemOrder(
	_ context.Context, id project.ID, articleIDs []project.ArticleID,
) error {
	r.count.inc("UpdateItemOrder")
	if err := r.fault.take("UpdateItemOrder"); err != nil {
		return err
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	for i, articleID := range articleIDs {
		for j := range r.items[id] {
			if r.items[id][j].articleID == articleID {
				r.items[id][j].sortOrder = i + 1
			}
		}
	}
	return nil
}

// Snapshot 参与事务回滚。
func (r *ProjectRepo) Snapshot() any {
	r.mus.Lock()
	defer r.mus.Unlock()
	projects := make(map[project.ID]*project.Project, len(r.projects))
	for k, v := range r.projects {
		projects[k] = v
	}
	items := make(map[project.ID][]projectItem, len(r.items))
	for k, rows := range r.items {
		items[k] = append([]projectItem(nil), rows...)
	}
	return projectState{projects: projects, items: items, onlyAdmin: r.lastOnlyAdminAuthors}
}

// RestoreState 覆盖回快照。
func (r *ProjectRepo) RestoreState(s any) {
	state, ok := s.(projectState)
	if !ok {
		return
	}
	r.mus.Lock()
	defer r.mus.Unlock()
	r.projects = state.projects
	r.items = state.items
	r.lastOnlyAdminAuthors = state.onlyAdmin
}

type projectState struct {
	projects  map[project.ID]*project.Project
	items     map[project.ID][]projectItem
	onlyAdmin bool
}

// ListCardsOnlyAdminAuthors 返回最近一次读面收到的过滤开关。
func (r *ProjectRepo) ListCardsOnlyAdminAuthors() bool {
	r.mus.Lock()
	defer r.mus.Unlock()
	return r.lastOnlyAdminAuthors
}

// sortedProjectsLocked 按序号升序返回项目（同序号按 ID 兜底，保证稳定）。
func (r *ProjectRepo) sortedProjectsLocked() []*project.Project {
	out := make([]*project.Project, 0, len(r.projects))
	for _, pj := range r.projects {
		out = append(out, pj)
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
func (r *ProjectRepo) liveItemsLocked(id project.ID) []projectItem {
	out := make([]projectItem, 0, len(r.items[id]))
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
func (r *ProjectRepo) publicArticlesLocked(id project.ID) []project.IncludedArticle {
	rows := r.liveItemsLocked(id)
	out := make([]project.IncludedArticle, 0, len(rows))
	for _, it := range rows {
		if r.arts == nil {
			out = append(out, project.IncludedArticle{ArticleID: it.articleID, SortOrder: it.sortOrder})
			continue
		}
		a, err := r.arts.Find(context.Background(), it.articleID)
		if err != nil || !a.IsPubliclyVisible() {
			continue
		}
		out = append(out, project.IncludedArticle{
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
func (r *ProjectRepo) compactLocked(id project.ID) {
	for i, it := range r.liveItemsLocked(id) {
		for j := range r.items[id] {
			if r.items[id][j].articleID == it.articleID {
				r.items[id][j].sortOrder = i + 1
				break
			}
		}
	}
}
