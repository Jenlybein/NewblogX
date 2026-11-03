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
// 与 CollectionRepo 同一做法：卡片的篇数与封面回退都从目录树派生，读模型与
// 生产同一口径，所以"文章不再公开后篇数跟着变"可以在用例层被断言，
// 不必等到 adapter/mysql 的集成测试。
type ProjectRepo struct {
	mus  sync.Mutex
	arts *ArticleRepo
	// outs / users 让卡片读模型与生产一样从目录树与作者角色派生。
	outs  *OutlineRepo
	users *Users

	projects map[project.ID]*project.Project

	// lastOnlyAdminAuthors 记录最近一次读面收到的过滤开关。
	// 内存里没有 user_models 可 join，这里只证明 app 把开关传下去了。
	lastOnlyAdminAuthors bool

	fault fault
	count counter
}

// NewProjectRepo 创建空仓储；arts / outs / users 用于读模型派生（可为 nil）。
func NewProjectRepo(arts *ArticleRepo, outs *OutlineRepo, users *Users) *ProjectRepo {
	return &ProjectRepo{
		arts:     arts,
		outs:     outs,
		users:    users,
		projects: make(map[project.ID]*project.Project),
	}
}

// SeedProject 直接预置一个项目（跳过用例层的权限与校验）。
func (r *ProjectRepo) SeedProject(p *project.Project) *ProjectRepo {
	r.mus.Lock()
	defer r.mus.Unlock()
	r.projects[p.ID()] = p
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
		// 卡片读面只认目录树：收录关系没有第二份真相。
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

// Snapshot 参与事务回滚。
func (r *ProjectRepo) Snapshot() any {
	r.mus.Lock()
	defer r.mus.Unlock()
	projects := make(map[project.ID]*project.Project, len(r.projects))
	for k, v := range r.projects {
		projects[k] = v
	}
	return projectState{projects: projects, onlyAdmin: r.lastOnlyAdminAuthors}
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
	r.lastOnlyAdminAuthors = state.onlyAdmin
}

type projectState struct {
	projects  map[project.ID]*project.Project
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
