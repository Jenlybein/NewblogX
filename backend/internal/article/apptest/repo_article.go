package apptest

import (
	"context"
	"sort"
	"strings"
	"sync"

	"myblogx/internal/article/domain"
)

// ErrConflict 模拟唯一键冲突（生产里对应 MySQL 1062）。
var ErrConflict = errConflict{}

type errConflict struct{}

func (errConflict) Error() string { return "apptest: 唯一键冲突" }

// ArticleRepo 是 domain.Repository 的内存实现。
//
// 特点：
//   - 存的是聚合快照（深拷贝进出），测试改了返回值也不会污染仓储内部状态；
//   - Create 撞 ID 返回 ErrConflict，可用来测"重复创建"；
//   - 支持故障注入与事务回滚。
type ArticleRepo struct {
	mu    sync.Mutex
	byID  map[domain.ArticleID]*domain.Article
	seq   []domain.ArticleID // 保持插入顺序，List 结果才稳定
	fault fault
	count counter
}

// NewArticleRepo 创建空仓储。
func NewArticleRepo() *ArticleRepo {
	return &ArticleRepo{byID: make(map[domain.ArticleID]*domain.Article)}
}

// Seed 预置数据（测试准备阶段用，不计入调用统计）。
func (r *ArticleRepo) Seed(articles ...*domain.Article) *ArticleRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range articles {
		cp := cloneArticle(a)
		if _, ok := r.byID[cp.ID()]; !ok {
			r.seq = append(r.seq, cp.ID())
		}
		r.byID[cp.ID()] = cp
	}
	return r
}

// FailNext 让下一次 method 返回 err。
func (r *ArticleRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回方法调用次数。
func (r *ArticleRepo) Calls(method string) int { return r.count.Calls(method) }

// Find 按 ID 读取。
func (r *ArticleRepo) Find(_ context.Context, id domain.ArticleID) (*domain.Article, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok || a.Deleted() {
		return nil, domain.ErrArticleNotFound
	}
	return cloneArticle(a), nil
}

// FindIncludingDeleted 按 ID 读取，**包含软删行**。
//
// 后台运营位列表走 LEFT JOIN 读关系行所指文章的状态与标题，软删的文章仍要
// 能读到（才能标出"被删"而不是"不存在"）。这里模拟那条 LEFT JOIN 的语义；
// 普通读路径仍必须用 Find，避免把软删文章当成可展示内容。
func (r *ArticleRepo) FindIncludingDeleted(_ context.Context, id domain.ArticleID) (*domain.Article, error) {
	r.count.inc("FindIncludingDeleted")
	if err := r.fault.take("FindIncludingDeleted"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrArticleNotFound
	}
	return cloneArticle(a), nil
}

// FindMany 批量读取。
func (r *ArticleRepo) FindMany(_ context.Context, ids []domain.ArticleID) ([]*domain.Article, error) {
	r.count.inc("FindMany")
	if err := r.fault.take("FindMany"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.Article, 0, len(ids))
	for _, id := range ids {
		if a, ok := r.byID[id]; ok && !a.Deleted() {
			out = append(out, cloneArticle(a))
		}
	}
	return out, nil
}

// List 按条件分页。
func (r *ArticleRepo) List(_ context.Context, f domain.Filter, p domain.Page) ([]*domain.Article, int64, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, 0, err
	}
	p = p.Normalize()

	r.mu.Lock()
	defer r.mu.Unlock()

	var matched []*domain.Article
	for _, id := range r.seq {
		a, ok := r.byID[id]
		if !ok || a.Deleted() {
			continue
		}
		if !matchFilter(a, f) {
			continue
		}
		matched = append(matched, cloneArticle(a))
	}

	total := int64(len(matched))
	start := p.Offset
	if start > len(matched) {
		start = len(matched)
	}
	end := start + p.Limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

// Create 新增；ID 重复返回 ErrConflict。
func (r *ArticleRepo) Create(_ context.Context, a *domain.Article) error {
	r.count.inc("Create")
	if err := r.fault.take("Create"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[a.ID()]; ok {
		return ErrConflict
	}
	r.seq = append(r.seq, a.ID())
	r.byID[a.ID()] = cloneArticle(a)
	return nil
}

// Update 保存变更；不存在返回 ErrArticleNotFound。
func (r *ArticleRepo) Update(_ context.Context, a *domain.Article) error {
	r.count.inc("Update")
	if err := r.fault.take("Update"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[a.ID()]; !ok {
		return domain.ErrArticleNotFound
	}
	r.byID[a.ID()] = cloneArticle(a)
	return nil
}

// SoftDelete 标记删除。
func (r *ArticleRepo) SoftDelete(_ context.Context, id domain.ArticleID) error {
	r.count.inc("SoftDelete")
	if err := r.fault.take("SoftDelete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return domain.ErrArticleNotFound
	}
	clone := cloneArticle(a)
	clone.SoftDelete(clone.UpdatedAt())
	r.byID[id] = clone
	return nil
}

// Snapshot 参与事务回滚。
func (r *ArticleRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := make(map[domain.ArticleID]*domain.Article, len(r.byID))
	for k, v := range r.byID {
		state[k] = cloneArticle(v)
	}
	seq := append([]domain.ArticleID(nil), r.seq...)
	return articleState{byID: state, seq: seq}
}

// RestoreState 覆盖回快照。
func (r *ArticleRepo) RestoreState(s any) {
	state, ok := s.(articleState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = state.byID
	r.seq = state.seq
}

type articleState struct {
	byID map[domain.ArticleID]*domain.Article
	seq  []domain.ArticleID
}

// IDs 返回当前全部文章 ID（稳定顺序），便于断言。
func (r *ArticleRepo) IDs() []domain.ArticleID {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]domain.ArticleID(nil), r.seq...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// cloneArticle 通过 Restore 重建一份快照，避免仓储与调用方共享指针。
func cloneArticle(a *domain.Article) *domain.Article {
	if a == nil {
		return nil
	}
	return domain.Restore(domain.RestoreParams{
		ID:             a.ID(),
		AuthorID:       a.AuthorID(),
		Title:          a.Title(),
		Abstract:       a.Abstract(),
		Content:        a.Content(),
		CategoryID:     a.CategoryID(),
		Cover:          a.Cover(),
		CommentsToggle: a.CommentsToggle(),
		Status:         a.Status(),
		Visibility:     a.Visibility(),
		SubmittedAt:    a.SubmittedAt(),
		ReviewedAt:     a.ReviewedAt(),
		ReviewedBy:     a.ReviewedBy(),
		Deleted:        a.Deleted(),
		Counters:       a.Counters(),
		Tags:           a.Tags(),
		CreatedAt:      a.CreatedAt(),
		UpdatedAt:      a.UpdatedAt(),
	})
}

// detachCategory 把引用该分类的文章置空，返回受影响篇数。
//
// 由 CategoryRepo.Delete 调用：生产里"删除分类"的第一步就是这条批量 UPDATE
// （见 adapter/mysql 的 CategoryRepo.Delete），内存实现也必须真的发生，
// 否则用例层断言不了"文章变未分类"。
func (r *ArticleRepo) detachCategory(id domain.CategoryID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, a := range r.byID {
		got := a.CategoryID()
		if got == nil || *got != id {
			continue
		}
		// 与编辑接口同一条路径（CategorySet + 空 ID = 清空），
		// 保证"分类被删"和"作者手动改成未分类"落库形态一致。
		if _, err := a.ApplyEdit(domain.EditPatch{CategorySet: true}, DefaultTime); err != nil {
			continue
		}
		n++
	}
	return n
}

// matchFilter 在内存里复现 Filter 语义，保证 fake 与真库行为一致。
func matchFilter(a *domain.Article, f domain.Filter) bool {
	if f.OnlyPublic && !a.IsPubliclyVisible() {
		return false
	}
	if f.AuthorID != nil && a.AuthorID() != *f.AuthorID {
		return false
	}
	if f.CategoryID != nil {
		got := a.CategoryID()
		if got == nil || *got != *f.CategoryID {
			return false
		}
	}
	if len(f.Statuses) > 0 {
		found := false
		for _, s := range f.Statuses {
			if a.Status() == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.TagIDs) > 0 {
		have := a.Tags()
		found := false
		for _, want := range f.TagIDs {
			for _, got := range have {
				if got == want {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	if f.Keyword != "" {
		kw := strings.ToLower(f.Keyword)
		if !strings.Contains(strings.ToLower(a.Title()), kw) &&
			!strings.Contains(strings.ToLower(a.Abstract()), kw) {
			return false
		}
	}
	return true
}

// AuthorContentStats 汇总某作者的内容数字（票 354）。
func (r *ArticleRepo) AuthorContentStats(_ context.Context, authorID domain.UserID) (domain.ContentStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stats := domain.ContentStats{StatusCounts: make(map[domain.Status]int64)}
	for _, a := range r.byID {
		if a.AuthorID() != authorID || a.Deleted() {
			continue
		}
		stats.StatusCounts[a.Status()]++
		c := a.Counters()
		stats.SumView += int64(c.View)
		stats.SumDigg += int64(c.Digg)
		stats.SumComment += int64(c.Comment)
		stats.SumFavor += int64(c.Favor)
	}
	return stats, nil
}

// ListRecentByAuthor 按创建时间倒序列出某作者最近的文章。
func (r *ArticleRepo) ListRecentByAuthor(_ context.Context, authorID domain.UserID, limit int) ([]*domain.Article, error) {
	return r.listByAuthor(authorID, limit, func(a, b *domain.Article) bool {
		return a.CreatedAt().After(b.CreatedAt())
	})
}

// ListTopByAuthor 按阅读量倒序列出某作者的文章。
func (r *ArticleRepo) ListTopByAuthor(_ context.Context, authorID domain.UserID, limit int) ([]*domain.Article, error) {
	return r.listByAuthor(authorID, limit, func(a, b *domain.Article) bool {
		return a.Counters().View > b.Counters().View
	})
}

// listByAuthor 是上面两个读面的共用实现：只差一个比较函数。
func (r *ArticleRepo) listByAuthor(
	authorID domain.UserID, limit int, less func(a, b *domain.Article) bool,
) ([]*domain.Article, error) {
	if limit <= 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	picked := make([]*domain.Article, 0, len(r.byID))
	for _, a := range r.byID {
		if a.AuthorID() != authorID || a.Deleted() {
			continue
		}
		picked = append(picked, cloneArticle(a))
	}
	// 插入排序：条数以百计，稳定且不引入 sort 包的比较器噪音。
	for i := 1; i < len(picked); i++ {
		for j := i; j > 0 && less(picked[j], picked[j-1]); j-- {
			picked[j], picked[j-1] = picked[j-1], picked[j]
		}
	}
	if len(picked) > limit {
		picked = picked[:limit]
	}
	return picked, nil
}