package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
)

// ProjectRepo 是 project.Repository 的内存实现（只含读者侧公开读）。
//
// 与 CollectionRepo 同一做法：它持有文章 fake 的引用，读模型都从**真实的**
// fake 文章状态派生，所以"文章不再公开后读者侧看不到、关联行仍在"可以在
// 用例层被断言，不必等到 adapter/mysql 的集成测试。
//
// 本票没有写路径（管理端属于 #197），因此这个 fake 不参与事务回滚快照。
type ProjectRepo struct {
	mus  sync.Mutex
	arts *ArticleRepo

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

// NewProjectRepo 创建空仓储；arts 用于读模型查文章状态（可为 nil）。
func NewProjectRepo(arts *ArticleRepo) *ProjectRepo {
	return &ProjectRepo{
		arts:     arts,
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
		articles := r.publicArticlesLocked(pj.ID())
		// 开关关闭时，一篇公开文章都凑不出的项目不出现。
		if onlyAdminAuthors && len(articles) == 0 {
			continue
		}
		cover := pj.Cover()
		if cover == "" && len(articles) > 0 {
			// 没有封面时回退首篇公开文章的封面：卡片不能是破的。
			cover = articles[0].Cover
		}
		out = append(out, project.Card{
			ID:           pj.ID(),
			Title:        pj.Title(),
			Description:  pj.Description(),
			Cover:        cover,
			ArticleCount: len(articles),
			CreatedAt:    pj.CreatedAt(),
		})
	}

	total := int64(len(out))
	return pageSlice(out, p), total, nil
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
