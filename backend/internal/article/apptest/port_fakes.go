package apptest

import (
	"context"
	"sync"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// Counters 是 app.CounterPort 的内存实现。
//
// 只存增量，不存绝对值——与生产里 Redis 计数语义一致。
type Counters struct {
	mu    sync.Mutex
	byID  map[domain.ArticleID]domain.Counters
	fault fault
	count counter
}

// NewCounters 构造空的计数 fake。
func NewCounters() *Counters {
	return &Counters{byID: make(map[domain.ArticleID]domain.Counters)}
}

// FailNext 注入一次性故障。
func (c *Counters) FailNext(method string, err error) { c.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (c *Counters) Calls(method string) int { return c.count.Calls(method) }

// Add 累加增量。
func (c *Counters) Add(_ context.Context, id domain.ArticleID, delta domain.Counters) error {
	c.count.inc("Add")
	if err := c.fault.take("Add"); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.byID[id]
	cur.View += delta.View
	cur.Digg += delta.Digg
	cur.Comment += delta.Comment
	cur.Favor += delta.Favor
	c.byID[id] = cur
	return nil
}

// Load 批量读取增量。
func (c *Counters) Load(_ context.Context, ids []domain.ArticleID) (map[domain.ArticleID]domain.Counters, error) {
	c.count.inc("Load")
	if err := c.fault.take("Load"); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[domain.ArticleID]domain.Counters, len(ids))
	for _, id := range ids {
		out[id] = c.byID[id]
	}
	return out, nil
}

// Get 直接读取某文章的增量（测试断言用）。
func (c *Counters) Get(id domain.ArticleID) domain.Counters {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[id]
}

// Comments 是 app.CommentPort 的内存实现。
type Comments struct {
	mu        sync.Mutex
	byArticle map[domain.ArticleID][]uint64
	fault     fault
	count     counter
	nextID    ctype.ID
}

// NewComments 构造空的评论 fake。
func NewComments() *Comments {
	return &Comments{byArticle: make(map[domain.ArticleID][]uint64)}
}

// Seed 预置某文章的评论。
func (c *Comments) Seed(articleID domain.ArticleID, n int) *Comments {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < n; i++ {
		c.nextID = ctype.NewID(c.nextID.Uint64() + 1)
		c.byArticle[articleID] = append(c.byArticle[articleID], c.nextID.Uint64())
	}
	return c
}

// FailNext 注入一次性故障。
func (c *Comments) FailNext(method string, err error) { c.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (c *Comments) Calls(method string) int { return c.count.Calls(method) }

// DeleteByArticle 删除某文章全部评论，返回条数。
func (c *Comments) DeleteByArticle(_ context.Context, articleID domain.ArticleID) (int, error) {
	c.count.inc("DeleteByArticle")
	if err := c.fault.take("DeleteByArticle"); err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.byArticle[articleID])
	delete(c.byArticle, articleID)
	return n, nil
}

// CountByArticle 统计评论数。
func (c *Comments) CountByArticle(_ context.Context, articleID domain.ArticleID) (int, error) {
	c.count.inc("CountByArticle")
	if err := c.fault.take("CountByArticle"); err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byArticle[articleID]), nil
}

// Users 是 app.UserPort 的内存实现。
type Users struct {
	mu    sync.Mutex
	byID  map[domain.UserID]app.AuthorView
	fault fault
	count counter
}

// NewUsers 构造空的用户 fake。
func NewUsers() *Users {
	return &Users{byID: make(map[domain.UserID]app.AuthorView)}
}

// Seed 预置用户。
func (u *Users) Seed(views ...app.AuthorView) *Users {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, v := range views {
		u.byID[v.ID] = v
	}
	return u
}

// FailNext 注入一次性故障。
func (u *Users) FailNext(method string, err error) { u.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (u *Users) Calls(method string) int { return u.count.Calls(method) }

// Profile 取单个用户。
func (u *Users) Profile(_ context.Context, id domain.UserID) (app.AuthorView, error) {
	u.count.inc("Profile")
	if err := u.fault.take("Profile"); err != nil {
		return app.AuthorView{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	v, ok := u.byID[id]
	if !ok {
		// 与生产一致：用户不存在时返回空资料而不是报错，
		// 让文章仍能展示（作者昵称留空），避免一个脏 ID 让整页 500。
		return app.AuthorView{ID: id}, nil
	}
	return v, nil
}

// Profiles 批量取用户。
func (u *Users) Profiles(_ context.Context, ids []domain.UserID) (map[domain.UserID]app.AuthorView, error) {
	u.count.inc("Profiles")
	if err := u.fault.take("Profiles"); err != nil {
		return nil, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make(map[domain.UserID]app.AuthorView, len(ids))
	for _, id := range ids {
		if v, ok := u.byID[id]; ok {
			out[id] = v
			continue
		}
		out[id] = app.AuthorView{ID: id}
	}
	return out, nil
}

// Diggs 是 app.DiggPort 的内存实现。
type Diggs struct {
	mu    sync.Mutex
	set   map[itemKey2]struct{}
	fault fault
	count counter
}

// NewDiggs 构造空的点赞 fake。
func NewDiggs() *Diggs {
	return &Diggs{set: make(map[itemKey2]struct{})}
}

// FailNext 注入一次性故障。
func (d *Diggs) FailNext(method string, err error) { d.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (d *Diggs) Calls(method string) int { return d.count.Calls(method) }

// Exists 判断是否已点赞。
func (d *Diggs) Exists(_ context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	d.count.inc("Exists")
	if err := d.fault.take("Exists"); err != nil {
		return false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.set[itemKey2{userID, articleID}]
	return ok, nil
}

// Add 新增点赞；已存在返回 false。
func (d *Diggs) Add(_ context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	d.count.inc("Add")
	if err := d.fault.take("Add"); err != nil {
		return false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := itemKey2{userID, articleID}
	if _, ok := d.set[key]; ok {
		return false, nil
	}
	d.set[key] = struct{}{}
	return true, nil
}

// Remove 取消点赞；本不存在返回 false。
func (d *Diggs) Remove(_ context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error) {
	d.count.inc("Remove")
	if err := d.fault.take("Remove"); err != nil {
		return false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := itemKey2{userID, articleID}
	if _, ok := d.set[key]; !ok {
		return false, nil
	}
	delete(d.set, key)
	return true, nil
}

// DeleteByArticle 删除某文章全部点赞，返回条数。
func (d *Diggs) DeleteByArticle(_ context.Context, articleID domain.ArticleID) (int, error) {
	d.count.inc("DeleteByArticle")
	if err := d.fault.take("DeleteByArticle"); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for k := range d.set {
		if k.articleID == articleID {
			delete(d.set, k)
			n++
		}
	}
	return n, nil
}

// Count 返回某文章的点赞数（测试断言用）。
func (d *Diggs) Count(articleID domain.ArticleID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for k := range d.set {
		if k.articleID == articleID {
			n++
		}
	}
	return n
}
