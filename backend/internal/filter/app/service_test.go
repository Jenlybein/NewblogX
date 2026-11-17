package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// 用例层要钉的是三件事（ADR-0032 决策 11）：
//   - 过滤结果从哪来、命中记录写成什么形状；
//   - 记录写失败不阻断调用方（只记日志）；
//   - 引擎没装配时退化成"原样返回"，不 panic、不拒发。

// fakeRepo 是命中记录的仓储替身：记录调用参数，行为由用例指定。
type fakeRepo struct {
	created   []domain.Hit
	createErr error

	rows    []domain.Hit
	total   int64
	listErr error

	top      []domain.TopUser
	topErr   error
	gotQuery domain.Query
	gotSince time.Time
	gotLimit int
}

func (r *fakeRepo) Create(_ context.Context, hit *domain.Hit) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, *hit)
	return nil
}

func (r *fakeRepo) List(_ context.Context, q domain.Query) ([]domain.Hit, int64, error) {
	r.gotQuery = q
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	return r.rows, r.total, nil
}

func (r *fakeRepo) TopUsers(_ context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
	r.gotSince = since
	r.gotLimit = limit
	if r.topErr != nil {
		return nil, r.topErr
	}
	return r.top, nil
}

// fakeLogger 只数错误条数：失败方向由"有没有记一条 error"证明。
type fakeLogger struct{ errors int }

func (l *fakeLogger) Errorf(string, ...any) { l.errors++ }

// newEngine 构造一个只含"冰毒"（拦截 / 毒品）的引擎。
func newEngine(t *testing.T) *contentfilter.Engine {
	t.Helper()
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		nil,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	return engine
}

func TestFilterMasksTextAndRecordsPendingHit(t *testing.T) {
	repo := &fakeRepo{}
	logger := &fakeLogger{}
	svc := app.New(newEngine(t), repo, logger)

	out, err := svc.Filter(context.Background(), app.FilterInput{
		Surface:   domain.SurfaceComment,
		UserID:    ctype.NewID(7),
		ContentID: ctype.NewID(9001),
		Text:      "我这里有冰毒",
	})
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if out.Masked != "我这里有**" {
		t.Fatalf("命中词应替换成等长 *，实际 %q", out.Masked)
	}
	// "处"是一次出现而不是一个字符：一个两字词命中一次 = 1 处。
	if out.FilteredCount != 1 {
		t.Fatalf("替换处数应为 1，实际 %d", out.FilteredCount)
	}
	if len(repo.created) != 1 {
		t.Fatalf("应写一条命中记录，实际 %d 条", len(repo.created))
	}
	hit := repo.created[0]
	if hit.Surface != domain.SurfaceComment {
		t.Fatalf("文本面应为 comment，实际 %q", hit.Surface)
	}
	if hit.ContentID != ctype.NewID(9001) || hit.UserID != ctype.NewID(7) {
		t.Fatalf("内容 ID / 用户 ID 没接上: %+v", hit)
	}
	if hit.Status != domain.StatusPending {
		t.Fatalf("新记录应是待处理，实际 %q", hit.Status)
	}
	if len(hit.Words) != 1 || hit.Words[0].Word != "冰毒" {
		t.Fatalf("命中明细不对: %+v", hit.Words)
	}
	if hit.Words[0].Tier != contentfilter.TierBlock || hit.Words[0].Category != contentfilter.CategoryDrugs {
		t.Fatalf("档位 / 类目丢了: %+v", hit.Words[0])
	}
	if len(hit.Words[0].Snippets) != 1 {
		t.Fatalf("上下文片段应有 1 条，实际 %d", len(hit.Words[0].Snippets))
	}
	snippet := hit.Words[0].Snippets[0]
	if snippet.Text != "我这里有冰毒" || snippet.Start != 4 || snippet.End != 6 {
		t.Fatalf("上下文片段下标不对: %+v", snippet)
	}
	if hit.HitCount != 1 {
		t.Fatalf("命中处数应为 1，实际 %d", hit.HitCount)
	}
	if logger.errors != 0 {
		t.Fatalf("成功路径不该写 error 日志，实际 %d 条", logger.errors)
	}
}

func TestFilterWithoutHitsDoesNotRecord(t *testing.T) {
	repo := &fakeRepo{}
	svc := app.New(newEngine(t), repo, &fakeLogger{})

	out, err := svc.Filter(context.Background(), app.FilterInput{
		Surface: domain.SurfaceComment,
		Text:    "一句正常的话",
	})
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if out.Masked != "一句正常的话" || out.FilteredCount != 0 {
		t.Fatalf("没有命中时应原样返回，实际 %q / %d", out.Masked, out.FilteredCount)
	}
	if len(repo.created) != 0 {
		t.Fatalf("没有命中不该写记录，实际 %d 条", len(repo.created))
	}
}

// TestFilterRecordFailureDoesNotBlockCaller 是 ADR-0032 决策 11 的核心用例：
// 审计写不进去，替换后的文本照样返回、照样能用。
func TestFilterRecordFailureDoesNotBlockCaller(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("表锁超时")}
	logger := &fakeLogger{}
	svc := app.New(newEngine(t), repo, logger)

	out, err := svc.Filter(context.Background(), app.FilterInput{
		Surface:   domain.SurfaceComment,
		UserID:    ctype.NewID(7),
		ContentID: ctype.NewID(9002),
		Text:      "冰毒",
	})
	if err != nil {
		t.Fatalf("记录写失败不该向调用方返回错误: %v", err)
	}
	if out.Masked != "**" || out.FilteredCount != 1 {
		t.Fatalf("记录失败不影响替换结果，实际 %q / %d", out.Masked, out.FilteredCount)
	}
	if logger.errors != 1 {
		t.Fatalf("记录写失败应记一条 error 日志，实际 %d 条", logger.errors)
	}
}

// TestFilterWhitelistHitRecordedButNotCounted 对应验收条：
// 白名单命中不替换，但仍出现在风险页。
func TestFilterWhitelistHitRecordedButNotCounted(t *testing.T) {
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		[]string{"冰毒科普"},
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	repo := &fakeRepo{}
	svc := app.New(engine, repo, &fakeLogger{})

	out, err := svc.Filter(context.Background(), app.FilterInput{
		Surface: domain.SurfaceComment,
		Text:    "冰毒科普",
	})
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if out.Masked != "冰毒科普" || out.FilteredCount != 0 {
		t.Fatalf("白名单命中不该替换，实际 %q / %d", out.Masked, out.FilteredCount)
	}
	if len(repo.created) != 1 {
		t.Fatalf("白名单命中仍要进风险页，实际记录 %d 条", len(repo.created))
	}
}

// TestFilterWithoutEngineReturnsText 是装配遗漏时的退化：不让整站不可用。
func TestFilterWithoutEngineReturnsText(t *testing.T) {
	repo := &fakeRepo{}
	svc := app.New(nil, repo, &fakeLogger{})

	out, err := svc.Filter(context.Background(), app.FilterInput{Text: "冰毒"})
	if err != nil {
		t.Fatalf("引擎缺失不该报错: %v", err)
	}
	if out.Masked != "冰毒" {
		t.Fatalf("引擎缺失应原样返回，实际 %q", out.Masked)
	}
	if len(repo.created) != 0 {
		t.Fatal("引擎缺失时不该凭空写记录")
	}
}

// TestRecordSwallowsWriteError 钉住 Record 自己的失败方向（存量清洗也会用它）。
func TestRecordSwallowsWriteError(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("连接池耗尽")}
	logger := &fakeLogger{}
	svc := app.New(newEngine(t), repo, logger)

	svc.Record(context.Background(), domain.Hit{Surface: domain.SurfaceComment})

	if logger.errors != 1 {
		t.Fatalf("写失败应记一条 error 日志，实际 %d 条", logger.errors)
	}
}

func TestListPassesFiltersAndReturnsTopUsers(t *testing.T) {
	repo := &fakeRepo{
		rows:  []domain.Hit{{Surface: domain.SurfaceComment, Status: domain.StatusPending}},
		total: 1,
		top:   []domain.TopUser{{UserID: ctype.NewID(7), Nickname: "路人", HitCount: 12}},
	}
	svc := app.New(newEngine(t), repo, &fakeLogger{})

	result, err := svc.List(context.Background(), app.ListInput{
		Surface: domain.SurfaceComment,
		Status:  domain.StatusPending,
		UserID:  ctype.NewID(7),
		Page:    2,
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if repo.gotQuery.Surface != domain.SurfaceComment || repo.gotQuery.Status != domain.StatusPending {
		t.Fatalf("筛选条件没传下去: %+v", repo.gotQuery)
	}
	if repo.gotQuery.UserID != ctype.NewID(7) {
		t.Fatalf("用户筛选没传下去: %+v", repo.gotQuery)
	}
	if repo.gotQuery.Page.Page != 2 || repo.gotQuery.Page.Limit != 20 {
		t.Fatalf("分页没传下去: %+v", repo.gotQuery.Page)
	}
	if repo.gotLimit != app.TopUserLimit {
		t.Fatalf("Top 条数应为 %d，实际 %d", app.TopUserLimit, repo.gotLimit)
	}
	if since := time.Since(repo.gotSince); since < 89*24*time.Hour || since > 91*24*time.Hour {
		t.Fatalf("Top 统计窗口应约为 %d 天，实际 %v", domain.TopWindowDays, since)
	}
	if len(result.Items) != 1 || result.Total != 1 || len(result.TopUsers) != 1 {
		t.Fatalf("读面结果不对: %+v", result)
	}
}

// TestListToleratesTopUsersFailure：Top 是辅助视图，它失败不该让列表打不开。
func TestListToleratesTopUsersFailure(t *testing.T) {
	repo := &fakeRepo{
		rows:   []domain.Hit{{Surface: domain.SurfaceComment}},
		total:  1,
		topErr: errors.New("聚合超时"),
	}
	logger := &fakeLogger{}
	svc := app.New(newEngine(t), repo, logger)

	result, err := svc.List(context.Background(), app.ListInput{})
	if err != nil {
		t.Fatalf("Top 失败不该让列表失败: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("列表数据应照常返回，实际 %d 条", len(result.Items))
	}
	if len(result.TopUsers) != 0 {
		t.Fatalf("Top 失败时应返回空 Top，实际 %d 条", len(result.TopUsers))
	}
	if logger.errors != 1 {
		t.Fatalf("Top 失败应记一条 error 日志，实际 %d 条", logger.errors)
	}
}

// FindByID / MarkHandled 是处置能力（票 398）加进仓储端口的两个方法。
// 本文件的用例只关心"过滤与记录"，所以这里给出"查不到 / 不更新"的最简实现；
// 处置行为由 handle_test.go 的 handleRepo 覆盖。
func (r *fakeRepo) FindByID(_ context.Context, _ ctype.ID) (domain.Hit, error) {
	return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
}

func (r *fakeRepo) MarkHandled(_ context.Context, _ ctype.ID, _ domain.Handle) (bool, error) {
	return false, nil
}

// PurgeBefore 是保留期清理（ADR-0032 决策 10）加进仓储端口的方法。
// 本文件关心的是读面 / 处置，清理行为由 cron 侧用例覆盖，这里给"什么都不删"的最简实现。
func (r *fakeRepo) PurgeBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// WordStats 是词云统计（ADR-0032 决策 9）加进仓储端口的方法。
// 本文件不验证词云口径，给"空集"最简实现；口径由 adapter 与装配层用例覆盖。
func (r *fakeRepo) WordStats(_ context.Context, _ time.Time) ([]domain.WordStat, error) {
	return nil, nil
}
