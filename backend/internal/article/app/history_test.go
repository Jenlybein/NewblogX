package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
)

// ── 浏览历史列表：游标分页 ──

// TestListViewHistoryCursorHasMore 验证"多取一条判断 has_more"的分页口径。
//
// 游标分页没有 total（count 也答不了"下一页还有没有"），所以 has_more 只能靠
// 多取一条：取 limit+1 条，多出来的那条被切掉并置 HasMore。写错了表现是
// "下一页按钮永远不出现"或"最后一页后再翻一次却拿到上一页的重复数据"。
func TestListViewHistoryCursorHasMore(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

	// 三条记录，时间倒序：id=3 最新。
	for i, id := range []uint64{1, 2, 3} {
		if err := repos.FakeHistory().Upsert(ctx,
			viewhistory.NewRecord(strangerViewer().UserID, domain.ArticleID(ctype.NewID(id)), base.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatalf("预置浏览历史失败: %v", err)
		}
	}

	first, err := svc.ListViewHistory(ctx, strangerViewer(), app.ViewHistoryQuery{Limit: 2}, false)
	if err != nil {
		t.Fatalf("查历史第 1 页失败: %v", err)
	}
	if len(first.Records) != 2 ||
		first.Records[0].ArticleID().Uint64() != 3 || first.Records[1].ArticleID().Uint64() != 2 {
		t.Fatalf("第 1 页应为 [3 2]，实际 %v", recordsIDs(first.Records))
	}
	if !first.HasMore {
		t.Fatal("还有第 3 条时 has_more 应为 true")
	}
	// 查询层收到的 limit 应是 limit+1（多取一条），游标是第一页的 nil。
	if got := repos.FakeHistory().LastListQuery(); got.Limit != 3 || got.Before != nil {
		t.Fatalf("查询应为 limit=3（多取一条）且无游标，实际 limit=%d before=%v", got.Limit, got.Before)
	}

	// 用第 1 页最后一条作游标续页。
	last := first.Records[len(first.Records)-1]
	second, err := svc.ListViewHistory(ctx, strangerViewer(), app.ViewHistoryQuery{
		Limit:  2,
		Before: &viewhistory.Cursor{ViewedAt: last.ViewedAt(), ArticleID: last.ArticleID()},
	}, false)
	if err != nil {
		t.Fatalf("查历史第 2 页失败: %v", err)
	}
	if len(second.Records) != 1 || second.Records[0].ArticleID().Uint64() != 1 {
		t.Fatalf("第 2 页应为 [1]，实际 %v", recordsIDs(second.Records))
	}
	if second.HasMore {
		t.Fatal("末页 has_more 应为 false")
	}

	// 空用户：空列表 + 不再翻页。
	empty, err := svc.ListViewHistory(ctx, authorViewer(), app.ViewHistoryQuery{Limit: 2}, false)
	if err != nil {
		t.Fatalf("查空历史失败: %v", err)
	}
	if len(empty.Records) != 0 || empty.HasMore {
		t.Fatalf("空用户应返回空列表且 has_more=false，实际 %d 条 has_more=%v", len(empty.Records), empty.HasMore)
	}
}

// TestListViewHistoryCouplesKeywordAndVisibilityFilter 验证 keyword 收敛与读过滤开关下传。
//
// 历史页的搜索是服务端 keyword（按标题），与「显示非管理员发布文章」开关一样
// 必须落到查询层才有意义：漏传就是"搜了标题却搜不到"或"开关关了照样出现"。
func TestListViewHistoryCouplesKeywordAndVisibilityFilter(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.ListViewHistory(ctx, strangerViewer(), app.ViewHistoryQuery{
		Keyword: "  Go 并发  ",
		Limit:   10,
	}, true); err != nil {
		t.Fatalf("查历史失败: %v", err)
	}

	got := repos.FakeHistory().LastListQuery()
	if got.Keyword != "Go 并发" {
		t.Fatalf("关键词应去首尾空白后下传，实际 %q", got.Keyword)
	}
	if !got.OnlyAdminAuthors {
		t.Fatal("读过滤开关应原样下传（true）")
	}
	if got.UserID != stranger {
		t.Fatalf("查询的用户应来自 viewer（%s），实际 %s", stranger, got.UserID)
	}
}

// TestListViewHistoryRequiresLogin 验证未登录不给历史（历史是私人数据）。
func TestListViewHistoryRequiresLogin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.ListViewHistory(ctx, domain.Anonymous(), app.ViewHistoryQuery{Limit: 10}, false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("匿名查历史应 403，实际 %v", err)
	}
	if calls := repos.FakeHistory().Calls("List"); calls != 0 {
		t.Fatalf("未登录不应触达仓储，实际调用 %d 次", calls)
	}
}

// ── 清空历史 ──

// TestClearViewHistory 验证清空历史：只清自己、二次点击幂等、未登录拒绝。
func TestClearViewHistory(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

	// 两条属于 stranger，一条属于 author —— 清空不能波及别人。
	for i, spec := range []struct {
		user domain.UserID
		id   uint64
	}{{stranger, 1}, {stranger, 2}, {author, 3}} {
		if err := repos.FakeHistory().Upsert(ctx,
			viewhistory.NewRecord(spec.user, domain.ArticleID(ctype.NewID(spec.id)), base.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatalf("预置浏览历史失败: %v", err)
		}
	}

	if err := svc.ClearViewHistory(ctx, strangerViewer()); err != nil {
		t.Fatalf("清空历史失败: %v", err)
	}

	left, err := svc.ListViewHistory(ctx, strangerViewer(), app.ViewHistoryQuery{Limit: 10}, false)
	if err != nil {
		t.Fatalf("清空后查历史失败: %v", err)
	}
	if len(left.Records) != 0 || left.HasMore {
		t.Fatalf("清空后应为空列表，实际 %v", recordsIDs(left.Records))
	}

	// 别人的历史不受影响。
	others, err := svc.ListViewHistory(ctx, authorViewer(), app.ViewHistoryQuery{Limit: 10}, false)
	if err != nil {
		t.Fatalf("查别人的历史失败: %v", err)
	}
	if len(others.Records) != 1 || others.Records[0].ArticleID().Uint64() != 3 {
		t.Fatalf("清空只应动自己的历史，作者应仍有 [3]，实际 %v", recordsIDs(others.Records))
	}

	// 幂等：再清一次不报错。
	if err := svc.ClearViewHistory(ctx, strangerViewer()); err != nil {
		t.Fatalf("重复清空应幂等，实际 %v", err)
	}

	if err := svc.ClearViewHistory(ctx, domain.Anonymous()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("匿名清空应 403，实际 %v", err)
	}
}

// recordsIDs 把记录压成 ID 串，便于断言顺序。
func recordsIDs(records []*viewhistory.Record) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r.ArticleID().String())
	}
	return out
}
