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
