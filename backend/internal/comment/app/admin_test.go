package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 管理端「评论管理」的用例（第一期：列表 / 批量删除 / 禁言 / 封禁）。
//
// 这一层钉的是外部可观察行为，不钉实现细节：
//   - 列表按各维度筛选、分页口径（默认 20 / 上限 100）、has_more 透传，
//     文章标题来自 ArticleInfo 端口（评论模块不与 article 表 JOIN）；
//   - 批量删除走与 Remove 相同的链路（同根回复一起删、计数只扣已发布条数），
//     部分失败如实逐条报告，已删掉的 ID 记成功（不重复删、不报假失败）；
//   - 禁言档位只有 1/7/30/永久（0），理由必填且 ≤200 字；
//   - 三个写接口 + 列表对非管理员一律 comment.forbidden。

// adminListArticleID 是列表用例里的文章 ID（与 fixture 预置的标题对上）。
var adminListArticleID = ctype.NewID(100)

// TestAdminListFiltersAndPaginates 钉住筛选透传、分页口径与行映射。
func TestAdminListFiltersAndPaginates(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local)
	// 库里"已删除"的真实形状：删除动作只写 deleted_at，status 列仍是已发布
	// （见 adapter/mysql 的 SoftDeleteMany）。列表必须把它报告成"已删除"。
	deletedAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	f.repo.adminRows = []domain.Comment{
		{
			ID:           ctype.NewID(31),
			CreatedAt:    time.Date(2026, 11, 1, 10, 0, 0, 0, time.Local),
			Content:      "还在的评论",
			UserID:       ctype.NewID(7),
			UserNickname: "路人甲",
			UserAvatar:   "a.png",
			ArticleID:    adminListArticleID,
			DiggCount:    3,
			ReplyCount:   2,
			Status:       domain.StatusPublished,
		},
		{
			ID:           ctype.NewID(32),
			CreatedAt:    time.Date(2026, 11, 1, 11, 0, 0, 0, time.Local),
			Content:      "已经删掉的评论",
			UserID:       ctype.NewID(8),
			UserNickname: "路人乙",
			ArticleID:    adminListArticleID,
			Status:       domain.StatusPublished,
			DeletedAt:    &deletedAt,
		},
	}
	f.repo.adminHasMore = true

	items, hasMore, err := f.svc.AdminList(context.Background(), adminViewer, domain.AdminFilter{
		Keyword:     "  刷屏  ",
		UserID:      ctype.NewID(7),
		UserKeyword: "  路人  ",
		ArticleID:   adminListArticleID,
		FromAt:      &from,
		ToAt:        &to,
		// 已删除视图：管理端要能看到被删掉的评论（这是它和读者侧列表的区别）。
		Status: domain.AdminStatusDeleted,
	}, domain.AdminPage{Page: 2, Limit: 50})
	if err != nil {
		t.Fatalf("管理端列表不该失败: %v", err)
	}
	if !hasMore {
		t.Fatal("has_more 应原样透传给上层（否则前端的「加载更多」按钮永远不出现）")
	}

	// 筛选条件必须去首尾空白后透传：空白关键词会退化成 LIKE '%%'（全表扫描）。
	if f.repo.adminFilter.Keyword != "刷屏" {
		t.Fatalf("关键词应去空白后透传，实际 %q", f.repo.adminFilter.Keyword)
	}
	if f.repo.adminFilter.UserKeyword != "路人" {
		t.Fatalf("昵称关键词应去空白后透传，实际 %q", f.repo.adminFilter.UserKeyword)
	}
	if f.repo.adminFilter.UserID != ctype.NewID(7) || f.repo.adminFilter.ArticleID != adminListArticleID {
		t.Fatalf("用户 / 文章筛选透传不对: %+v", f.repo.adminFilter)
	}
	if f.repo.adminFilter.FromAt == nil || !f.repo.adminFilter.FromAt.Equal(from) {
		t.Fatalf("起始时间透传不对: %v", f.repo.adminFilter.FromAt)
	}
	if f.repo.adminFilter.ToAt == nil || !f.repo.adminFilter.ToAt.Equal(to) {
		t.Fatalf("结束时间透传不对: %v", f.repo.adminFilter.ToAt)
	}
	if f.repo.adminFilter.Status != domain.AdminStatusDeleted {
		t.Fatalf("状态筛选透传不对: %q", f.repo.adminFilter.Status)
	}
	if f.repo.adminPage.Page != 2 || f.repo.adminPage.Limit != 50 {
		t.Fatalf("分页透传不对: %+v", f.repo.adminPage)
	}

	if len(items) != 2 {
		t.Fatalf("应返回 2 行，实际 %d 行", len(items))
	}
	if items[0].ArticleTitle != "Golang 并发模型" {
		t.Fatalf("文章标题应来自 ArticleInfo 端口，实际 %q", items[0].ArticleTitle)
	}
	if items[0].Deleted || items[0].Status != int8(domain.StatusPublished) {
		t.Fatalf("已发布行不该标 deleted: %+v", items[0])
	}
	if !items[1].Deleted || items[1].Status != int8(domain.StatusDeleted) {
		t.Fatalf("软删过的行应报告 deleted=true、status=已删除（库里 status 仍是 2）: %+v", items[1])
	}
	if items[0].UserNickname != "路人甲" || items[0].UserAvatar != "a.png" {
		t.Fatalf("用户快照应原样带出（列表不 JOIN 用户表）: %+v", items[0])
	}

	// 标题只按"这一页里出现过的文章 ID"批量查一次，不逐条查。
	if f.articles.calls.titles != 1 {
		t.Fatalf("文章标题应批量查一次，实际 %d 次", f.articles.calls.titles)
	}
	if len(f.articles.calls.titlesIDs) != 1 {
		t.Fatalf("两行同一篇文章应只查一个 ID，实际 %v", f.articles.calls.titlesIDs)
	}
}

// TestAdminListNormalizesPage 钉住分页口径：默认 20 条、上限 100、页码至少 1。
func TestAdminListNormalizesPage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		page     domain.AdminPage
		wantPage int
		wantSize int
	}{
		{"缺省用 20 条", domain.AdminPage{}, 1, domain.AdminDefaultLimit},
		{"超过上限夹到 100", domain.AdminPage{Page: 3, Limit: 1000}, 3, domain.AdminMaxLimit},
		{"页码 0 归 1", domain.AdminPage{Page: 0, Limit: 10}, 1, 10},
	}
	for _, c := range cases {
		f := newFixture(t)
		if _, _, err := f.svc.AdminList(context.Background(), adminViewer, domain.AdminFilter{}, c.page); err != nil {
			t.Fatalf("%s: 不该失败: %v", c.name, err)
		}
		if f.repo.adminPage.Page != c.wantPage || f.repo.adminPage.Limit != c.wantSize {
			t.Fatalf("%s: 分页应为 (%d,%d)，实际 %+v", c.name, c.wantPage, c.wantSize, f.repo.adminPage)
		}
	}
}

// TestAdminListRejectsUnknownStatus 未知状态值必须报参数错误，不能静默当成 all。
func TestAdminListRejectsUnknownStatus(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	// handler 会先 parse；这里直接给用例塞一个非法值，验证"绕过 HTTP 也拒绝"。
	_, _, err := f.svc.AdminList(context.Background(), adminViewer,
		domain.AdminFilter{Status: domain.AdminStatus("published-ish")}, domain.AdminPage{})
	if !apperr.IsCode(err, apperr.CodeCommentAdminStatusInvalid) {
		t.Fatalf("未知状态应返回 comment.admin_status_invalid，实际 %v", err)
	}
}

// TestAdminListSurvivesArticleTitleFailure 标题读不到不该让整个列表打不开。
func TestAdminListSurvivesArticleTitleFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.repo.adminRows = []domain.Comment{{
		ID:        ctype.NewID(41),
		ArticleID: ctype.NewID(999),
		UserID:    ctype.NewID(7),
		Status:    domain.StatusPublished,
	}}
	f.articles.titlesErr = errors.New("文章标题查询失败")

	items, _, err := f.svc.AdminList(context.Background(), adminViewer, domain.AdminFilter{}, domain.AdminPage{})
	if err != nil {
		t.Fatalf("标题失败不该让列表整体失败: %v", err)
	}
	if len(items) != 1 || items[0].ArticleTitle != "" {
		t.Fatalf("标题失败时应留空串但行还在，实际 %+v", items)
	}
	if f.logger.errors == 0 {
		t.Fatal("标题失败必须留一条 error 日志，否则这个坏查询永远没人知道")
	}
}

// TestAdminRemoveManyReportsPartialFailure 部分失败要如实逐条报告，不能整单回滚。
func TestAdminRemoveManyReportsPartialFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	// 三条都是二级评论（非根）：删除时走单条路径，不会触发 ListFamily 级联。
	ids := []ctype.ID{ctype.NewID(51), ctype.NewID(52), ctype.NewID(53)}
	for _, id := range ids {
		f.repo.byID[id.Uint64()] = domain.Comment{
			ID:        id,
			ArticleID: adminListArticleID,
			UserID:    ctype.NewID(7),
			ReplyID:   ctype.NewID(50),
			RootID:    ctype.NewID(50),
			Status:    domain.StatusPublished,
		}
	}
	// 中间那条报一个"没有错误码"的内部错误：文案应折成通用服务异常，不透 SQL。
	f.repo.findErrors[52] = errors.New("dial tcp 127.0.0.1:3306: connect: refused")

	result, err := f.svc.AdminRemoveMany(context.Background(), adminViewer, ids)
	if err != nil {
		t.Fatalf("逐条失败不该让整批报错: %v", err)
	}
	if result.Succeeded != 2 {
		t.Fatalf("应成功 2 条，实际 %d", result.Succeeded)
	}
	if len(result.Failed) != 1 || result.Failed[0].ID != ctype.NewID(52) {
		t.Fatalf("应只有 52 失败，实际 %+v", result.Failed)
	}
	if result.Failed[0].Message != apperr.Message(apperr.CodeCommonServiceFailed) {
		t.Fatalf("内部错误应折成通用服务异常文案，实际 %q", result.Failed[0].Message)
	}
	// 成功的两条已经生效（不留"整批回滚"的半截状态）。
	if len(f.repo.softDeleted) != 2 {
		t.Fatalf("成功的那两条应已软删，实际 %v", f.repo.softDeleted)
	}
}

// TestAdminRemoveManyCascadesRootFamilyAndCounters 钉住"删除仍走原有级联"。
func TestAdminRemoveManyCascadesRootFamilyAndCounters(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	rootID := ctype.NewID(61)
	root := domain.Comment{
		ID: rootID, ArticleID: adminListArticleID, UserID: ctype.NewID(7),
		Status: domain.StatusPublished,
	}
	publishedReply := domain.Comment{
		ID: ctype.NewID(62), ArticleID: adminListArticleID, RootID: rootID,
		ReplyID: rootID, UserID: ctype.NewID(8), Status: domain.StatusPublished,
	}
	deletedReply := domain.Comment{
		ID: ctype.NewID(63), ArticleID: adminListArticleID, RootID: rootID,
		ReplyID: rootID, UserID: ctype.NewID(9), Status: domain.StatusDeleted,
	}
	for _, row := range []domain.Comment{root, publishedReply, deletedReply} {
		f.repo.byID[row.ID.Uint64()] = row
	}
	f.repo.family[rootID.Uint64()] = []domain.Comment{root, publishedReply, deletedReply}

	result, err := f.svc.AdminRemoveMany(context.Background(), adminViewer, []ctype.ID{rootID})
	if err != nil {
		t.Fatalf("删除根评论不该失败: %v", err)
	}
	if result.Succeeded != 1 || len(result.Failed) != 0 {
		t.Fatalf("一条根评论应记 1 条成功，实际 %+v", result)
	}
	// 同根下的回复（含早已删除的那条）一起软删：级联语义必须在后台也成立。
	wantIDs := []uint64{61, 62, 63}
	if len(f.repo.softDeleted) != len(wantIDs) {
		t.Fatalf("级联应软删 %v，实际 %v", wantIDs, f.repo.softDeleted)
	}
	for i, want := range wantIDs {
		if f.repo.softDeleted[i] != want {
			t.Fatalf("级联软删顺序 / 内容不对: want=%v got=%v", wantIDs, f.repo.softDeleted)
		}
	}
	// 文章计数只按**已发布**的条数扣（已删除的那条早就不在计数里了）。
	if f.cache.added != 1 || f.cache.lastArticleDelta != -2 {
		t.Fatalf("文章评论数应扣 2（只算已发布），实际调用 %d 次、增量 %d",
			f.cache.added, f.cache.lastArticleDelta)
	}
}

// TestAdminRemoveManyTreatsAlreadyGoneAsSuccess 同批选了一级评论与它下面的回复时，
// 后者的级联已经把前者删掉；这种情况记成功而不是假失败。
func TestAdminRemoveManyTreatsAlreadyGoneAsSuccess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	rootID := ctype.NewID(71)
	root := domain.Comment{
		ID: rootID, ArticleID: adminListArticleID, UserID: ctype.NewID(7),
		Status: domain.StatusPublished,
	}
	reply := domain.Comment{
		ID: ctype.NewID(72), ArticleID: adminListArticleID, RootID: rootID,
		ReplyID: rootID, UserID: ctype.NewID(8), Status: domain.StatusPublished,
	}
	f.repo.byID[rootID.Uint64()] = root
	f.repo.family[rootID.Uint64()] = []domain.Comment{root, reply}

	result, err := f.svc.AdminRemoveMany(context.Background(), adminViewer, []ctype.ID{rootID, reply.ID})
	if err != nil {
		t.Fatalf("不该失败: %v", err)
	}
	if result.Succeeded != 2 || len(result.Failed) != 0 {
		t.Fatalf("已删除的 ID 应记成功，实际 %+v", result)
	}
}

// TestAdminRemoveManyRejectsBadSelection 请求级非法在动手之前整体拒绝。
func TestAdminRemoveManyRejectsBadSelection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ids  []ctype.ID
	}{
		{"一条都没选", nil},
		{"超过 100 条", makeManyIDs(domain.MaxAdminBatchIDs + 1)},
		{"混了空 ID", []ctype.ID{ctype.NewID(81), ""}},
	}
	for _, c := range cases {
		f := newFixture(t)
		if _, err := f.svc.AdminRemoveMany(context.Background(), adminViewer, c.ids); !apperr.IsCode(err, apperr.CodeCommentAdminBatchInvalid) {
			t.Fatalf("%s: 应返回 comment.admin_batch_invalid，实际 %v", c.name, err)
		}
		if len(f.repo.softDeleted) != 0 {
			t.Fatalf("%s: 参数非法时一条都不该删，实际 %v", c.name, f.repo.softDeleted)
		}
	}
}

// makeManyIDs 造一批互不相同的评论 ID。
func makeManyIDs(n int) []ctype.ID {
	ids := make([]ctype.ID, 0, n)
	for i := 1; i <= n; i++ {
		ids = append(ids, ctype.NewID(uint64(100+i)))
	}
	return ids
}

// TestAdminMutePermanentIsExplicit 永久档必须是显式的 0，原样交给账号端口。
func TestAdminMutePermanentIsExplicit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	wantUntil := f.accounts.muteUntil

	until, err := f.svc.AdminMute(context.Background(), adminViewer, ctype.NewID(7), 0, " 刷屏  ")
	if err != nil {
		t.Fatalf("永久禁言不该失败: %v", err)
	}
	if len(f.accounts.muteCalls) != 1 {
		t.Fatalf("应调用一次账号处置端口，实际 %d 次", len(f.accounts.muteCalls))
	}
	call := f.accounts.muteCalls[0]
	if call.days != 0 {
		t.Fatalf("永久档应原样传 0（由桥接层换算成远期时间），实际传了 %d 天", call.days)
	}
	if call.target != ctype.NewID(7) || call.actor != adminViewer.ID {
		t.Fatalf("禁言目标 / 发起人不对: %+v", call)
	}
	if !until.Equal(wantUntil) {
		t.Fatalf("应回传账号端口给的到期时间，实际 %v", until)
	}
}

// TestAdminMuteRejectsBadInput 档位与理由的校验都要给出明确错误码。
func TestAdminMuteRejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		viewer  domain.Viewer
		target  ctype.ID
		days    int
		reason  string
		wantErr apperr.Code
	}{
		{"非管理员", domain.Viewer{ID: ctype.NewID(7)}, ctype.NewID(7), 7, "刷屏", apperr.CodeCommentForbidden},
		{"目标用户为空", adminViewer, "", 7, "刷屏", apperr.CodeCommentAdminUserInvalid},
		{"理由为空", adminViewer, ctype.NewID(7), 7, "   ", apperr.CodeCommentAdminReasonRequired},
		{"理由超过 200 字", adminViewer, ctype.NewID(7), 7, strings.Repeat("长", domain.MaxAdminReasonLen+1), apperr.CodeCommentAdminReasonTooLong},
		{"天数不在四档", adminViewer, ctype.NewID(7), 3, "刷屏", apperr.CodeCommentAdminDaysInvalid},
		{"天数为负", adminViewer, ctype.NewID(7), -1, "刷屏", apperr.CodeCommentAdminDaysInvalid},
	}
	for _, c := range cases {
		f := newFixture(t)
		_, err := f.svc.AdminMute(context.Background(), c.viewer, c.target, c.days, c.reason)
		if !apperr.IsCode(err, c.wantErr) {
			t.Fatalf("%s: 应返回 %s，实际 %v", c.name, c.wantErr, err)
		}
		if len(f.accounts.muteCalls) != 0 {
			t.Fatalf("%s: 参数非法时不该真的禁言，实际调用 %+v", c.name, f.accounts.muteCalls)
		}
	}
}

// TestAdminBanCallsAccountDisposer 封禁要落到账号端口（走 user 的 AdminUpdate 口径）。
func TestAdminBanCallsAccountDisposer(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	if err := f.svc.AdminBan(context.Background(), adminViewer, ctype.NewID(9), "屡次违规"); err != nil {
		t.Fatalf("封禁不该失败: %v", err)
	}
	if len(f.accounts.banCalls) != 1 {
		t.Fatalf("应调用一次封禁，实际 %d 次", len(f.accounts.banCalls))
	}
	if f.accounts.banCalls[0].target != ctype.NewID(9) || f.accounts.banCalls[0].actor != adminViewer.ID {
		t.Fatalf("封禁目标 / 发起人不对: %+v", f.accounts.banCalls[0])
	}

	// 理由可选，但超长要拒绝。
	if err := f.svc.AdminBan(context.Background(), adminViewer, ctype.NewID(9), strings.Repeat("长", domain.MaxAdminReasonLen+1)); !apperr.IsCode(err, apperr.CodeCommentAdminReasonTooLong) {
		t.Fatalf("超长理由应返回 comment.admin_reason_too_long，实际 %v", err)
	}
}

// TestAdminWritesRejectNonAdmin 列表与三个写接口对普通用户一律 comment.forbidden。
func TestAdminWritesRejectNonAdmin(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ordinary := domain.Viewer{ID: ctype.NewID(7)}

	if _, _, err := f.svc.AdminList(context.Background(), ordinary, domain.AdminFilter{}, domain.AdminPage{}); !apperr.IsCode(err, apperr.CodeCommentForbidden) {
		t.Fatalf("普通用户看管理端列表应返回 comment.forbidden，实际 %v", err)
	}
	if _, err := f.svc.AdminRemoveMany(context.Background(), ordinary, []ctype.ID{ctype.NewID(91)}); !apperr.IsCode(err, apperr.CodeCommentForbidden) {
		t.Fatalf("普通用户批量删除应返回 comment.forbidden，实际 %v", err)
	}
	if _, err := f.svc.AdminMute(context.Background(), ordinary, ctype.NewID(9), 7, "刷屏"); !apperr.IsCode(err, apperr.CodeCommentForbidden) {
		t.Fatalf("普通用户禁言应返回 comment.forbidden，实际 %v", err)
	}
	if err := f.svc.AdminBan(context.Background(), ordinary, ctype.NewID(9), "违规"); !apperr.IsCode(err, apperr.CodeCommentForbidden) {
		t.Fatalf("普通用户封禁应返回 comment.forbidden，实际 %v", err)
	}

	// 被拒绝时不能有任何副作用。
	if len(f.accounts.muteCalls) != 0 || len(f.accounts.banCalls) != 0 || len(f.repo.softDeleted) != 0 {
		t.Fatalf("非管理员调用不该产生副作用: mute=%v ban=%v deleted=%v",
			f.accounts.muteCalls, f.accounts.banCalls, f.repo.softDeleted)
	}
}

// TestAdminAccountActionsNeedDisposer 账号处置端口没接时返回服务异常码（不是 nil panic）。
func TestAdminAccountActionsNeedDisposer(t *testing.T) {
	t.Parallel()

	// 直接构造一份"没注入账号端口"的服务：WithAccountDisposer 是可选装配项。
	repo := newFakeRepo()
	svc := app.New(repo, fakeDiggs{}, fakeCounters{}, fakeRelations{}, fakeUsers{},
		fakeArticles{}, &fakeCache{}, &fakeNotifier{}, fakeIDs{}, &fakeLogger{})

	if _, err := svc.AdminMute(context.Background(), adminViewer, ctype.NewID(9), 7, "刷屏"); !apperr.IsCode(err, apperr.CodeCommentAdminUnavailable) {
		t.Fatalf("未装配处置端口时禁言应返回 comment.admin_unavailable，实际 %v", err)
	}
	if err := svc.AdminBan(context.Background(), adminViewer, ctype.NewID(9), "违规"); !apperr.IsCode(err, apperr.CodeCommentAdminUnavailable) {
		t.Fatalf("未装配处置端口时封禁应返回 comment.admin_unavailable，实际 %v", err)
	}
}
