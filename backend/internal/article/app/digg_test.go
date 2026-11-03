package app_test

// 本文件是「点赞关系行口径」在用例层的契约（#96 / #235）。
//
// 为什么单独成文件：service_test.go 已经逼近单文件行数上限（A7），
// 而点赞的语义值得一组自包含的用例 —— 它们共享同一个背景（唯一索引
// 不含 deleted_at），拆出来后 service_test.go 只留编排类用例。
//
// 2026-11-04（ADR-0024 决策 3）：接口从 toggle 改成**幂等目标态**。
// 调用方说的是"我要它处于已赞 / 未赞"，服务端按目标收敛、重复同态即无操作。
// 这既是数据损坏的解药（陈旧按钮不会再把自己真实的赞取消掉），
// 也让重试成为安全操作。

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// TestDiggToggleThreeTimes 覆盖用户视角的连击路径（#96）。
//
// 断言的是"最终状态"而不是调用序列：关系行、计数增量、事件三者必须与
// 「点赞 → 取消 → 再点赞」的语义一致。
//
// 这条能红：口径不一致时（软删行不被视作"已存在"），第三次点击会撞唯一索引，
// 或者计数与关系行脱节（加了行没加计数 / 减了计数没删行）。
func TestDiggToggleThreeTimes(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	// 第 1 次：点赞（目标态 liked=true）。
	got, err := svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: true})
	if err != nil {
		t.Fatalf("第 1 次点赞失败: %v", err)
	}
	if !got.IsDigg {
		t.Fatal("第 1 次应处于已点赞")
	}
	if got.DiggCount != 1 {
		t.Fatalf("第 1 次响应里的权威计数应为 1，实际 %d", got.DiggCount)
	}
	if got := repos.FakeCounters().Get(id).Digg; got != 1 {
		t.Fatalf("第 1 次后计数应为 1，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 1 {
		t.Fatalf("第 1 次后应有 1 条关系行，实际 %d", got)
	}

	// 第 2 次：取消（目标态 liked=false）。
	got, err = svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: false})
	if err != nil {
		t.Fatalf("第 2 次（取消）失败: %v", err)
	}
	if got.IsDigg {
		t.Fatal("第 2 次应处于未点赞")
	}
	if got.DiggCount != 0 {
		t.Fatalf("取消后响应里的权威计数应为 0，实际 %d", got.DiggCount)
	}
	if got := repos.FakeCounters().Get(id).Digg; got != 0 {
		t.Fatalf("取消后计数应回到 0，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 0 {
		t.Fatalf("取消后不应有活关系行，实际 %d", got)
	}

	// 第 3 次：再点赞 —— 线上就是这一步报 duplicated key。
	got, err = svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: true})
	if err != nil {
		t.Fatalf("第 3 次（再点赞）失败: %v", err)
	}
	if !got.IsDigg {
		t.Fatal("第 3 次应处于已点赞")
	}
	if got := repos.FakeCounters().Get(id).Digg; got != 1 {
		t.Fatalf("第 3 次后计数应为 1，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 1 {
		t.Fatalf("第 3 次后应有 1 条关系行，实际 %d", got)
	}

	// 事件只在"真新增"时发：三次连击共两次真新增（第 1、3 次），取消不发。
	if names := repos.FakeOutbox().Names(); len(names) != 2 {
		t.Fatalf("两次真新增应发 2 条点赞事件，实际 %v", names)
	}
}

// TestDiggCountOnlyMovesOnRealChange 验证计数与关系行的真实变化严格一致。
//
// 这是 AC 第 2 条的可执行形式：只有真新增才 +1，只有真移除才 −1。
// 幂等重复（对已点赞的再 Add、对未点赞的再 Remove）绝不能动计数。
func TestDiggCountOnlyMovesOnRealChange(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	// 连点 5 次：赞、取消、赞、取消、赞。
	want := []bool{true, false, true, false, true}
	for i, target := range want {
		got, err := svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: target})
		if err != nil {
			t.Fatalf("第 %d 次点击失败: %v", i+1, err)
		}
		if got.IsDigg != target {
			t.Fatalf("第 %d 次应为 liked=%v，实际 %v", i+1, target, got.IsDigg)
		}
		// 计数必须等于"关系行是否处于已点赞"：1 或 0，不会累积也不会为负。
		wantCount := 0
		if target {
			wantCount = 1
		}
		if got.DiggCount != wantCount {
			t.Fatalf("第 %d 次响应的权威计数应为 %d，实际 %d", i+1, wantCount, got.DiggCount)
		}
		if got := repos.FakeCounters().Get(id).Digg; got != wantCount {
			t.Fatalf("第 %d 次后计数应为 %d，实际 %d（计数与关系行脱节）", i+1, wantCount, got)
		}
		if got := repos.FakeDiggs().Count(id); got != wantCount {
			t.Fatalf("第 %d 次后关系行应为 %d，实际 %d", i+1, wantCount, got)
		}
	}

	// 末次点击是"取消之后重新点赞"。此时那一行必须被**恢复**成活行，
	// 而不是"软删行留在原地 + 另起一行"—— 后者正是 #96 的缺陷形态：
	// 唯一键被残行占着，下一次操作必然撞键。
	if got := repos.FakeDiggs().SoftCount(id); got != 0 {
		t.Fatalf("再点赞后应把残行恢复为活行、不留软删行，实际残留 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 1 {
		t.Fatalf("再点赞后应恰好 1 条活关系行，实际 %d", got)
	}
}

// TestDiggIdempotentTargetState 钉住"幂等目标态"（#235 / ADR-0024 决策 3）。
//
// 危险场景：按钮显示「未赞」时再点一次，旧契约发的是 toggle → 后端发现已存在
// → Remove，把用户真实的点赞取消掉。新语义下"重复提交同一个目标态"必须无操作：
// 不改关系行、不改计数、不发事件，仍然返回**当前权威计数**（前端据此回填）。
func TestDiggIdempotentTargetState(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	// 第一次点赞是真实变化。
	got, err := svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: true})
	if err != nil {
		t.Fatalf("点赞失败: %v", err)
	}
	if !got.IsDigg || got.DiggCount != 1 {
		t.Fatalf("首次点赞应 liked=true / digg_count=1，实际 liked=%v count=%d", got.IsDigg, got.DiggCount)
	}
	eventsAfterFirst := len(repos.FakeOutbox().Names())

	// 第二次同态：无操作，但响应仍然是权威值（不是让前端自己猜）。
	got, err = svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: true})
	if err != nil {
		t.Fatalf("重复点赞（同态）失败: %v", err)
	}
	if !got.IsDigg || got.DiggCount != 1 {
		t.Fatalf("重复同态不应改变状态，实际 liked=%v count=%d", got.IsDigg, got.DiggCount)
	}
	if delta := repos.FakeCounters().Get(id).Digg; delta != 1 {
		t.Fatalf("重复同态不应再 +1，实际增量 %d", delta)
	}
	if names := repos.FakeOutbox().Names(); len(names) != eventsAfterFirst {
		t.Fatalf("重复同态不应再发通知，实际 %v", names)
	}

	// 取消两次同理：第一次真移除，第二次同态无操作。
	got, err = svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: false})
	if err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	if got.IsDigg || got.DiggCount != 0 {
		t.Fatalf("取消后应 liked=false / count=0，实际 liked=%v count=%d", got.IsDigg, got.DiggCount)
	}

	got, err = svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: false})
	if err != nil {
		t.Fatalf("重复取消（同态）失败: %v", err)
	}
	if got.IsDigg || got.DiggCount != 0 {
		t.Fatalf("重复取消不应改变状态，实际 liked=%v count=%d", got.IsDigg, got.DiggCount)
	}
	if delta := repos.FakeCounters().Get(id).Digg; delta != 0 {
		t.Fatalf("重复取消不应把计数减成负数，实际增量 %d", delta)
	}
	if names := repos.FakeOutbox().Names(); len(names) != eventsAfterFirst {
		t.Fatalf("重复取消不应再发通知，实际 %v", names)
	}
}

// TestDiggRejectsInvisibleArticleAndAnonymous 验证领域层规则不变（#96 硬要求 5）。
//
// 文章不可见、或操作者非登录态，都按"文章不存在"返回 —— 不泄露私有文章
// 是否存在。这条保护的是"修点赞别把可见性判断一起改坏"。
func TestDiggRejectsInvisibleArticleAndAnonymous(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusDraft)

	// 未登录。
	if _, err := svc.Digg(ctx, app.DiggCmd{Actor: domain.Anonymous(), ArticleID: id, Liked: true}); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("匿名点赞应按文章不存在返回，实际 %v", err)
	}
	// 已登录但文章不可见（草稿、非作者）。
	if _, err := svc.Digg(ctx, app.DiggCmd{Actor: strangerViewer(), ArticleID: id, Liked: true}); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("不可见文章应按文章不存在返回，实际 %v", err)
	}
	// 两种拒绝都不该产生任何副作用。
	if got := repos.FakeCounters().Get(id).Digg; got != 0 {
		t.Fatalf("被拒绝的点赞不应改计数，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 0 {
		t.Fatalf("被拒绝的点赞不应留下关系行，实际 %d", got)
	}
	if names := repos.FakeOutbox().Names(); len(names) != 0 {
		t.Fatalf("被拒绝的点赞不应发事件，实际 %v", names)
	}
}
