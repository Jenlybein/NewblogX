package apptest

import (
	"context"
	"myblogx/internal/kernel/ctype"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
)

// FavoriteRepo 是 favorite.Repository 的内存实现。
//
// DefaultFolder 会在不存在时**自动创建**默认收藏夹——这是旧实现的行为，
// 用户第一次收藏不需要先建夹子。测试要能覆盖这条路径。
type FavoriteRepo struct {
	mu      sync.Mutex
	folders map[favorite.ID]*favorite.Folder
	items   map[itemKey]*favorite.Item
	nextID  favorite.ID
	fault   fault
	count   counter
	// lastOnlyAdminAuthors 记录 ListItems 收到的读过滤开关（见该方法注释）。
	lastOnlyAdminAuthors bool
}

type itemKey struct {
	articleID domain.ArticleID
	userID    domain.UserID
	folderID  favorite.ID
}

// NewFavoriteRepo 创建空仓储。
func NewFavoriteRepo() *FavoriteRepo {
	return &FavoriteRepo{
		folders: make(map[favorite.ID]*favorite.Folder),
		items:   make(map[itemKey]*favorite.Item),
	}
}

// SeedFolder 预置收藏夹。
func (r *FavoriteRepo) SeedFolder(f *favorite.Folder) *FavoriteRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.folders[f.ID()] = cloneFolder(f)
	if f.ID().Uint64() > r.nextID.Uint64() {
		r.nextID = f.ID()
	}
	return r
}

// FailNext 注入一次性故障。
func (r *FavoriteRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *FavoriteRepo) Calls(method string) int { return r.count.Calls(method) }

// FindFolder 读取收藏夹。
func (r *FavoriteRepo) FindFolder(_ context.Context, id favorite.ID) (*favorite.Folder, error) {
	r.count.inc("FindFolder")
	if err := r.fault.take("FindFolder"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.folders[id]
	if !ok {
		return nil, favorite.ErrFolderNotFound
	}
	return cloneFolder(f), nil
}

// DefaultFolder 取默认收藏夹，不存在则创建。
func (r *FavoriteRepo) DefaultFolder(_ context.Context, userID domain.UserID) (*favorite.Folder, error) {
	r.count.inc("DefaultFolder")
	if err := r.fault.take("DefaultFolder"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, f := range r.folders {
		if f.UserID() == userID && f.IsDefault() {
			return cloneFolder(f), nil
		}
	}

	r.nextID = ctype.NewID(r.nextID.Uint64() + 1)
	id := r.nextID
	created := favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: id, UserID: userID, Title: "默认收藏", IsDefault: true,
	})
	r.folders[id] = created
	return cloneFolder(created), nil
}

// ListFolders 列出某用户的收藏夹。
func (r *FavoriteRepo) ListFolders(_ context.Context, userID domain.UserID) ([]*favorite.Folder, error) {
	r.count.inc("ListFolders")
	if err := r.fault.take("ListFolders"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*favorite.Folder
	for _, f := range r.folders {
		if f.UserID() == userID {
			out = append(out, cloneFolder(f))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID().Uint64() < out[j].ID().Uint64() })
	return out, nil
}

// ExistsFolderTitle 判断名称重复。
func (r *FavoriteRepo) ExistsFolderTitle(_ context.Context, userID domain.UserID, title string, excludeID favorite.ID) (bool, error) {
	r.count.inc("ExistsFolderTitle")
	if err := r.fault.take("ExistsFolderTitle"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, f := range r.folders {
		if id == excludeID {
			continue
		}
		if f.UserID() == userID && f.Title() == title {
			return true, nil
		}
	}
	return false, nil
}

// SaveFolder 新建或更新收藏夹。
func (r *FavoriteRepo) SaveFolder(_ context.Context, f *favorite.Folder) error {
	r.count.inc("SaveFolder")
	if err := r.fault.take("SaveFolder"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, other := range r.folders {
		if id == f.ID() {
			continue
		}
		if other.UserID() == f.UserID() && other.Title() == f.Title() {
			return ErrConflict
		}
	}
	r.folders[f.ID()] = cloneFolder(f)
	if f.ID().Uint64() > r.nextID.Uint64() {
		r.nextID = f.ID()
	}
	return nil
}

// DeleteFolder 删除收藏夹与其下收藏。
func (r *FavoriteRepo) DeleteFolder(_ context.Context, id favorite.ID) error {
	r.count.inc("DeleteFolder")
	if err := r.fault.take("DeleteFolder"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.folders[id]; !ok {
		return favorite.ErrFolderNotFound
	}
	delete(r.folders, id)
	for k := range r.items {
		if k.folderID == id {
			delete(r.items, k)
		}
	}
	return nil
}

// FindItem 读取某夹子里的收藏。
func (r *FavoriteRepo) FindItem(_ context.Context, articleID domain.ArticleID, userID domain.UserID, folderID favorite.ID) (*favorite.Item, error) {
	r.count.inc("FindItem")
	if err := r.fault.take("FindItem"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	it, ok := r.items[itemKey{articleID, userID, folderID}]
	if !ok {
		return nil, favorite.ErrItemNotFound
	}
	return cloneItem(it), nil
}

// FindItemAnyFolder 读取某用户对某文章的任意收藏。
func (r *FavoriteRepo) FindItemAnyFolder(_ context.Context, articleID domain.ArticleID, userID domain.UserID) (*favorite.Item, error) {
	r.count.inc("FindItemAnyFolder")
	if err := r.fault.take("FindItemAnyFolder"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, it := range r.items {
		if k.articleID == articleID && k.userID == userID {
			return cloneItem(it), nil
		}
	}
	return nil, favorite.ErrItemNotFound
}

// SaveItem 新建收藏。
func (r *FavoriteRepo) SaveItem(_ context.Context, i *favorite.Item) error {
	r.count.inc("SaveItem")
	if err := r.fault.take("SaveItem"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := itemKey{i.ArticleID(), i.UserID(), i.FolderID()}
	if _, ok := r.items[key]; ok {
		return ErrConflict
	}
	r.items[key] = cloneItem(i)
	return nil
}

// DeleteItem 删除收藏。
func (r *FavoriteRepo) DeleteItem(_ context.Context, articleID domain.ArticleID, userID domain.UserID, folderID favorite.ID) error {
	r.count.inc("DeleteItem")
	if err := r.fault.take("DeleteItem"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := itemKey{articleID, userID, folderID}
	if _, ok := r.items[key]; !ok {
		return favorite.ErrItemNotFound
	}
	delete(r.items, key)
	return nil
}

// DeleteItemsByArticle 删除某文章的全部收藏。
func (r *FavoriteRepo) DeleteItemsByArticle(_ context.Context, articleID domain.ArticleID) error {
	r.count.inc("DeleteItemsByArticle")
	if err := r.fault.take("DeleteItemsByArticle"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.items {
		if k.articleID == articleID {
			delete(r.items, k)
		}
	}
	return nil
}

// ListItems 分页列出某夹子的收藏。
//
// 内存 fake 没有用户表，拿不到"作者是不是管理员"，所以它**不做**过滤，
// 只把收到的过滤开关记下来供断言"app 把开关传给了查询层"；过滤语义本身
// 由 MySQL 集成测试守（adapter/mysql/social_repo_integration_test.go）。
func (r *FavoriteRepo) ListItems(_ context.Context, folderID favorite.ID, offset, limit int, onlyAdminAuthors bool) ([]*favorite.Item, int64, error) {
	r.count.inc("ListItems")
	if err := r.fault.take("ListItems"); err != nil {
		return nil, 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors
	var all []*favorite.Item
	for k, it := range r.items {
		if k.folderID == folderID {
			all = append(all, cloneItem(it))
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ArticleID().Uint64() > all[j].ArticleID().Uint64() })

	total := int64(len(all))
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

// ListItemsOnlyAdminAuthors 返回上一次 ListItems 收到的过滤开关。
func (r *FavoriteRepo) ListItemsOnlyAdminAuthors() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastOnlyAdminAuthors
}

// CountByArticle 统计某文章被收藏的次数。
func (r *FavoriteRepo) CountByArticle(_ context.Context, articleID domain.ArticleID) (int, error) {
	r.count.inc("CountByArticle")
	if err := r.fault.take("CountByArticle"); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k := range r.items {
		if k.articleID == articleID {
			n++
		}
	}
	return n, nil
}

// Snapshot 参与事务回滚。
func (r *FavoriteRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	folders := make(map[favorite.ID]*favorite.Folder, len(r.folders))
	for k, v := range r.folders {
		folders[k] = cloneFolder(v)
	}
	items := make(map[itemKey]*favorite.Item, len(r.items))
	for k, v := range r.items {
		items[k] = cloneItem(v)
	}
	return favoriteState{folders: folders, items: items, nextID: r.nextID}
}

// RestoreState 覆盖回快照。
func (r *FavoriteRepo) RestoreState(s any) {
	state, ok := s.(favoriteState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.folders = state.folders
	r.items = state.items
	r.nextID = state.nextID
}

type favoriteState struct {
	folders map[favorite.ID]*favorite.Folder
	items   map[itemKey]*favorite.Item
	nextID  favorite.ID
}

func cloneFolder(f *favorite.Folder) *favorite.Folder {
	if f == nil {
		return nil
	}
	return favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: f.ID(), UserID: f.UserID(), Title: f.Title(), Cover: f.Cover(),
		Abstract: f.Abstract(), IsDefault: f.IsDefault(), ArticleCount: f.ArticleCount(),
	})
}

func cloneItem(i *favorite.Item) *favorite.Item {
	if i == nil {
		return nil
	}
	return favorite.RestoreItem(favorite.RestoreItemParams{
		ArticleID:       i.ArticleID(),
		UserID:          i.UserID(),
		FolderID:        i.FolderID(),
		ArticleTitle:    i.ArticleTitle(),
		ArticleAbstract: i.ArticleAbstract(),
		ArticleCover:    i.ArticleCover(),
		ArticleStatus:   i.ArticleStatus(),
		AuthorID:        i.AuthorID(),
		AuthorNickname:  i.AuthorNickname(),
		AuthorAvatar:    i.AuthorAvatar(),
	})
}

// TopRepo 是 top.Repository 的内存实现。
type TopRepo struct {
	mu      sync.Mutex
	entries map[itemKey2]struct{}
	fault   fault
	count   counter
}

type itemKey2 struct {
	userID    domain.UserID
	articleID domain.ArticleID
}

// NewTopRepo 创建空仓储。
func NewTopRepo() *TopRepo {
	return &TopRepo{entries: make(map[itemKey2]struct{})}
}

// FailNext 注入一次性故障。
func (r *TopRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *TopRepo) Calls(method string) int { return r.count.Calls(method) }

// Save 新增置顶（幂等）。
func (r *TopRepo) Save(_ context.Context, e *top.Entry) error {
	r.count.inc("Save")
	if err := r.fault.take("Save"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[itemKey2{e.UserID(), e.ArticleID()}] = struct{}{}
	return nil
}

// Delete 取消置顶（幂等）。
func (r *TopRepo) Delete(_ context.Context, userID domain.UserID, articleID domain.ArticleID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, itemKey2{userID, articleID})
	return nil
}

// ListCards 列出置顶卡片。
//
// 内存实现没有文章表可 join，所以只返回置顶 ID 与 Title 占位——
// 覆盖的是"接口形状与调用路径"，具体 SQL 投影由集成测试保证。
func (r *TopRepo) ListCards(_ context.Context, userID domain.UserID) ([]top.Card, error) {
	r.count.inc("ListCards")
	if err := r.fault.take("ListCards"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []top.Card
	for k := range r.entries {
		if k.userID == userID {
			out = append(out, top.Card{ArticleID: k.articleID})
		}
	}
	return out, nil
}

// Exists 判断是否已置顶。
func (r *TopRepo) Exists(_ context.Context, userID domain.UserID, articleID domain.ArticleID) (bool, error) {
	r.count.inc("Exists")
	if err := r.fault.take("Exists"); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[itemKey2{userID, articleID}]
	return ok, nil
}

// DeleteByArticle 删除某文章的全部置顶。
func (r *TopRepo) DeleteByArticle(_ context.Context, articleID domain.ArticleID) error {
	r.count.inc("DeleteByArticle")
	if err := r.fault.take("DeleteByArticle"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.entries {
		if k.articleID == articleID {
			delete(r.entries, k)
		}
	}
	return nil
}

// Snapshot 参与事务回滚。
func (r *TopRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[itemKey2]struct{}, len(r.entries))
	for k := range r.entries {
		cp[k] = struct{}{}
	}
	return cp
}

// RestoreState 覆盖回快照。
func (r *TopRepo) RestoreState(s any) {
	if cp, ok := s.(map[itemKey2]struct{}); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.entries = cp
	}
}

// ViewHistoryRepo 是 viewhistory.Repository 的内存实现。
type ViewHistoryRepo struct {
	mu    sync.Mutex
	byKey map[itemKey2]*viewhistory.Record
	fault fault
	count counter
	// lastOnlyAdminAuthors 记录 List 收到的读过滤开关（见该方法注释）。
	lastOnlyAdminAuthors bool
}

// NewViewHistoryRepo 创建空仓储。
func NewViewHistoryRepo() *ViewHistoryRepo {
	return &ViewHistoryRepo{byKey: make(map[itemKey2]*viewhistory.Record)}
}

// FailNext 注入一次性故障。
func (r *ViewHistoryRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *ViewHistoryRepo) Calls(method string) int { return r.count.Calls(method) }

// Upsert 写入或刷新浏览记录。
func (r *ViewHistoryRepo) Upsert(_ context.Context, rec *viewhistory.Record) error {
	r.count.inc("Upsert")
	if err := r.fault.take("Upsert"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := itemKey2{rec.UserID(), rec.ArticleID()}
	r.byKey[key] = viewhistory.NewRecord(rec.UserID(), rec.ArticleID(), rec.ViewedAt())
	return nil
}

// List 列出浏览历史。
//
// 与收藏内容同理：fake 没有文章表，拿不到文章作者，所以不过滤、只记录开关；
// 真实过滤由 MySQL 集成测试守。
func (r *ViewHistoryRepo) List(_ context.Context, userID domain.UserID, offset, limit int, onlyAdminAuthors bool) ([]*viewhistory.Record, int64, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastOnlyAdminAuthors = onlyAdminAuthors
	var all []*viewhistory.Record
	for k, v := range r.byKey {
		if k.userID == userID {
			all = append(all, v)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ViewedAt().After(all[j].ViewedAt()) })

	total := int64(len(all))
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

// ListOnlyAdminAuthors 返回上一次 List 收到的过滤开关。
func (r *ViewHistoryRepo) ListOnlyAdminAuthors() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastOnlyAdminAuthors
}

// Delete 删除指定记录。
func (r *ViewHistoryRepo) Delete(_ context.Context, userID domain.UserID, articleIDs []domain.ArticleID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, aid := range articleIDs {
		delete(r.byKey, itemKey2{userID, aid})
	}
	return nil
}

// DeleteByArticle 删除某文章的全部记录。
func (r *ViewHistoryRepo) DeleteByArticle(_ context.Context, articleID domain.ArticleID) error {
	r.count.inc("DeleteByArticle")
	if err := r.fault.take("DeleteByArticle"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.byKey {
		if k.articleID == articleID {
			delete(r.byKey, k)
		}
	}
	return nil
}

// Snapshot 参与事务回滚。
func (r *ViewHistoryRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[itemKey2]*viewhistory.Record, len(r.byKey))
	for k, v := range r.byKey {
		cp[k] = v
	}
	return cp
}

// RestoreState 覆盖回快照。
func (r *ViewHistoryRepo) RestoreState(s any) {
	if cp, ok := s.(map[itemKey2]*viewhistory.Record); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.byKey = cp
	}
}

// ReviewRepo 是 review.Repository 的内存实现。
type ReviewRepo struct {
	mu    sync.Mutex
	byID  map[uint64]*review.Task
	seq   []uint64
	fault fault
	count counter
}

// NewReviewRepo 创建空仓储。
func NewReviewRepo() *ReviewRepo {
	return &ReviewRepo{byID: make(map[uint64]*review.Task)}
}

// Seed 预置任务。
func (r *ReviewRepo) Seed(tasks ...*review.Task) *ReviewRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tasks {
		cp := cloneTask(t)
		if _, ok := r.byID[cp.ID()]; !ok {
			r.seq = append(r.seq, cp.ID())
		}
		r.byID[cp.ID()] = cp
	}
	return r
}

// FailNext 注入一次性故障。
func (r *ReviewRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *ReviewRepo) Calls(method string) int { return r.count.Calls(method) }

// Save 新建任务。
func (r *ReviewRepo) Save(_ context.Context, t *review.Task) error {
	r.count.inc("Save")
	if err := r.fault.take("Save"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID()]; !ok {
		r.seq = append(r.seq, t.ID())
	}
	r.byID[t.ID()] = cloneTask(t)
	return nil
}

// Update 保存任务变更。
func (r *ReviewRepo) Update(_ context.Context, t *review.Task) error {
	r.count.inc("Update")
	if err := r.fault.take("Update"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID()]; !ok {
		return review.ErrTaskNotFound
	}
	r.byID[t.ID()] = cloneTask(t)
	return nil
}

// Find 读取任务。
func (r *ReviewRepo) Find(_ context.Context, id uint64) (*review.Task, error) {
	r.count.inc("Find")
	if err := r.fault.take("Find"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return nil, review.ErrTaskNotFound
	}
	return cloneTask(t), nil
}

// ListPendingByArticle 取某文章的待处理任务。
func (r *ReviewRepo) ListPendingByArticle(_ context.Context, articleID domain.ArticleID) ([]*review.Task, error) {
	r.count.inc("ListPendingByArticle")
	if err := r.fault.take("ListPendingByArticle"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*review.Task
	for _, id := range r.seq {
		if t, ok := r.byID[id]; ok && t.ArticleID() == articleID && t.IsPending() {
			out = append(out, cloneTask(t))
		}
	}
	return out, nil
}

// List 分页列出任务。
func (r *ReviewRepo) List(_ context.Context, status *review.Status, offset, limit int) ([]*review.Task, int64, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []*review.Task
	for _, id := range r.seq {
		t, ok := r.byID[id]
		if !ok {
			continue
		}
		if status != nil && t.Status() != *status {
			continue
		}
		all = append(all, cloneTask(t))
	}
	total := int64(len(all))
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

// Snapshot 参与事务回滚。
func (r *ReviewRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[uint64]*review.Task, len(r.byID))
	for k, v := range r.byID {
		cp[k] = cloneTask(v)
	}
	return reviewState{byID: cp, seq: append([]uint64(nil), r.seq...)}
}

// RestoreState 覆盖回快照。
func (r *ReviewRepo) RestoreState(s any) {
	state, ok := s.(reviewState)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = state.byID
	r.seq = state.seq
}

type reviewState struct {
	byID map[uint64]*review.Task
	seq  []uint64
}

func cloneTask(t *review.Task) *review.Task {
	if t == nil {
		return nil
	}
	return review.RestoreTask(review.RestoreTaskParams{
		ID:            ctype.NewID(t.ID()),
		ArticleID:     t.ArticleID(),
		AuthorID:      t.AuthorID(),
		ArticleTitle:  t.ArticleTitle(),
		AuthorName:    t.AuthorName(),
		PublishStatus: t.PublishStatus(),
		Stage:         t.Stage(),
		Source:        t.Source(),
		Status:        t.Status(),
		Reason:        t.Reason(),
		ReviewedAt:    t.ReviewedAt(),
		ReviewedBy:    t.ReviewedBy(),
		CreatedAt:     t.CreatedAt(),
	})
}
