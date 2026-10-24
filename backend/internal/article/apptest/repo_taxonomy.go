package apptest

import (
	"context"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
)

// CategoryRepo 是 taxonomy.CategoryRepository 的内存实现。
//
// 唯一约束 (userID, title) 与库里 uk_category_user_title 一致；
// Save 撞唯一键返回 ErrConflict。
type CategoryRepo struct {
	mu    sync.Mutex
	byID  map[domain.CategoryID]*taxonomy.Category
	seq   []domain.CategoryID
	fault fault
	count counter
}

// NewCategoryRepo 创建空仓储。
func NewCategoryRepo() *CategoryRepo {
	return &CategoryRepo{byID: make(map[domain.CategoryID]*taxonomy.Category)}
}

// Seed 预置数据。
func (r *CategoryRepo) Seed(items ...*taxonomy.Category) *CategoryRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range items {
		cp := cloneCategory(c)
		if _, ok := r.byID[cp.ID()]; !ok {
			r.seq = append(r.seq, cp.ID())
		}
		r.byID[cp.ID()] = cp
	}
	return r
}

// FailNext 注入一次性故障。
func (r *CategoryRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *CategoryRepo) Calls(method string) int { return r.count.Calls(method) }

// Find 读取分类。
func (r *CategoryRepo) Find(_ context.Context, id domain.CategoryID) (*taxonomy.Category, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok {
		return nil, taxonomy.ErrCategoryNotFound
	}
	return cloneCategory(c), nil
}

// ListByUser 列出某用户的分类。
func (r *CategoryRepo) ListByUser(_ context.Context, userID domain.UserID) ([]*taxonomy.Category, error) {
	r.count.inc("ListByUser")
	if err := r.fault.take("ListByUser"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*taxonomy.Category
	for _, id := range r.seq {
		if c, ok := r.byID[id]; ok && c.UserID() == userID {
			out = append(out, cloneCategory(c))
		}
	}
	return out, nil
}

// ExistsTitle 判断同用户下名称是否重复。
func (r *CategoryRepo) ExistsTitle(_ context.Context, userID domain.UserID, title string, excludeID domain.CategoryID) (bool, error) {
	r.count.inc("ExistsTitle")
	if err := r.fault.take("ExistsTitle"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.byID {
		if id == excludeID {
			continue
		}
		if c.UserID() == userID && c.Title() == title {
			return true, nil
		}
	}
	return false, nil
}

// Save 新建或更新。
func (r *CategoryRepo) Save(_ context.Context, c *taxonomy.Category) error {
	r.count.inc("Save")
	if err := r.fault.take("Save"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, other := range r.byID {
		if id == c.ID() {
			continue
		}
		if other.UserID() == c.UserID() && other.Title() == c.Title() {
			return ErrConflict
		}
	}
	if _, ok := r.byID[c.ID()]; !ok {
		r.seq = append(r.seq, c.ID())
	}
	r.byID[c.ID()] = cloneCategory(c)
	return nil
}

// Delete 删除分类。
func (r *CategoryRepo) Delete(_ context.Context, id domain.CategoryID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return taxonomy.ErrCategoryNotFound
	}
	delete(r.byID, id)
	return nil
}

// Snapshot 参与事务回滚。
func (r *CategoryRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := make(map[domain.CategoryID]*taxonomy.Category, len(r.byID))
	for k, v := range r.byID {
		state[k] = cloneCategory(v)
	}
	return categoryState{byID: state, seq: append([]domain.CategoryID(nil), r.seq...)}
}

// RestoreState 覆盖回快照。
func (r *CategoryRepo) RestoreState(s any) {
	state, ok := s.(categoryState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = state.byID
	r.seq = state.seq
}

type categoryState struct {
	byID map[domain.CategoryID]*taxonomy.Category
	seq  []domain.CategoryID
}

func cloneCategory(c *taxonomy.Category) *taxonomy.Category {
	if c == nil {
		return nil
	}
	return taxonomy.RestoreCategory(c.ID(), c.UserID(), c.Title())
}

// TagRepo 是 taxonomy.TagRepository 的内存实现。
//
// 同时保存"文章 → 标签"的关联，因为 ReplaceArticleTags 与 ArticleTagIDs
// 是标签域的职责（对应库里的 article_tag_models 关系表）。
type TagRepo struct {
	mu          sync.Mutex
	byID        map[domain.TagID]*taxonomy.Tag
	seq         []domain.TagID
	articleTags map[domain.ArticleID][]domain.TagID
	fault       fault
	count       counter
}

// NewTagRepo 创建空仓储。
func NewTagRepo() *TagRepo {
	return &TagRepo{
		byID:        make(map[domain.TagID]*taxonomy.Tag),
		articleTags: make(map[domain.ArticleID][]domain.TagID),
	}
}

// Seed 预置标签。
func (r *TagRepo) Seed(tags ...*taxonomy.Tag) *TagRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		cp := cloneTag(t)
		if _, ok := r.byID[cp.ID()]; !ok {
			r.seq = append(r.seq, cp.ID())
		}
		r.byID[cp.ID()] = cp
	}
	return r
}

// SeedArticleTags 预置文章与标签的关联。
func (r *TagRepo) SeedArticleTags(articleID domain.ArticleID, tagIDs ...domain.TagID) *TagRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.articleTags[articleID] = append([]domain.TagID(nil), tagIDs...)
	return r
}

// FailNext 注入一次性故障。
func (r *TagRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *TagRepo) Calls(method string) int { return r.count.Calls(method) }

// Find 读取标签。
func (r *TagRepo) Find(_ context.Context, id domain.TagID) (*taxonomy.Tag, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return nil, taxonomy.ErrTagNotFound
	}
	return cloneTag(t), nil
}

// FindMany 批量读取；缺失的 ID 直接跳过。
func (r *TagRepo) FindMany(_ context.Context, ids []domain.TagID) ([]*taxonomy.Tag, error) {
	r.count.inc("FindMany")
	if err := r.fault.take("FindMany"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*taxonomy.Tag, 0, len(ids))
	for _, id := range ids {
		if t, ok := r.byID[id]; ok {
			out = append(out, cloneTag(t))
		}
	}
	return out, nil
}

// List 列出标签。
func (r *TagRepo) List(_ context.Context, includeDisabled bool) ([]*taxonomy.Tag, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*taxonomy.Tag
	for _, id := range r.seq {
		if t, ok := r.byID[id]; ok && (includeDisabled || t.Enabled()) {
			out = append(out, cloneTag(t))
		}
	}
	return out, nil
}

// ExistsTitle 判断标签名是否重复。
func (r *TagRepo) ExistsTitle(_ context.Context, title string, excludeID domain.TagID) (bool, error) {
	r.count.inc("ExistsTitle")
	if err := r.fault.take("ExistsTitle"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.byID {
		if id == excludeID {
			continue
		}
		if t.Title() == title {
			return true, nil
		}
	}
	return false, nil
}

// Save 新建或更新标签。
func (r *TagRepo) Save(_ context.Context, t *taxonomy.Tag) error {
	r.count.inc("Save")
	if err := r.fault.take("Save"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID()]; !ok {
		r.seq = append(r.seq, t.ID())
	}
	r.byID[t.ID()] = cloneTag(t)
	return nil
}

// Delete 删除标签及关联。
func (r *TagRepo) Delete(_ context.Context, id domain.TagID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return taxonomy.ErrTagNotFound
	}
	delete(r.byID, id)
	for aid, tags := range r.articleTags {
		kept := tags[:0]
		for _, t := range tags {
			if t != id {
				kept = append(kept, t)
			}
		}
		r.articleTags[aid] = kept
	}
	return nil
}

// ReplaceArticleTags 用给定集合替换文章标签。
func (r *TagRepo) ReplaceArticleTags(_ context.Context, articleID domain.ArticleID, tagIDs []domain.TagID) error {
	r.count.inc("ReplaceArticleTags")
	if err := r.fault.take("ReplaceArticleTags"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.articleTags[articleID] = append([]domain.TagID(nil), tagIDs...)
	return nil
}

// ArticleTagIDs 读取文章标签。
func (r *TagRepo) ArticleTagIDs(_ context.Context, articleID domain.ArticleID) ([]domain.TagID, error) {
	r.count.inc("ArticleTagIDs")
	if err := r.fault.take("ArticleTagIDs"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.TagID(nil), r.articleTags[articleID]...), nil
}

// Snapshot 参与事务回滚。
func (r *TagRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := make(map[domain.TagID]*taxonomy.Tag, len(r.byID))
	for k, v := range r.byID {
		byID[k] = cloneTag(v)
	}
	links := make(map[domain.ArticleID][]domain.TagID, len(r.articleTags))
	for k, v := range r.articleTags {
		links[k] = append([]domain.TagID(nil), v...)
	}
	return tagState{byID: byID, seq: append([]domain.TagID(nil), r.seq...), links: links}
}

// RestoreState 覆盖回快照。
func (r *TagRepo) RestoreState(s any) {
	state, ok := s.(tagState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = state.byID
	r.seq = state.seq
	r.articleTags = state.links
}

type tagState struct {
	byID  map[domain.TagID]*taxonomy.Tag
	seq   []domain.TagID
	links map[domain.ArticleID][]domain.TagID
}

func cloneTag(t *taxonomy.Tag) *taxonomy.Tag {
	if t == nil {
		return nil
	}
	return taxonomy.RestoreTag(taxonomy.RestoreTagParams{
		ID:           t.ID(),
		Title:        t.Title(),
		Sort:         t.Sort(),
		Description:  t.Description(),
		ArticleCount: t.ArticleCount(),
		Enabled:      t.Enabled(),
	})
}
