package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
)

// fakeQualityGate 是 app.QualityGatePort 的内存实现（票 306 / ADR-0032）。
//
// 只模拟"端口返回什么"：给分、给问题、或报错。它不模拟模型行为——这里要验的
// 是"我们拿这个分数做了什么决定"，不是模型会不会打分。
type fakeQualityGate struct {
	score int
	// dimensions 是六个维度的分（ADR-0040 的木桶判定要用）。留空表示"这次评分
	// 没给维度分"，此时只按总分判。
	dimensions []app.QualityDimension
	issues     []app.QualityIssue
	err        error
	calls      int
	lastGate   app.QualityGateInput
}

func (f *fakeQualityGate) Evaluate(_ context.Context, in app.QualityGateInput) (app.QualityGateScore, error) {
	f.calls++
	f.lastGate = in
	if f.err != nil {
		return app.QualityGateScore{}, f.err
	}
	return app.QualityGateScore{TotalScore: f.score, Dimensions: f.dimensions, Issues: f.issues}, nil
}

// evenDimensions 返回六个都给同一分的维度（桶判定的"全过线"用例）。
func evenDimensions(score int) []app.QualityDimension {
	return []app.QualityDimension{
		{Name: "clarity", Score: score},
		{Name: "structure", Score: score},
		{Name: "completeness", Score: score},
		{Name: "readability", Score: score},
		{Name: "persuasiveness", Score: score},
		{Name: "language", Score: score},
	}
}

// gatePolicy 返回"闸门开着、及格线 50、灰带 2"（实际放行线 52）。
func gatePolicy() app.QualityGatePolicy {
	return app.QualityGatePolicy{
		Enabled:            true,
		PassScore:          50,
		GrayBandWidth:      2,
		BandPoorMax:        30,
		BandUnqualifiedMax: 59,
		BandNormalMax:      80,
		BandGoodMax:        89,
	}
}

// newGateService 组装一个带闸门端口与配额 fake 的服务。
func newGateService(t *testing.T, gate app.QualityGatePort) (*app.Service, *apptest.Repos, *apptest.SubmitQuota) {
	t.Helper()
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	quota := apptest.NewSubmitQuota(15)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), quota, nil, nil, gate)
	repos.FakeUsers().Seed(app.AuthorView{ID: author, Nickname: "作者甲", Username: "u100"})
	repos.FakeCategories().Seed()
	return svc, repos, quota
}

// submitCmd 返回一次"提交"命令（普通作者 + 站点跳过审核）。
func gateSubmitCmd(gate app.QualityGatePolicy) app.CreateArticleCmd {
	return app.CreateArticleCmd{
		Actor:         authorViewer(),
		Title:         "标题",
		Content:       "正文",
		TargetStatus:  domain.StatusExamining,
		Visibility:    domain.VisibilityVisible,
		SkipExamining: true, // 站点跳过审核 → 达标即发布
		Policy:        domain.OpenWritePolicy(),
		QualityGate:   gate,
	}
}

// TestGateDisabledKeepsSubmitUnchanged 钉住"闸门关着时提交路径与引入前完全一致"。
func TestGateDisabledKeepsSubmitUnchanged(t *testing.T) {
	gate := &fakeQualityGate{score: 10} // 即使只有 10 分也不该影响判定
	svc, _, _ := newGateService(t, gate)

	a, err := svc.Create(context.Background(), gateSubmitCmd(app.QualityGatePolicy{}))
	if err != nil {
		t.Fatalf("闸门关闭时不应报错: %v", err)
	}
	if a.Status() != domain.StatusPublished {
		t.Fatalf("闸门关闭时应照旧发布，实际 %v", a.Status())
	}
	if gate.calls != 0 {
		t.Fatalf("闸门关闭时不应调用评分，实际调用 %d 次", gate.calls)
	}
}

// TestGatePassesWhenScoreReachesPublishLine 钉住"分数够就继续走发布/审核"。
func TestGatePassesWhenScoreReachesPublishLine(t *testing.T) {
	gate := &fakeQualityGate{score: 52} // = 及格线 50 + 灰带 2
	svc, _, quota := newGateService(t, gate)

	a, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))
	if err != nil {
		t.Fatalf("分数达到放行线不应报错: %v", err)
	}
	if a.Status() != domain.StatusPublished {
		t.Fatalf("分数达标应按站点策略发布，实际 %v", a.Status())
	}
	if gate.calls != 1 {
		t.Fatalf("闸门开着时应评一次，实际 %d", gate.calls)
	}
	if quota.Used(author) != 1 {
		t.Fatalf("成功提交应占一次额度，实际 %d", quota.Used(author))
	}
}

// 票 302 新规则 + 闸门同时生效：达标也只是"已发布待可见"，
// 不能因为闸门通过就跳过异步模型判定。
func TestGatePassWithReviewStillPendingVisibility(t *testing.T) {
	gate := &fakeQualityGate{score: 80}
	svc, repos, _ := newGateService(t, gate)

	cmd := gateSubmitCmd(gatePolicy())
	cmd.SkipExamining = false

	a, err := svc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatalf("达标提交不应报错: %v", err)
	}
	if a.Status() != domain.StatusPublished || a.Visibility() != domain.VisibilityPendingReview {
		t.Fatalf("达标且非免审核应落 Published + pending_review，实际 %s / %s", a.Status(), a.Visibility())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticlePublished {
		t.Fatalf("应发 published 事件触发异步判定，实际 %v", names)
	}
}

// TestGateRejectsBelowLineAsDraft 钉住"不达标落草稿并带上分数/差额/问题"。
func TestGateRejectsBelowLineAsDraft(t *testing.T) {
	gate := &fakeQualityGate{
		score:  40,
		issues: []app.QualityIssue{{Reason: "结构松散", Suggestion: "按步骤重排"}},
	}
	svc, repos, _ := newGateService(t, gate)

	_, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))

	var details *app.QualityRejectionDetails
	if !errors.As(err, &details) {
		t.Fatalf("应返回带数据的质量拒绝，实际 %v", err)
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeArticleQualityRejected {
		t.Fatalf("错误码应为 quality_rejected，实际 %s ok=%v", code, ok)
	}
	if details.Score != 40 || details.RequiredScore != 52 || details.Gap != 12 {
		t.Fatalf("分数/放行线/差额不对: score=%d required=%d gap=%d",
			details.Score, details.RequiredScore, details.Gap)
	}
	if len(details.Issues) != 1 {
		t.Fatalf("问题列表应原样带给作者，实际 %d 条", len(details.Issues))
	}

	// 文章确实落库为草稿：作者能继续改、能立刻重投。
	a, err := repos.FakeArticles().Find(context.Background(), details.ArticleID)
	if err != nil || a == nil {
		t.Fatalf("被打回的文章应已落库: %v", err)
	}
	if a.Status() != domain.StatusDraft {
		t.Fatalf("被打回的文章应为草稿，实际 %v", a.Status())
	}
}

// TestGateGrayBandDoesNotPass 钉住灰带：及格线上方 width 分内仍不放行。
func TestGateGrayBandDoesNotPass(t *testing.T) {
	gate := &fakeQualityGate{score: 51} // 高于及格线 50，但仍在 50–52 的灰带内
	svc, _, _ := newGateService(t, gate)

	_, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))
	var details *app.QualityRejectionDetails
	if !errors.As(err, &details) {
		t.Fatalf("灰带内应不放行，实际 %v", err)
	}
	if details.Gap != 1 {
		t.Fatalf("灰带内的差额应为 1 分，实际 %d", details.Gap)
	}
}

// TestGateBucketRejectsWhenSingleDimensionBelowPassScore 钉住木桶判定（ADR-0040）：
// 总分过线，但某一项没过及格线，仍然不放行。
//
// 这条正是线上那次"两行祝福语拿到 51 分并被发布"的结构性修复：加权总分会让
// 长板补短板，所以"六项都过线"必须单独成立。作者侧必须被点名是哪一项不够，
// 否则他看到"还差 0 分"只会去凑字数。
func TestGateBucketRejectsWhenSingleDimensionBelowPassScore(t *testing.T) {
	gate := &fakeQualityGate{
		score: 78, // 远高于放行线 52
		dimensions: []app.QualityDimension{
			{Name: "clarity", Score: 82},
			{Name: "structure", Score: 70},
			{Name: "completeness", Score: 41}, // 只有这一项没过 50 分的及格线
			{Name: "readability", Score: 76},
			{Name: "persuasiveness", Score: 62},
			{Name: "language", Score: 80},
		},
	}
	svc, repos, _ := newGateService(t, gate)

	_, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))

	var details *app.QualityRejectionDetails
	if !errors.As(err, &details) {
		t.Fatalf("单项未过及格线应不放行，实际 %v", err)
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeArticleQualityRejected {
		t.Fatalf("错误码应为 quality_rejected，实际 %s ok=%v", code, ok)
	}
	if details.Score != 78 || details.RequiredScore != 52 {
		t.Fatalf("分数/放行线不对: score=%d required=%d", details.Score, details.RequiredScore)
	}
	// 总分够线时差额必须是 0，不能给负数让作者误解成"还差 -26 分"。
	if details.Gap != 0 {
		t.Fatalf("总分过线时差额应为 0，实际 %d", details.Gap)
	}
	if details.MinDimensionScore != 50 {
		t.Fatalf("维度下限应等于及格线 50，实际 %d", details.MinDimensionScore)
	}
	if len(details.WeakDimensions) != 1 || details.WeakDimensions[0].Name != "completeness" {
		t.Fatalf("应只点名 completeness 一项，实际 %v", details.WeakDimensions)
	}
	if details.WeakDimensions[0].Score != 41 {
		t.Fatalf("弱项分数应原样带回，实际 %d", details.WeakDimensions[0].Score)
	}

	// 落点与总分不足时一致：草稿，可改可重投。
	a, err := repos.FakeArticles().Find(context.Background(), details.ArticleID)
	if err != nil || a == nil {
		t.Fatalf("被打回的文章应已落库: %v", err)
	}
	if a.Status() != domain.StatusDraft {
		t.Fatalf("被打回的文章应为草稿，实际 %v", a.Status())
	}
}

// TestGateBucketPassesWhenEveryDimensionClears 钉住木桶的另一半：六项都过线
// 且总分过线才放行。
func TestGateBucketPassesWhenEveryDimensionClears(t *testing.T) {
	gate := &fakeQualityGate{score: 78, dimensions: evenDimensions(60)}
	svc, _, _ := newGateService(t, gate)

	a, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))
	if err != nil {
		t.Fatalf("六项都过线且总分过线应放行: %v", err)
	}
	if a.Status() != domain.StatusPublished {
		t.Fatalf("达标应发布，实际 %s", a.Status())
	}
}

// 边界：正好等于及格线算过线（约定是 ">="，与总分口径一致）。
func TestGateBucketAcceptsDimensionExactlyAtPassScore(t *testing.T) {
	gate := &fakeQualityGate{score: 52, dimensions: evenDimensions(50)}
	svc, _, _ := newGateService(t, gate)

	if _, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy())); err != nil {
		t.Fatalf("维度正好 50 分应算过线: %v", err)
	}
}

// 旧评分记录 / 端口没填维度分时退回"只看总分"：不能因为拿不到维度分就把
// 所有人的提交都拒掉，但也不能假装它过了桶判定。
func TestGateBucketFallsBackToTotalWhenDimensionsMissing(t *testing.T) {
	gate := &fakeQualityGate{score: 78}
	svc, _, _ := newGateService(t, gate)

	if _, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy())); err != nil {
		t.Fatalf("没有维度分时应按总分放行: %v", err)
	}
}

// TestGateScoreUnavailableFailsClosedAndReleasesQuota 钉住失败关闭 + 归还额度。
func TestGateScoreUnavailableFailsClosedAndReleasesQuota(t *testing.T) {
	gate := &fakeQualityGate{err: errors.New("上游超时")}
	svc, repos, quota := newGateService(t, gate)

	_, err := svc.Create(context.Background(), gateSubmitCmd(gatePolicy()))
	if err == nil {
		t.Fatal("拿不到可用分数必须失败关闭")
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeArticleScoreUnavailable {
		t.Fatalf("错误码应为 score_unavailable，实际 %s ok=%v", code, ok)
	}
	// 不落库：本次提交什么都没发生。
	got, _, _ := repos.FakeArticles().List(context.Background(),
		domain.Filter{Statuses: []domain.Status{domain.StatusDraft}}, domain.Page{Limit: 100})
	if len(got) != 0 {
		t.Fatalf("评分失败时不应落库，实际 %d 篇", len(got))
	}
	// 不占额度：故障不该让作者当天少一次机会。
	if quota.Used(author) != 0 {
		t.Fatalf("评分失败不应占用额度，实际已用 %d", quota.Used(author))
	}
	if quota.Releases() != 1 {
		t.Fatalf("应归还一次预占，实际 %d", quota.Releases())
	}
}

// TestGateAdminExemptFromQuotaButNotFromGate 钉住管理员豁免的是配额、不是闸门。
func TestGateAdminExemptFromQuotaButNotFromGate(t *testing.T) {
	gate := &fakeQualityGate{score: 40}
	svc, _, quota := newGateService(t, gate)

	cmd := gateSubmitCmd(gatePolicy())
	cmd.Actor = adminViewer()
	_, err := svc.Create(context.Background(), cmd)
	if err == nil {
		t.Fatal("管理员也不该绕过质量闸门")
	}
	if quota.Used(author) != 0 {
		t.Fatalf("管理员不受配额约束，实际已用 %d", quota.Used(author))
	}
}
