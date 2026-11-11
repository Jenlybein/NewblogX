package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
)

// CategoryRepo 是 taxonomy.CategoryRepository 的内存实现。
//
// 唯一约束 (userID, title) 与库里 uk_category_user_title 一致；
// Save 撞唯一键返回 ErrConflict。
//
// 删除要像生产一样连带"解除文章归属"（生产在一个事务里先置空文章的分类、
// 再软删分类），所以它需要文章 fake 的引用：没有接上时只删分类本身。
type CategoryRepo struct {
	mu       sync.Mutex
	byID     map[domain.CategoryID]*taxonomy.Category
	seq      []domain.CategoryID
	articles *ArticleRepo
	fault    fault
	count    counter
}

// NewCategoryRepo 创建空仓储。
func NewCategoryRepo() *CategoryRepo {
	return &CategoryRepo{byID: make(map[domain.CategoryID]*taxonomy.Category)}
}

// WithArticles 接上文章 fake：删除分类时把挂在该分类下的文章置空。
//
// 与 NewCollectionRepo(articles, ...) 同一做法：内存实现也要让"读者看到的结果"
// 真实发生，否则用例层只能断言"删分类的方法被调了"，断言不了"文章变未分类"。
func (r *CategoryRepo) WithArticles(articles *ArticleRepo) *CategoryRepo {
	r.articles = articles
	return r
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

// Delete 删除分类，并把挂在该分类下的文章置空（对应生产里的两步同事务）。
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
	if r.articles != nil {
		r.articles.detachCategory(id)
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
//
// 与生产读面同一口径：`sort desc, id asc`，`seq` 只作为同序时的稳定插入顺序。
// 拖拽排序的用例要断言"换序后列表真的变了"，这里必须真的按 sort 排。
func (r *TagRepo) List(_ context.Context, includeDisabled bool) ([]*taxonomy.Tag, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*taxonomy.Tag, 0, len(r.seq))
	for _, id := range r.sortedIDs() {
		if t, ok := r.byID[id]; ok && (includeDisabled || t.Enabled()) {
			out = append(out, cloneTag(t))
		}
	}
	return out, nil
}

// sortedIDs 按 `sort desc, id asc` 返回未删除标签的 ID（生产读面的内存等价实现）。
//
// 排序只做 sort 降序、同值时保留 seq 顺序：ID 是 bigint 的字面量，内存里按字符串
// 比较不是生产语义，用例也不会依赖同序时的 ID 先后。
//
// 调用方必须已持有 r.mu。
func (r *TagRepo) sortedIDs() []domain.TagID {
	ids := make([]domain.TagID, 0, len(r.seq))
	for _, id := range r.seq {
		if _, ok := r.byID[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return r.byID[ids[i]].Sort() > r.byID[ids[j]].Sort()
	})
	return ids
}

// ListIDs 返回全部未删除标签的 ID（换序的集合校验用）。
func (r *TagRepo) ListIDs(_ context.Context) ([]domain.TagID, error) {
	r.count.inc("ListIDs")
	if err := r.fault.take("ListIDs"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sortedIDs(), nil
}

// UpdateSortOrders 把给定顺序落成 sort 值（第 0 个拿最大 sort，读面降序展示）。
func (r *TagRepo) UpdateSortOrders(_ context.Context, ids []domain.TagID) error {
	r.count.inc("UpdateSortOrders")
	if err := r.fault.take("UpdateSortOrders"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(ids)
	for i, id := range ids {
		t, ok := r.byID[id]
		if !ok {
			continue // 集合一致性已由用例层校验，脏 ID 不在这里报错
		}
		if err := t.Rename(t.Title(), n-i, t.Description()); err != nil {
			return err
		}
	}
	return nil
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

// CountArticleRefs 统计仍引用该标签的文章数（对应关联表的实时统计）。
func (r *TagRepo) CountArticleRefs(_ context.Context, id domain.TagID) (int, error) {
	r.count.inc("CountArticleRefs")
	if err := r.fault.take("CountArticleRefs"); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, tags := range r.articleTags {
		for _, t := range tags {
			if t == id {
				n++
				break
			}
		}
	}
	return n, nil
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
