package app_test

// 本文件是「点赞关系行口径」在用例层的契约（#96）。
//
// 为什么单独成文件：service_test.go 已经逼近单文件行数上限（A7），
// 而点赞的语义值得一组自包含的用例 —— 它们共享同一个背景（唯一索引
// 不含 deleted_at），拆出来后 service_test.go 只留编排类用例。

import (
	"context"
	"errors"
	"testing"

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

	// 第 1 次：点赞。
	liked, err := svc.Digg(ctx, id, strangerViewer())
	if err != nil {
		t.Fatalf("第 1 次点赞失败: %v", err)
	}
	if !liked {
		t.Fatal("第 1 次应处于已点赞")
	}
	if got := repos.FakeCounters().Get(id).Digg; got != 1 {
		t.Fatalf("第 1 次后计数应为 1，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 1 {
		t.Fatalf("第 1 次后应有 1 条关系行，实际 %d", got)
	}

	// 第 2 次：取消。
	liked, err = svc.Digg(ctx, id, strangerViewer())
	if err != nil {
		t.Fatalf("第 2 次（取消）失败: %v", err)
	}
	if liked {
		t.Fatal("第 2 次应处于未点赞")
	}
	if got := repos.FakeCounters().Get(id).Digg; got != 0 {
		t.Fatalf("取消后计数应回到 0，实际 %d", got)
	}
	if got := repos.FakeDiggs().Count(id); got != 0 {
		t.Fatalf("取消后不应有活关系行，实际 %d", got)
	}

	// 第 3 次：再点赞 —— 线上就是这一步报 duplicated key。
	liked, err = svc.Digg(ctx, id, strangerViewer())
	if err != nil {
		t.Fatalf("第 3 次（再点赞）失败: %v", err)
	}
	if !liked {
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
	counters := repos.FakeCounters()
	for i, expect := range want {
		liked, err := svc.Digg(ctx, id, strangerViewer())
		if err != nil {
			t.Fatalf("第 %d 次点击失败: %v", i+1, err)
		}
		if liked != expect {
			t.Fatalf("第 %d 次应为 liked=%v，实际 %v", i+1, expect, liked)
		}
		// 计数必须等于"关系行是否处于已点赞"：1 或 0，不会累积也不会为负。
		wantCount := 0
		if expect {
			wantCount = 1
		}
		if got := counters.Get(id).Digg; got != wantCount {
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
	if _, err := svc.Digg(ctx, id, domain.Anonymous()); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("匿名点赞应按文章不存在返回，实际 %v", err)
	}
	// 已登录但文章不可见（草稿、非作者）。
	if _, err := svc.Digg(ctx, id, strangerViewer()); !errors.Is(err, domain.ErrArticleNotFound) {
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
