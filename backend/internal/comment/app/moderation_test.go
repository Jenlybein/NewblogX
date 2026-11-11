package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 评论审核（票 359）的用例。
//
// 这一层钉的是**判定顺序与失败方向**，不是模型本身：
//   - 阻断词命中 → 拒绝，且**不入库**（不留痕、不计数、不发通知）；
//   - 观察词命中 → 即使开了免审核也留待人工；
//   - AI 判 clean → 直发；suspect / violation / 报错 / 超时 → 留待人工；
//   - 没接入审核时行为与改造前一致（不能因为没配就拒绝所有评论）。

// ── 最小 fake ────────────────────────────────────────────────

type fakeRepo struct {
	created  []*domain.Comment
	statuses map[uint64]domain.Status
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{statuses: map[uint64]domain.Status{}}
}

func (r *fakeRepo) Create(_ context.Context, c *domain.Comment) error {
	c.ID = ctype.NewID(1)
	c.CreatedAt = time.Now()
	r.created = append(r.created, c)
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id domain.CommentID, status domain.Status) error {
	r.statuses[id.Uint64()] = status
	return nil
}

func (r *fakeRepo) FindByID(context.Context, domain.CommentID) (domain.Comment, error) {
	return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
}
func (r *fakeRepo) FindPublished(context.Context, domain.CommentID, uint64) (domain.Comment, error) {
	return domain.Comment{}, apperr.New(apperr.CodeCommentNotFound)
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
func (r *fakeRepo) ListManaged(context.Context, domain.ManagedQuery) ([]domain.Comment, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepo) SoftDeleteMany(context.Context, []domain.CommentID) error { return nil }

type fakeDiggs struct{}

func (fakeDiggs) Exists(context.Context, uint64, uint64) (bool, error)        { return false, nil }
func (fakeDiggs) Delete(context.Context, uint64, uint64) (bool, error)        { return false, nil }
func (fakeDiggs) Create(context.Context, uint64, uint64) (bool, error)        { return false, nil }
func (fakeDiggs) DeleteByComments(context.Context, []uint64) error            { return nil }
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

func (a fakeArticles) BaseMap(context.Context, []uint64) (map[uint64]domain.Article, error) {
	return nil, nil
}
func (a fakeArticles) IDsByAuthor(context.Context, ctype.ID, ctype.ID) ([]ctype.ID, error) {
	return nil, nil
}

type fakeCache struct{ added int }

func (c *fakeCache) AddArticleComments(context.Context, uint64, int) error { c.added++; return nil }
func (c *fakeCache) AddReply(context.Context, uint64, int) error           { return nil }
func (c *fakeCache) DelReply(context.Context, uint64) error               { return nil }
func (c *fakeCache) AddDigg(context.Context, uint64, int) error           { return nil }
func (c *fakeCache) DelDigg(context.Context, uint64) error                { return nil }

type fakeNotifier struct{ count int }

func (n *fakeNotifier) CommentCreated(context.Context, domain.CommentNotice) error { n.count++; return nil }
func (n *fakeNotifier) ReplyCreated(context.Context, domain.ReplyNotice) error     { return nil }
func (n *fakeNotifier) DiggCreated(context.Context, domain.DiggNotice) error       { return nil }

type fakeIDs struct{}

func (fakeIDs) NextCommentID() domain.CommentID { return ctype.NewID(1) }

type fakeLogger struct{ errors int }

func (l *fakeLogger) Errorf(string, ...any) { l.errors++ }

// fakeChecker 是 AI 判定端口的替身：结论与错误由用例指定。
type fakeChecker struct {
	result *app.ComplianceResult
	err    error
	// delay 用来模拟超时。
	delay time.Duration
	calls int
	// gotMaxTokens 记录调用方传进来的输出上限，用来验证"钉在 1000"。
	gotMaxTokens int
}

func (c *fakeChecker) CheckComment(ctx context.Context, in app.ComplianceInput) (*app.ComplianceResult, error) {
	c.calls++
	c.gotMaxTokens = in.MaxTokens
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return c.result, c.err
}

// ── fixture ──────────────────────────────────────────────────

type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	cache    *fakeCache
	notifier *fakeNotifier
	checker  *fakeChecker
	logger   *fakeLogger
}

func newFixture(t *testing.T, policy *app.ModerationPolicy, checker app.ComplianceChecker, skipExam bool) *fixture {
	t.Helper()
	f := &fixture{
		repo:     newFakeRepo(),
		cache:    &fakeCache{},
		notifier: &fakeNotifier{},
		logger:   &fakeLogger{},
	}
	if checker == nil {
		f.checker = &fakeChecker{result: &app.ComplianceResult{Verdict: app.VerdictClean}}
	} else {
		f.checker = checker.(*fakeChecker)
	}

	svc := app.New(
		f.repo, fakeDiggs{}, fakeCounters{}, fakeRelations{}, fakeUsers{},
		fakeArticles{title: "Golang 并发模型", comments: true},
		f.cache, f.notifier, fakeIDs{}, f.logger,
		func() bool { return skipExam },
	)
	if policy != nil {
		p := *policy
		svc = svc.WithModeration(func() app.ModerationPolicy { return p }, f.checker)
	}
	f.svc = svc
	return f
}

func (f *fixture) create(t *testing.T, content string) (*app.CreateResult, error) {
	t.Helper()
	return f.svc.Create(context.Background(), domain.Viewer{ID: ctype.NewID(7)}, app.CreateInput{
		Content:   content,
		ArticleID: ctype.NewID(100),
	})
}

// ── 用例 ─────────────────────────────────────────────────────

func TestCreateRejectsBlockWordBeforeInsert(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		BlockWords: []string{"代开发票"},
	}, nil, true)

	_, err := f.create(t, "需要代开发票的私信我")
	if err == nil {
		t.Fatal("命中阻断词必须拒绝")
	}
	if !apperr.IsCode(err, apperr.CodeCommentBlocked) {
		t.Fatalf("错误码应为 comment.blocked，实际 %v", err)
	}
	// 被拒的评论不该留下任何痕迹。
	if len(f.repo.created) != 0 {
		t.Fatalf("被拒的评论不该入库，实际入库 %d 条", len(f.repo.created))
	}
	if f.cache.added != 0 || f.notifier.count != 0 {
		t.Fatal("被拒的评论不该计数或发通知")
	}
	// 词表命中时不必花一次模型调用。
	if f.checker.calls != 0 {
		t.Fatalf("阻断词命中时不该调用 AI，实际调用 %d 次", f.checker.calls)
	}
}

func TestCreateHoldsWatchWordEvenWhenSkipExamining(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		WatchWords: []string{"加微信"},
	}, nil, true)

	result, err := f.create(t, "有问题加微信聊")
	if err != nil {
		t.Fatalf("观察词不该拒绝: %v", err)
	}
	if result.Status != int8(domain.StatusExamining) {
		t.Fatalf("观察词命中应留待人工，实际状态 %d", result.Status)
	}
	if _, ok := f.repo.statuses[result.ID.Uint64()]; ok {
		t.Fatal("留待人工时不该改状态为已发布")
	}
	// 已经要人工看了，不该再花钱判定。
	if f.checker.calls != 0 {
		t.Fatalf("留待人工的评论不该调用 AI，实际 %d 次", f.checker.calls)
	}
}

func TestCreatePublishesWhenAISaysClean(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		AIEnabled: true, AIMaxTokens: 1000, AITimeout: 3 * time.Second,
	}, &fakeChecker{result: &app.ComplianceResult{Verdict: app.VerdictClean}}, true)

	result, err := f.create(t, "写得很清楚，学到了")
	if err != nil {
		t.Fatalf("干净评论应发布: %v", err)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("AI 判 clean 应直发，实际状态 %d", result.Status)
	}
	if f.checker.calls != 1 {
		t.Fatalf("应调用一次 AI，实际 %d", f.checker.calls)
	}
	if f.checker.gotMaxTokens != 1000 {
		t.Fatalf("输出上限应钉在 1000，实际 %d", f.checker.gotMaxTokens)
	}
}

func TestCreateHoldsWhenAIFindsViolation(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		AIEnabled: true, AIMaxTokens: 1000, AITimeout: 3 * time.Second,
	}, &fakeChecker{result: &app.ComplianceResult{
		Verdict: app.VerdictViolation, Categories: []string{"违法交易"},
	}}, true)

	result, err := f.create(t, "看起来正常的一句话")
	if err != nil {
		t.Fatalf("违规不该抛错（对用户表现为审核中）: %v", err)
	}
	if result.Status != int8(domain.StatusExamining) {
		t.Fatalf("AI 判违规应留待人工，实际状态 %d", result.Status)
	}
}

func TestCreateHoldsWhenAIErrors(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		AIEnabled: true, AIMaxTokens: 1000, AITimeout: 3 * time.Second,
	}, &fakeChecker{err: errors.New("上游 502")}, true)

	result, err := f.create(t, "一句正常评论")
	if err != nil {
		t.Fatalf("AI 不可用不该让发评论失败: %v", err)
	}
	if result.Status != int8(domain.StatusExamining) {
		t.Fatalf("判定失败必须按「未判定」处理（留待人工），实际状态 %d", result.Status)
	}
}

func TestCreateHoldsWhenAITimesOut(t *testing.T) {
	f := newFixture(t, &app.ModerationPolicy{
		AIEnabled: true, AIMaxTokens: 1000, AITimeout: 50 * time.Millisecond,
	}, &fakeChecker{result: &app.ComplianceResult{Verdict: app.VerdictClean}, delay: 2 * time.Second}, true)

	start := time.Now()
	result, err := f.create(t, "一句正常评论")
	if err != nil {
		t.Fatalf("超时不该让发评论失败: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("超时必须有上限，实际耗时 %v", elapsed)
	}
	if result.Status != int8(domain.StatusExamining) {
		t.Fatalf("超时应留待人工，实际状态 %d", result.Status)
	}
}

func TestCreateWithoutModerationKeepsOldBehaviour(t *testing.T) {
	// 没接入审核（policy=nil）：行为必须与改造前完全一致 —— 开免审核就直发。
	f := newFixture(t, nil, nil, true)

	result, err := f.create(t, "随便一句话")
	if err != nil {
		t.Fatalf("未接入审核时不该拒绝: %v", err)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("未接入审核时应按 skip_examining 直发，实际状态 %d", result.Status)
	}
	if f.checker.calls != 0 {
		t.Fatal("未接入审核时不该调用 AI")
	}
}

func TestCreateHoldsWhenSkipExaminingOff(t *testing.T) {
	// 免审核关闭：一切评论都进人工，AI 也不该被调用（已经要人看了）。
	f := newFixture(t, &app.ModerationPolicy{
		AIEnabled: true, AIMaxTokens: 1000, AITimeout: 3 * time.Second,
	}, nil, false)

	result, err := f.create(t, "一句正常评论")
	if err != nil {
		t.Fatalf("不该失败: %v", err)
	}
	if result.Status != int8(domain.StatusExamining) {
		t.Fatalf("免审核关闭时应留待人工，实际状态 %d", result.Status)
	}
	if f.checker.calls != 0 {
		t.Fatalf("已经要人工看的评论不该调用 AI，实际 %d 次", f.checker.calls)
	}
}

func TestZeroPolicyFallsBackToSafeDefaults(t *testing.T) {
	// 配置漏填时兜底：输出上限 1000。零值等于"不限制输出"，那是危险的默认
	// （模型可能长篇输出，白付钱也拖慢同步路径）。
	f := newFixture(t, &app.ModerationPolicy{AIEnabled: true}, nil, true)

	if _, err := f.create(t, "一句正常评论"); err != nil {
		t.Fatalf("不该失败: %v", err)
	}
	if f.checker.gotMaxTokens != 1000 {
		t.Fatalf("输出上限漏填时应兜底为 1000，实际 %d", f.checker.gotMaxTokens)
	}
}

func TestZeroTimeoutDoesNotHang(t *testing.T) {
	// 超时漏填时兜底 8 秒：没有兜底的话 context.WithTimeout(0) 会立刻取消，
	// 表现为"所有评论都进人工"，同样是个静默故障。
	f := newFixture(t, &app.ModerationPolicy{AIEnabled: true, AIMaxTokens: 1000}, nil, true)

	result, err := f.create(t, "一句正常评论")
	if err != nil {
		t.Fatalf("不该失败: %v", err)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("兜底超时应让判定正常完成并直发，实际状态 %d", result.Status)
	}
}
