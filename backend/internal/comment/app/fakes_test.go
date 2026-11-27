package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 评论用例层的替身与 fixture。
//
// 票 400 之后这里只剩"仓储 / 缓存 / 通知 / 日志"这类基础依赖：评论审核的
// 策略快照与 AI 判定端口都已退场，不再需要它们的假实现。
//
// 管理端评论管理（列表 / 批量删除 / 禁言 / 封禁）把 fakeRepo 的 FindByID 与
// ListFamily 从"永远 not_found"扩成可预置的映射：批量删除要验证级联、逐条失败
// 与"已删过"的幂等语义，光靠"永远查不到"是钉不住的。

// fakeMuteCall 是一次禁言调用。
type fakeMuteCall struct {
	actor  ctype.ID
	target ctype.ID
	days   int
}

// fakeBanCall 是一次封禁调用。
type fakeBanCall struct {
	actor  ctype.ID
	target ctype.ID
}

// fakeRepo 只记住"落了什么 / 查到了什么"，不模拟 SQL。
type fakeRepo struct {
	created []*domain.Comment
	// published 是回复目标查找用的预置评论（按 ID）。
	published map[uint64]domain.Comment

	// byID 是 FindByID 能查到的行（管理端批量删除用；不在表里 = not_found）。
	byID map[uint64]domain.Comment
	// findErrors 让某条 FindByID 报指定错误（批量删除的"部分失败"靠它）。
	findErrors map[uint64]error
	// family 是 ListFamily 的预置结果（键是根评论 ID）。
	family map[uint64][]domain.Comment
	// softDeleted 记录被软删的 ID；softDeleteErr 让级联那一步失败。
	softDeleted   []uint64
	softDeleteErr error

	// listAdmin 的预置结果与入参记录（管理端列表用例用）。
	adminRows    []domain.Comment
	adminHasMore bool
	adminErr     error
	adminFilter  domain.AdminFilter
	adminPage    domain.AdminPage
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		published:  map[uint64]domain.Comment{},
		byID:       map[uint64]domain.Comment{},
		findErrors: map[uint64]error{},
		family:     map[uint64][]domain.Comment{},
	}
}

func (r *fakeRepo) Create(_ context.Context, c *domain.Comment) error {
	// 只补空 ID：评论内容过滤（票 396）会在 Create 之前先生成 ID，好让命中记录
	// 能指向新评论；无条件覆盖会让那条链路在测试里看起来"ID 对不上"。
	if c.ID.IsZero() {
		c.ID = ctype.NewID(1)
	}
	c.CreatedAt = time.Now()
	r.created = append(r.created, c)
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id domain.CommentID) (domain.Comment, error) {
	if err, ok := r.findErrors[id.Uint64()]; ok {
		return domain.Comment{}, err
	}
	got, ok := r.byID[id.Uint64()]
	if !ok {
		return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
	}
	return got, nil
}

func (r *fakeRepo) FindPublished(_ context.Context, id domain.CommentID, _ uint64) (domain.Comment, error) {
	got, ok := r.published[id.Uint64()]
	if !ok {
		return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
	}
	return got, nil
}

func (r *fakeRepo) FindPublishedByID(context.Context, domain.CommentID) (domain.Comment, error) {
	return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
}
func (r *fakeRepo) FindByIDs(context.Context, []domain.CommentID) ([]domain.Comment, error) {
	return nil, nil
}
func (r *fakeRepo) ListRoot(context.Context, uint64, domain.Page) ([]domain.Comment, bool, error) {
	return nil, false, nil
}
func (r *fakeRepo) ListReplies(context.Context, uint64, uint64, domain.Page) ([]domain.Comment, bool, error) {
	return nil, false, nil
}
func (r *fakeRepo) ListFamily(_ context.Context, rootID domain.CommentID) ([]domain.Comment, error) {
	return r.family[rootID.Uint64()], nil
}

// ListAdmin 返回预置的管理端列表行，并把入参记下来供用例断言筛选/分页。
func (r *fakeRepo) ListAdmin(_ context.Context, filter domain.AdminFilter, page domain.AdminPage) ([]domain.Comment, bool, error) {
	r.adminFilter = filter
	r.adminPage = page
	if r.adminErr != nil {
		return nil, false, r.adminErr
	}
	return r.adminRows, r.adminHasMore, nil
}

func (r *fakeRepo) SoftDeleteMany(_ context.Context, ids []domain.CommentID) error {
	if r.softDeleteErr != nil {
		return r.softDeleteErr
	}
	for _, id := range ids {
		r.softDeleted = append(r.softDeleted, id.Uint64())
	}
	return nil
}

type fakeDiggs struct{}

func (fakeDiggs) Exists(context.Context, uint64, uint64) (bool, error) { return false, nil }
func (fakeDiggs) Delete(context.Context, uint64, uint64) (bool, error) { return false, nil }
func (fakeDiggs) Create(context.Context, uint64, uint64) (bool, error) { return false, nil }
func (fakeDiggs) DeleteByComments(context.Context, []uint64) error     { return nil }
func (fakeDiggs) DiggedSet(context.Context, uint64, []uint64) (map[uint64]bool, error) {
	return nil, nil
}

type fakeCounters struct{}

func (fakeCounters) Batch(context.Context, []uint64) (map[uint64]int, map[uint64]int) {
	return nil, nil
}

type fakeRelations struct{}

func (fakeRelations) Batch(_ context.Context, _ uint64, ids []uint64) (map[uint64]int8, error) {
	out := map[uint64]int8{}
	for _, id := range ids {
		out[id] = 0
	}
	return out, nil
}

type fakeUsers struct{}

func (fakeUsers) Displays(_ context.Context, ids []uint64) (map[uint64]domain.Display, error) {
	out := map[uint64]domain.Display{}
	for _, id := range ids {
		out[id] = domain.Display{Nickname: "路人", Avatar: ""}
	}
	return out, nil
}

// fakeArticleCalls 记录 TitlesByIDs 的调用（用指针字段，fake 本身仍按值传递）。
type fakeArticleCalls struct {
	titles    int
	titlesIDs []ctype.ID
}

type fakeArticles struct {
	title    string
	comments bool
	// titles 是 TitlesByIDs 能返回的文章标题；titlesErr 让那一步失败。
	titles    map[uint64]string
	titlesErr error
	calls     *fakeArticleCalls
}

func (a fakeArticles) ByID(_ context.Context, id uint64) (domain.Article, error) {
	return domain.Article{ID: ctype.NewID(id), Title: a.title, CommentsToggle: a.comments}, nil
}

// TitlesByIDs 批量取标题：只返回预置里有的 ID（找不到的不进 map）。
//
// 用值接收者而不是指针：既有用例按值传 fakeArticles{}（见 create_test.go），
// 指针接收者会让那些调用点编译不过。调用记录挂在 calls 指针上。
func (a fakeArticles) TitlesByIDs(_ context.Context, ids []ctype.ID) (map[ctype.ID]string, error) {
	if a.calls != nil {
		a.calls.titles++
		a.calls.titlesIDs = ids
	}
	if a.titlesErr != nil {
		return nil, a.titlesErr
	}
	out := make(map[ctype.ID]string, len(ids))
	for _, id := range ids {
		if title, ok := a.titles[id.Uint64()]; ok {
			out[id] = title
		}
	}
	return out, nil
}

type fakeCache struct {
	added int
	// lastArticleDelta 是最近一次文章评论数增量的值（删评论时用来断言"只扣
	// 已发布的那几条"，负数是扣减）。
	lastArticleDelta int
}

func (c *fakeCache) AddArticleComments(_ context.Context, _ uint64, delta int) error {
	c.added++
	c.lastArticleDelta = delta
	return nil
}
func (c *fakeCache) AddReply(context.Context, uint64, int) error { return nil }
func (c *fakeCache) DelReply(context.Context, uint64) error      { return nil }
func (c *fakeCache) AddDigg(context.Context, uint64, int) error  { return nil }
func (c *fakeCache) DelDigg(context.Context, uint64) error       { return nil }

type fakeNotifier struct {
	count int
	// lastContent 是最近一次通知里带的评论正文：过滤必须发生在通知之前
	//（票 396：通知里也是替换后的文本）。
	lastContent string
}

func (n *fakeNotifier) CommentCreated(_ context.Context, in domain.CommentNotice) error {
	n.count++
	n.lastContent = in.Content
	return nil
}
func (n *fakeNotifier) ReplyCreated(context.Context, domain.ReplyNotice) error { return nil }
func (n *fakeNotifier) DiggCreated(context.Context, domain.DiggNotice) error   { return nil }

type fakeIDs struct{}

func (fakeIDs) NextCommentID() domain.CommentID { return ctype.NewID(1) }

type fakeLogger struct{ errors int }

func (l *fakeLogger) Errorf(string, ...any) { l.errors++ }

// fakeAccounts 是账号处置端口（禁言 / 封禁）的替身。
//
// 它把"管理员传了什么给 user 模块"记下来：评论模块的职责到"调用正确的人、
// 正确的档位"为止，账号到底怎么落库是 user 模块自己的契约。
type fakeAccounts struct {
	muteCalls []fakeMuteCall
	banCalls  []fakeBanCall
	muteUntil time.Time
	muteErr   error
	banErr    error
}

func newFakeAccounts() *fakeAccounts {
	// 默认返回一个远期到期时间（与"永久"同形状）：用例只需要它是一个未来时刻。
	return &fakeAccounts{muteUntil: time.Now().Add(30 * 24 * time.Hour)}
}

func (a *fakeAccounts) Mute(_ context.Context, actor, target ctype.ID, days int) (time.Time, error) {
	a.muteCalls = append(a.muteCalls, fakeMuteCall{actor: actor, target: target, days: days})
	if a.muteErr != nil {
		return time.Time{}, a.muteErr
	}
	return a.muteUntil, nil
}

func (a *fakeAccounts) Ban(_ context.Context, actor, target ctype.ID) error {
	a.banCalls = append(a.banCalls, fakeBanCall{actor: actor, target: target})
	return a.banErr
}

// fixture 是一套完整装配的评论用例服务（未接入内容过滤）。
type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	cache    *fakeCache
	notifier *fakeNotifier
	logger   *fakeLogger
	accounts *fakeAccounts
	articles *fakeArticles
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo:     newFakeRepo(),
		cache:    &fakeCache{},
		notifier: &fakeNotifier{},
		logger:   &fakeLogger{},
		accounts: newFakeAccounts(),
		articles: &fakeArticles{
			title: "Golang 并发模型", comments: true,
			titles: map[uint64]string{100: "Golang 并发模型"},
			calls:  &fakeArticleCalls{},
		},
	}
	f.svc = app.New(
		f.repo, fakeDiggs{}, fakeCounters{}, fakeRelations{}, fakeUsers{},
		f.articles,
		f.cache, f.notifier, fakeIDs{}, f.logger,
	).WithAccountDisposer(f.accounts)
	return f
}

func (f *fixture) create(t *testing.T, content string) (*app.CreateResult, error) {
	t.Helper()
	return f.svc.Create(context.Background(), domain.Viewer{ID: ctype.NewID(7)}, app.CreateInput{
		Content:   content,
		ArticleID: ctype.NewID(100),
	})
}

// adminViewer 是管理端测试用的操作者。
var adminViewer = domain.Viewer{ID: ctype.NewID(1), Admin: true}

// 编译期确认替身满足端口形状（新增端口方法而忘了补 fake 会在这里炸，而不是
// 等到某条用例跑到一半才 nil panic）。
var (
	_ domain.Repository      = (*fakeRepo)(nil)
	_ domain.AccountDisposer = (*fakeAccounts)(nil)
	_ domain.ArticleInfo     = fakeArticles{}
	_ domain.ArticleInfo     = (*fakeArticles)(nil)
)
