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

// fakeRepo 只记住"落了什么"，不模拟 SQL。
type fakeRepo struct {
	created []*domain.Comment
	// published 是回复目标查找用的预置评论（按 ID）。
	published map[uint64]domain.Comment
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{published: map[uint64]domain.Comment{}}
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

func (r *fakeRepo) FindByID(context.Context, domain.CommentID) (domain.Comment, error) {
	return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
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
func (r *fakeRepo) ListFamily(context.Context, domain.CommentID) ([]domain.Comment, error) {
	return nil, nil
}
func (r *fakeRepo) SoftDeleteMany(context.Context, []domain.CommentID) error { return nil }

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

type fakeArticles struct {
	title    string
	comments bool
}

func (a fakeArticles) ByID(_ context.Context, id uint64) (domain.Article, error) {
	return domain.Article{ID: ctype.NewID(id), Title: a.title, CommentsToggle: a.comments}, nil
}

type fakeCache struct{ added int }

func (c *fakeCache) AddArticleComments(context.Context, uint64, int) error { c.added++; return nil }
func (c *fakeCache) AddReply(context.Context, uint64, int) error           { return nil }
func (c *fakeCache) DelReply(context.Context, uint64) error                { return nil }
func (c *fakeCache) AddDigg(context.Context, uint64, int) error            { return nil }
func (c *fakeCache) DelDigg(context.Context, uint64) error                 { return nil }

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

// fixture 是一套完整装配的评论用例服务（未接入内容过滤）。
type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	cache    *fakeCache
	notifier *fakeNotifier
	logger   *fakeLogger
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo:     newFakeRepo(),
		cache:    &fakeCache{},
		notifier: &fakeNotifier{},
		logger:   &fakeLogger{},
	}
	f.svc = app.New(
		f.repo, fakeDiggs{}, fakeCounters{}, fakeRelations{}, fakeUsers{},
		fakeArticles{title: "Golang 并发模型", comments: true},
		f.cache, f.notifier, fakeIDs{}, f.logger,
	)
	return f
}

func (f *fixture) create(t *testing.T, content string) (*app.CreateResult, error) {
	t.Helper()
	return f.svc.Create(context.Background(), domain.Viewer{ID: ctype.NewID(7)}, app.CreateInput{
		Content:   content,
		ArticleID: ctype.NewID(100),
	})
}
