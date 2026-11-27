package app

import (
	"context"
	"testing"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 回归：2026-11-27 线上那篇 6 分文章被放行。
//
// 当时质量闸门还没读到评分（并发的评分刚提交），于是报
// `article.score_unavailable` 让事件重试；而合规判定是**另一个**订阅者，
// 中继在错误后仍会执行它，它判 clean 之后把文章推成 visible。重试回来时
// 闸门只看「已发布待可见」，文章已不是待可见，闸门从此对它永不再判。
//
// 现在两步在同一个函数里：拿不到分数就直接返回错误，**不可能**走到合规判定。
// 本用例钉住的就是这条 —— 分数不可用时文章的可见性一个字节都不能变。
func TestJudgePublish_拿不到分数时绝不推进可见性(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000001)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.ensureScore = func(context.Context, ctype.ID) error {
		return errTestScoreUnavailable
	}
	// 合规判定必须**一次都没被调用**：这是本事故的直接回归钉。
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("拿不到分数时绝不能走到合规判定")
		return nil
	}

	err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps)
	if err == nil {
		t.Fatal("拿不到分数时必须报错让事件重试，绝不静默放行")
	}

	got := readArticle(t, infra, articleID)
	if got.VisibilityStatus != enum.ArticleVisibilityPendingReview {
		t.Fatalf("拿不到分数时可见性必须保持 pending_review，实际 %s", got.VisibilityStatus)
	}
	if got.PublishStatus != enum.ArticleStatusPublished {
		t.Fatalf("拿不到分数时状态必须保持已发布，实际 %d", got.PublishStatus)
	}
}

// 失败计数到上限仍拿不到分数：转人工，不把文章永久停在「已发布待可见」。
//
// 选转人工而不是退回草稿，是因为作者没做错任何事 —— 让一个模型故障变成
// 「请你重写一遍」是惩罚错人（ADR-0030 的失败语义）。
func TestJudgePublish_分数持续不可用时转人工(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000011)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.ensureScore = func(context.Context, ctype.ID) error { return errTestScoreUnavailable }
	// 计数已达上限（第 3 次失败）。
	deps.scoreFailures = func(context.Context, ctype.ID) (int, error) { return scoreEscalateFailures, nil }

	var escalated []ctype.ID
	deps.escalate = func(_ context.Context, article noticeArticleRow) error {
		escalated = append(escalated, article.ID)
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("转人工路径不该走到合规判定")
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("达到失败上限后应转人工并收尾事件，不该继续报错: %v", err)
	}
	if len(escalated) != 1 || escalated[0].Uint64() != articleID.Uint64() {
		t.Fatalf("应把该文章转人工一次，实际 %v", escalated)
	}
}

// 上限之前必须继续重试，不能提前转人工 —— 否则一次短暂抖动就把文章推给人。
//
// 计数来自**独立计数**而不是 outbox 的 `e.Attempts`：后者是整条事件的投递次数，
// 兄弟订阅者（通知等）失败也会让它涨，拿它数"评分失败"会把一次故障误判成三次。
// 本用例用「e.Attempts 已经很大、但计数值仍是 1」钉住这条解耦。
func TestJudgePublish_未到上限时继续重试且不看eAttempts(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000031)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.ensureScore = func(context.Context, ctype.ID) error { return errTestScoreUnavailable }
	deps.scoreFailures = func(context.Context, ctype.ID) (int, error) { return 1, nil }

	escalated := 0
	deps.escalate = func(context.Context, noticeArticleRow) error {
		escalated++
		return nil
	}

	err := judgePublish(context.Background(), infra, outboxrelay.Event{
		AggregateID: articleID,
		// 兄弟订阅者反复失败会让它涨到很大：这正是不能拿它计数理由。
		Attempts: 9,
	}, deps)
	if err == nil {
		t.Fatal("未到上限时必须报错让事件重试")
	}
	if escalated != 0 {
		t.Fatalf("未到上限不该转人工，实际转了 %d 次", escalated)
	}
}

// 质量未达标：退回草稿 + 通知，且**绝不**再走到合规判定。
func TestJudgePublish_质量未达标时退回草稿且不放行(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000041)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	var demoted []ctype.ID
	deps.demote = func(_ context.Context, article noticeArticleRow, _ articleapp.QualityAssessment) error {
		demoted = append(demoted, article.ID)
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("质量未达标时绝不能走到合规判定")
		return nil
	}
	deps.assess = func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
		return articleapp.QualityAssessment{Score: 6, Required: 52}, nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("退回草稿应成功: %v", err)
	}
	if len(demoted) != 1 {
		t.Fatalf("应退回草稿一次，实际 %v", demoted)
	}
}

// 质量已过：交给合规判定（它是「可见」的唯一出口）。
func TestJudgePublish_质量已过才交给合规判定(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000051)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.assess = func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
		return articleapp.QualityAssessment{Score: 80, Required: 52, Passed: true}, nil
	}
	called := 0
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		called++
		return nil
	}
	// 开关关闭（默认）：自动放行不得插手，这条是"现状不变"的守卫。
	deps.publishVisible = func(context.Context, ctype.ID) error {
		t.Fatal("「质量达标自动放行」关闭时不该直接放开可见")
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("质量已过应成功: %v", err)
	}
	if called != 1 {
		t.Fatalf("质量已过应交给合规判定一次，实际 %d", called)
	}
}

// ── 「质量达标自动放行」（站点开关，默认 false）─────────────
//
// 语义：质量闸门判定通过之后，不再等第③层 AI 合规判定，直接转 visible。
// 下面五条一起钉住边界 —— 跳过的是"等 AI 判定"这一步，不是"判定失败也放行"：
//  1. 开 + 质量过 → 直接放开可见、不调合规判定；
//  2. 开 + 质量不过 → 照旧退回草稿，绝不放开；
//  3. 开 + 拿不到分数 → 照旧重试 / 转人工，绝不放开；
//  4. 关 → 维持现状（交给合规判定）；
//  5. 闸门关 + 开 → 仍走合规判定（没有质量结论，就没有可依据的放行凭据）。

func TestJudgePublish_自动放行开启且质量已过时跳过合规判定(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000071)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.aiComplianceEnabled = false
	deps.assess = func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
		return articleapp.QualityAssessment{Score: 80, Required: 52, Passed: true}, nil
	}
	var published []ctype.ID
	deps.publishVisible = func(_ context.Context, id ctype.ID) error {
		published = append(published, id)
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("自动放行开启且质量已过时，不该再等 AI 合规判定")
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("自动放行应成功: %v", err)
	}
	if len(published) != 1 || published[0].Uint64() != articleID.Uint64() {
		t.Fatalf("应直接把该文章放开可见一次，实际 %v", published)
	}
}

func TestJudgePublish_自动放行开启但质量不过时仍退回草稿(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000081)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.aiComplianceEnabled = false
	var demoted []ctype.ID
	deps.demote = func(_ context.Context, article noticeArticleRow, _ articleapp.QualityAssessment) error {
		demoted = append(demoted, article.ID)
		return nil
	}
	deps.assess = func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
		return articleapp.QualityAssessment{Score: 6, Required: 52}, nil
	}
	deps.publishVisible = func(context.Context, ctype.ID) error {
		t.Fatal("质量不过时绝不能因为自动放行开关就直接放开可见")
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("质量不过时不该走到合规判定")
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("退回草稿应成功: %v", err)
	}
	if len(demoted) != 1 {
		t.Fatalf("应退回草稿一次，实际 %v", demoted)
	}
	// 可见性没被这条判定路径推进：deps.demote 是替身、不写库，所以这里只能证明
	// "判定路径自己没有放开可见"，退回草稿本身由 demote 的真实实现负责。
	if got := readArticle(t, infra, articleID); got.VisibilityStatus != enum.ArticleVisibilityPendingReview {
		t.Fatalf("质量不过时判定路径不得推进可见性，实际 %s", got.VisibilityStatus)
	}
}

func TestJudgePublish_自动放行开启但拿不到分数时绝不放行(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000091)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.aiComplianceEnabled = false
	deps.ensureScore = func(context.Context, ctype.ID) error { return errTestScoreUnavailable }
	deps.publishVisible = func(context.Context, ctype.ID) error {
		t.Fatal("拿不到分数时自动放行绝不能放开可见")
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("拿不到分数时不该走到合规判定")
		return nil
	}

	err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps)
	if err == nil {
		t.Fatal("拿不到分数时必须报错让事件重试，绝不因为自动放行开关就静默放行")
	}
	if got := readArticle(t, infra, articleID); got.VisibilityStatus != enum.ArticleVisibilityPendingReview {
		t.Fatalf("拿不到分数时可见性必须保持 pending_review，实际 %s", got.VisibilityStatus)
	}
}

func TestJudgePublish_自动放行关闭时仍交给合规判定(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000101)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.aiComplianceEnabled = true
	deps.assess = func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
		return articleapp.QualityAssessment{Score: 80, Required: 52, Passed: true}, nil
	}
	called := 0
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		called++
		return nil
	}
	deps.publishVisible = func(context.Context, ctype.ID) error {
		t.Fatal("开关关闭时不该直接放开可见（这条是现状守卫）")
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("开关关闭时应走合规判定: %v", err)
	}
	if called != 1 {
		t.Fatalf("开关关闭时合规判定应被调用一次，实际 %d", called)
	}
}

// TestJudgePublish_闸门关闭且AI关时直接可见 钉住"没有机器判据 ⇒ 可见"这条出口。
//
// 与旧用例（TestJudgePublish_闸门关闭时自动放行开关不生效）的差别是语义本身变了：
// 旧开关「质量达标自动放行」表达的是"质量判过之后就跳过 AI"，所以闸门关着时
// 它不成立（没有质量结论可跳）。新开关「AI 合规判定」表达的是"不要用 AI 判"，
// 闸门关着同样成立 —— 两条判据都不存在，文章就该像提交侧那条同源分支一样直接可见。
func TestJudgePublish_闸门关闭且AI关时直接可见(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000111)
	// 闸门关闭 + AI 关：两条机器判据都不存在。
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: false})
	deps.aiComplianceEnabled = false
	complianceCalls := 0
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		complianceCalls++
		return nil
	}
	published := 0
	deps.publishVisible = func(context.Context, ctype.ID) error {
		published++
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("闸门关闭且 AI 关时应直接放开可见: %v", err)
	}
	if published != 1 {
		t.Fatalf("应直接放开可见一次，实际 %d 次", published)
	}
	if complianceCalls != 0 {
		t.Fatalf("AI 关掉之后不该再调合规判定，实际 %d 次", complianceCalls)
	}
}

// 闸门关闭时评分失败**不阻塞**合规判定：此时分数只用于审核队列展示，
// 不该让一次模型故障卡住整条发布链路（与引入闸门之前的行为一致）。
func TestJudgePublish_闸门关闭时评分失败不阻塞(t *testing.T) {
	infra, articleID := pendingArticle(t, 304000000000000061)
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: false})
	deps.ensureScore = func(context.Context, ctype.ID) error { return errTestScoreUnavailable }
	warned := 0
	deps.warnf = func(string, ...any) { warned++ }
	called := 0
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		called++
		return nil
	}

	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("闸门关闭时评分失败不该阻塞: %v", err)
	}
	if called != 1 {
		t.Fatalf("闸门关闭时仍应交给合规判定一次，实际 %d", called)
	}
	if warned == 0 {
		t.Fatal("降级路径必须留一条日志，否则线上无法复盘")
	}
}

// 已不是「已发布待可见」的文章，这条链路不该再碰它。
func TestJudgePublish_只管待可见的文章(t *testing.T) {
	infra := &Infra{DB: testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{}), Logger: testutil.Logger()}
	articleID := ctype.NewID(304000000000000021)
	if err := infra.DB.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "已公开的文章",
		AuthorID:         ctype.NewID(304000000000000022),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityVisible,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("已公开的文章不该再进合规判定")
		return nil
	}
	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: articleID}, deps); err != nil {
		t.Fatalf("已公开的文章不该被这条链路判定: %v", err)
	}
}

// 文章已经不在了（被删了）：这条链路该安静退出，不能报错。
//
// 为什么必须钉住：2026-11 的 8 条死信里有 4 条就是"创建事件晚到、文章已被删"，
// 当时的评分订阅者把 gorm.ErrRecordNotFound 当失败 → 重试 5 次 → 死信告警常亮。
// 删除是幂等的终点：文章都没了，还判什么发布？重试再多次也不会有不同的答案。
func TestJudgePublish_文章已被删时安静退出(t *testing.T) {
	infra := &Infra{DB: testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{}), Logger: testutil.Logger()}
	deps := fakeDeps(t, articleapp.QualityGatePolicy{Enabled: true, PassScore: 50, GrayBandWidth: 2})
	deps.ensureScore = func(context.Context, ctype.ID) error {
		t.Fatal("文章不存在时不该去补评分")
		return nil
	}
	deps.compliance = func(context.Context, outboxrelay.Event) error {
		t.Fatal("文章不存在时不该进合规判定")
		return nil
	}

	// 不预置文章行 —— 这就是"事件到了、文章已经没了"。
	missing := ctype.NewID(304000000000000031)
	if err := judgePublish(context.Background(), infra, outboxrelay.Event{AggregateID: missing}, deps); err != nil {
		t.Fatalf("文章不存在不该返回错误（会让中继重试到死信）: %v", err)
	}
}

// ── 用例脚手架 ─────────────────────────────────────────────

// errTestScoreUnavailable 是"评分通道不可用"的替身错误。
var errTestScoreUnavailable = &testError{"评分暂时不可用"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func pendingArticle(t *testing.T, id uint64) (*Infra, ctype.ID) {
	t.Helper()
	infra := &Infra{
		DB:     testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{}, &schema.ArticleReviewTaskRow{}),
		Logger: testutil.Logger(),
	}
	articleID := ctype.NewID(id)
	if err := infra.DB.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "待可见的文章",
		AuthorID:         ctype.NewID(id + 1),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityPendingReview,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}
	return infra, articleID
}

func readArticle(t *testing.T, infra *Infra, articleID ctype.ID) schema.ArticleRow {
	t.Helper()
	var got schema.ArticleRow
	if err := infra.DB.First(&got, "id = ?", articleID).Error; err != nil {
		t.Fatalf("读回文章失败: %v", err)
	}
	return got
}

// fakeDeps 构造一份"什么都不做、但每一步都可替换"的依赖。
//
// 默认实现让用例只需替换自己关心的那一两步：不需要为了测一条分支去装配
// 一整套业务服务（那只会测到装配代码）。
func fakeDeps(t *testing.T, policy articleapp.QualityGatePolicy) publishJudgeHooks {
	t.Helper()
	return publishJudgeHooks{
		policy:        policy,
		ensureScore:   func(context.Context, ctype.ID) error { return nil },
		scoreFailures: func(context.Context, ctype.ID) (int, error) { return 1, nil },
		clearFailures: func(context.Context, ctype.ID) error { return nil },
		assess: func(context.Context, ctype.ID, articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
			return articleapp.QualityAssessment{Score: 80, Required: policy.PublishLine(), Passed: true}, nil
		},
		demote:   func(context.Context, noticeArticleRow, articleapp.QualityAssessment) error { return nil },
		escalate: func(context.Context, noticeArticleRow) error { return nil },
		compliance: func(context.Context, outboxrelay.Event) error {
			return nil
		},
		// 默认「评分开 + AI 合规判定开」：既有用例的语义保持不变（质量已过 → 合规判定）。
		// 要测两条跳过路径的用例自己覆盖这两个值。
		scoreEnabled:        true,
		aiComplianceEnabled: true,
		// 默认实现让"开关开"的用例不必都自己给一个空实现；要断言调用次数时覆盖它。
		publishVisible: func(context.Context, ctype.ID) error { return nil },
		warnf:          func(string, ...any) {},
	}
}
