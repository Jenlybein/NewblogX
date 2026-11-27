package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 「命中即拒」是文章面的 opt-in 处置（content_filter.auto_dispose.block_action=reject）。
//
// 本文件钉住四条口径，缺一条都会让这个开关出岔子：
//   - 端口报"命中即拒"时**一个字节都不落库**，也不建审核任务、不发 article.published；
//   - 拒绝 ≠ 不记录：过滤（含写命中记录）确实跑过了，证据留在风险页；
//   - 端口报 false（默认，或命中落在白名单里）时行为与今天逐字节一致；
//   - Update 与 Create 同一口径，否则"存草稿时被替换成星号、提交时不再命中"
//     会让拒发失效。
//
// 端口上那个布尔由装配层合取"block 档真被打星"与"站点配了 reject"
// （见 app/article_wiring.go 的 articleTextFilterBridge）；这里只验用例层
// 拿到这个事实之后怎么处理。

// 错误文案必须是固定那句，且不带命中词 / 档位 / 类目 —— 回显等于把词表送出去。
func assertContentBlockedError(t *testing.T, err error) {
	t.Helper()
	if !apperr.IsCode(err, apperr.CodeArticleContentBlocked) {
		t.Fatalf("应返回 article.content_blocked，实际 %v", err)
	}
	if got, want := err.Error(), apperr.Message(apperr.CodeArticleContentBlocked); got != want {
		t.Fatalf("错误文案应为固定那句，实际 %q", got)
	}
}

// TestCreateRejectsMaskedBlockTier 提交路径：命中即拒，什么都不落库。
func TestCreateRejectsMaskedBlockTier(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"代开发票"}, maskedBlockTier: true}
	svc, repos := newFilteredService(t, filter)

	created, err := svc.Create(context.Background(), app.CreateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "关于代开发票的科普",
		Content:       "代开发票是违法行为，请勿尝试",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: false,
	})
	assertContentBlockedError(t, err)
	if created != nil {
		t.Fatalf("被拒的提交不该带回任何创建结果，实际 %+v", created)
	}

	// 1) 一个字节都没落库。
	if calls := repos.FakeArticles().Calls("Create"); calls != 0 {
		t.Fatalf("被拒的提交不该写文章，实际 Create 调用了 %d 次", calls)
	}
	if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
		t.Fatalf("被拒的提交不该留下文章，实际 %v", ids)
	}
	// 2) 不建审核任务（否则人工队列里会多一条永远查不到文章的孤儿任务）。
	if calls := repos.FakeReviews().Calls("Save"); calls != 0 {
		t.Fatalf("被拒的提交不该建审核任务，实际 Save 调用了 %d 次", calls)
	}
	// 3) 不发 article.published（下游评分 / 合规 / 通知都不该被惊动）。
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("被拒的提交不该发领域事件，实际 %v", names)
	}
	// 4) 拒绝 ≠ 不记录：过滤这一步确实跑过（命中记录在真实实现里就是这里写下的）。
	if len(filter.calls) == 0 {
		t.Fatal("过滤端口应被调用过（拒发不等于不做过滤与留痕）")
	}
}

// TestCreateRejectsMaskedBlockTierOnDraft 草稿也拦。
//
// 只拦提交的话，作者可以把命中词存进草稿（那一步已被替换成星号），再把星号改回
// 原词提交 —— 那时文本里已经没有词，拒发会整条失效。
func TestCreateRejectsMaskedBlockTierOnDraft(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"冰毒"}, maskedBlockTier: true}
	svc, repos := newFilteredService(t, filter)

	_, err := svc.Create(context.Background(), app.CreateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "冰毒科普",
		Content:       "本文讨论冰毒",
		TargetStatus:  domain.StatusDraft,
		SkipExamining: true,
	})
	assertContentBlockedError(t, err)
	if calls := repos.FakeArticles().Calls("Create"); calls != 0 {
		t.Fatalf("草稿被拒时同样不该写文章，实际 Create 调用了 %d 次", calls)
	}
}

// TestCreateMaskedWordsStillPassesSubmitWhenDispositionOff 默认（disposition 关）
// 时行为与今天逐字节一致：命中词被替换成等长星号，稿件照常提交。
//
// 这是"默认必须与改造前一致"的守卫：拒发是 opt-in，端口报 false 就只能打星。
func TestCreateMaskedWordsStillPassesSubmitWhenDispositionOff(t *testing.T) {
	t.Parallel()

	// maskedBlockTier 留零值 = 端口报 false（站点没配 reject，或命中落在白名单里）。
	filter := &fakeTextFilter{words: []string{"代开发票"}}
	svc, repos := newFilteredService(t, filter)

	created, err := svc.Create(context.Background(), app.CreateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "关于代开发票的科普",
		Content:       "代开发票是违法行为，请勿尝试",
		TargetStatus:  domain.StatusExamining,
		SkipExamining: false,
	})
	if err != nil {
		t.Fatalf("命中词经替换后应照常提交成功: %v", err)
	}
	if created.FilteredCount != 2 {
		t.Fatalf("标题与正文各命中一处，filtered_count 应为 2，实际 %d", created.FilteredCount)
	}
	if got := created.Article.Title(); got != "关于****的科普" {
		t.Fatalf("标题应为替换后文本，实际 %q", got)
	}
	if got := created.Article.Content(); got != "****是违法行为，请勿尝试" {
		t.Fatalf("正文应为替换后文本，实际 %q", got)
	}
	stored, err := repos.FakeArticles().Find(context.Background(), created.Article.ID())
	if err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	if stored.Status() != domain.StatusPublished || stored.Visibility() != domain.VisibilityPendingReview {
		t.Fatalf("应落 Published + pending_review，实际 %s / %s", stored.Status(), stored.Visibility())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 1 || names[0] != app.EventArticlePublished {
		t.Fatalf("应发一条 article.published，实际 %v", names)
	}
}

// TestUpdateRejectsMaskedBlockTier 更新路径同一口径：拒发时文章一个字段都不改。
func TestUpdateRejectsMaskedBlockTier(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"代开发票"}, maskedBlockTier: true}
	svc, repos := newFilteredService(t, filter)
	seedArticle(t, repos, ctype.NewID(1), author, domain.StatusDraft)
	original, err := repos.FakeArticles().Find(context.Background(), ctype.NewID(1))
	if err != nil {
		t.Fatalf("读种子文章失败: %v", err)
	}

	blockedText := "代开发票找我"
	_, err = svc.Update(context.Background(), app.UpdateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:     authorViewer(),
		Policy:    domain.OpenWritePolicy(),
		ArticleID: ctype.NewID(1),
		Content:   &blockedText,
	})
	assertContentBlockedError(t, err)

	if calls := repos.FakeArticles().Calls("Update"); calls != 0 {
		t.Fatalf("被拒的更新不该写文章，实际 Update 调用了 %d 次", calls)
	}
	stored, err := repos.FakeArticles().Find(context.Background(), ctype.NewID(1))
	if err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	if stored.Content() != original.Content() {
		t.Fatalf("被拒的更新不该改动正文，期望 %q，实际 %q", original.Content(), stored.Content())
	}
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("被拒的更新不该发领域事件，实际 %v", names)
	}
	if len(filter.calls) == 0 {
		t.Fatal("过滤端口应被调用过（拒发不等于不做过滤与留痕）")
	}
}

// TestUpdateWatchOnlyHitStillSaves 只有 watch 档命中（端口报 false）时照常保存。
//
// watch 档本期只有 mask：推广话术类词的误报代价低，拒发它们等于把"加微信"
// 三个字写成违规内容。
func TestUpdateWatchOnlyHitStillSaves(t *testing.T) {
	t.Parallel()

	// 词表里只有 watch 词；替身报 false（与真实链路里 block_action 判断的结果一致）。
	filter := &fakeTextFilter{words: []string{"加微信"}}
	svc, repos := newFilteredService(t, filter)
	seedArticle(t, repos, ctype.NewID(1), author, domain.StatusDraft)

	promoted := "想合作请加微信"
	result, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:     authorViewer(),
		Policy:    domain.OpenWritePolicy(),
		ArticleID: ctype.NewID(1),
		Content:   &promoted,
	})
	if err != nil {
		t.Fatalf("只有 watch 命中时应照常保存: %v", err)
	}
	if result.FilteredCount != 1 {
		t.Fatalf("watch 命中一处，filtered_count 应为 1，实际 %d", result.FilteredCount)
	}
	stored, err := repos.FakeArticles().Find(context.Background(), ctype.NewID(1))
	if err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	if got := stored.Content(); got != "想合作请***" {
		t.Fatalf("watch 命中同样打星入库，实际 %q", got)
	}
}

// TestUpdateDoesNotLeakWordTableForForeignArticle 更新别人的文章时不能暴露"这个词被拦"。
//
// 这是"拒发放在只读预演之后"的理由：预演里含"这篇文章是不是我的"这条判定。
// 放到它之前，任何登录用户都能拿别人的文章 ID 当探针，用两种错误码
// （content_blocked / not_found）把词表一条条问出来 —— 而不必拥有任何文章。
func TestUpdateDoesNotLeakWordTableForForeignArticle(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"代开发票"}, maskedBlockTier: true}
	svc, repos := newFilteredService(t, filter)
	seedArticle(t, repos, ctype.NewID(1), stranger, domain.StatusDraft)

	blockedText := "代开发票找我"
	_, err := svc.Update(context.Background(), app.UpdateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:     authorViewer(),
		Policy:    domain.OpenWritePolicy(),
		ArticleID: ctype.NewID(1),
		Content:   &blockedText,
	})
	if !apperr.IsCode(err, apperr.CodeArticleNotFound) {
		t.Fatalf("他人文章一律报不存在（不能因命中词变成 content_blocked），实际 %v", err)
	}
	if calls := repos.FakeArticles().Calls("Update"); calls != 0 {
		t.Fatalf("他人的文章不该被写，实际 Update 调用了 %d 次", calls)
	}
}

// TestCreateRejectsMaskedBlockTierWithoutRefundingQuota 被拒的提交**不归还**配额。
//
// 这是刻意的：拒发是作者自己能改掉的失败（换个说法就能过），归还额度等于给
// 反复试探词表的人发免费次数 —— 试探一次成本为零，"命中即拒"就只剩下日志噪音。
func TestCreateRejectsMaskedBlockTierWithoutRefundingQuota(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"代开发票"}, maskedBlockTier: true}
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	quota := apptest.NewSubmitQuota(15)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), quota, nil, nil).
		WithFilterDisposition(func() app.FilterDisposition {
			return app.FilterDisposition{Block: app.HitReject, Watch: app.HitRelease}
		}).
		WithContentFilter(fakeArticleTextFilter{inner: filter}).
		WithCollectionContentFilter(fakeCollectionTextFilter{inner: filter})
	repos.FakeUsers().Seed(app.AuthorView{ID: author, Nickname: "作者", Username: "u100"})
	repos.FakeCategories().Seed()

	_, err := svc.Create(context.Background(), app.CreateArticleCmd{
		ScoreEnabled: true, AIComplianceEnabled: true,
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "关于代开发票的科普",
		Content:      "代开发票是违法行为",
		TargetStatus: domain.StatusExamining,
	})
	assertContentBlockedError(t, err)

	if got := quota.Used(author); got != 1 {
		t.Fatalf("被拒的提交也不该归还配额（一次试探消耗一次额度），实际 Used=%d", got)
	}
	if ids := repos.FakeArticles().IDs(); len(ids) != 0 {
		t.Fatalf("被拒的提交不该落库，实际 %v", ids)
	}
}
