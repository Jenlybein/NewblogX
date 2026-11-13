package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件守合集 / 项目分组的三条口径（ADR-0019）：
//  1. 分组 CRUD 立即生效（不参与拖拽防抖）；
//  2. 整树换序是幂等整份覆盖，且集合不一致时整单拒绝；
//  3. 删分组只删编排 —— 条目退回未分组，合集 / 项目本身不动。

// newGroupFixture 复用既有 newService（同一套 fake 与 ID 序列），
// 只是把"管理员"这个常用视角取出来。
func newGroupFixture(t *testing.T) (*app.Service, *apptest.Repos, domain.Viewer) {
	t.Helper()
	svc, repos, _ := newService(t)
	return svc, repos, adminViewer()
}

func ownerID(n uint64) ctype.ID { return ctype.NewID(n) }

// TestSaveGroupIsImmediate 锁住"建分组立即生效、排在末尾"。
func TestSaveGroupIsImmediate(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	first, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "  第一批  ",
	})
	if err != nil {
		t.Fatalf("新建分组失败: %v", err)
	}
	// 标题去空白：用户输入里的空格不该进库。
	if first.Title() != "第一批" {
		t.Fatalf("标题应去空白，实际 %q", first.Title())
	}

	second, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "第二批",
	})
	if err != nil {
		t.Fatalf("新建第二个分组失败: %v", err)
	}
	if second.SortOrder() <= first.SortOrder() {
		t.Fatalf("新建应排在末尾：first=%d second=%d", first.SortOrder(), second.SortOrder())
	}

	groups, err := repos.FakeGroups().ListGroups(ctx)
	if err != nil {
		t.Fatalf("列分组失败: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("应有 2 个分组，实际 %d", len(groups))
	}
}

// TestSaveGroupRenamesWithoutTouchingOrder 锁住"改名不动顺序"。
func TestSaveGroupRenamesWithoutTouchingOrder(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	g, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "旧名",
	})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	before := g.SortOrder()

	renamed, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, ID: g.ID(), Title: "新名",
	})
	if err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if renamed.Title() != "新名" {
		t.Fatalf("标题应更新，实际 %q", renamed.Title())
	}
	if renamed.SortOrder() != before {
		t.Fatalf("改名不该动顺序：before=%d after=%d", before, renamed.SortOrder())
	}
	_ = repos
}

// TestSaveGroupRejectsEmptyTitle 锁住标题非空。
func TestSaveGroupRejectsEmptyTitle(t *testing.T) {
	svc, _, admin := newGroupFixture(t)
	_, err := svc.SaveGroup(context.Background(), app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "   ",
	})
	if !apperr.IsCode(err, apperr.CodeGroupTitleRequired) {
		t.Fatalf("空标题应被拒绝，实际 %v", err)
	}
}

// TestDeleteGroupMovesItemsToUngrouped 是这票最关键的一条：
// 删分组**不删合集 / 项目**，条目退回未分组。
func TestDeleteGroupMovesItemsToUngrouped(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	g, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "待删分组",
	})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	// 归属行指向真实存在的合集：读面会滤掉"实体已删、归属行还在"的幽灵 ID。
	for _, id := range []uint64{11, 12} {
		seedCollectionForGrouping(t, repos, id)
	}
	repos.FakeGroups().SeedAssignment(ownerID(11), g.ID(), 1)
	repos.FakeGroups().SeedAssignment(ownerID(12), g.ID(), 2)

	if err := svc.DeleteGroup(ctx, app.DeleteGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, ID: g.ID(),
	}); err != nil {
		t.Fatalf("删除分组失败: %v", err)
	}

	tree, err := svc.LoadGroupTree(ctx, admin, outline.OwnerCollection)
	if err != nil {
		t.Fatalf("读分组树失败: %v", err)
	}
	if len(tree.Groups) != 0 {
		t.Fatalf("分组应已删除，实际还剩 %d 个", len(tree.Groups))
	}
	if len(tree.Ungrouped) != 2 {
		t.Fatalf("两条条目应退回未分组，实际 %d", len(tree.Ungrouped))
	}
}

// TestReplaceGroupOrderIsIdempotent 锁住整树换序的幂等与顺序语义。
func TestReplaceGroupOrderIsIdempotent(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	g1, err := svc.SaveGroup(ctx, app.SaveGroupCmd{Actor: admin, OwnerKind: outline.OwnerCollection, Title: "G1"})
	if err != nil {
		t.Fatalf("新建 G1 失败: %v", err)
	}
	g2, err := svc.SaveGroup(ctx, app.SaveGroupCmd{Actor: admin, OwnerKind: outline.OwnerCollection, Title: "G2"})
	if err != nil {
		t.Fatalf("新建 G2 失败: %v", err)
	}
	// 条目全集取自**实体**全集，所以三个合集必须先真的存在。
	for _, id := range []uint64{21, 22, 23} {
		seedCollectionForGrouping(t, repos, id)
		repos.FakeGroups().SeedAssignment(ownerID(id), "", 0)
	}

	tree := grouping.TreeOrder{
		Groups: []grouping.GroupOrder{
			{ID: g2.ID(), Items: []ctype.ID{ownerID(23), ownerID(22)}},
			{ID: g1.ID(), Items: []ctype.ID{ownerID(21)}},
		},
		Ungrouped: nil,
	}
	if err := svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Tree: tree,
	}); err != nil {
		t.Fatalf("整树换序失败: %v", err)
	}

	// 同一份输入再提交一次：结果必须相同（幂等）。
	if err := svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Tree: tree,
	}); err != nil {
		t.Fatalf("重复提交应幂等，实际报错: %v", err)
	}

	read, err := svc.LoadGroupTree(ctx, admin, outline.OwnerCollection)
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(read.Groups) != 2 {
		t.Fatalf("应有 2 个分组，实际 %d", len(read.Groups))
	}
	// 顺序以提交为准：G2 在前。
	if read.Groups[0].ID != g2.ID() {
		t.Fatalf("第一个分组应是 G2，实际 %s", read.Groups[0].ID)
	}
	// 组内顺序以提交为准：23 在 22 前面。
	if len(read.Groups[0].Items) != 2 || read.Groups[0].Items[0] != ownerID(23) {
		t.Fatalf("G2 组内顺序应为 [23 22]，实际 %v", read.Groups[0].Items)
	}
}

// TestReplaceGroupOrderRejectsStaleInput 锁住集合不一致时整单拒绝。
//
// 后台拿到的是上一次读到的列表：别人刚建或刚删一个分组后，宽松的"缺几个也照写"
// 会把那条静默排丢。
func TestReplaceGroupOrderRejectsStaleInput(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	g, err := svc.SaveGroup(ctx, app.SaveGroupCmd{Actor: admin, OwnerKind: outline.OwnerCollection, Title: "G"})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	// 条目全集取自实体全集：先让这个合集真的存在，再给它一条未分组归属。
	seedCollectionForGrouping(t, repos, 31)
	repos.FakeGroups().SeedAssignment(ownerID(31), "", 0)

	// 提交里漏掉了一个分组。
	err = svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection,
		Tree: grouping.TreeOrder{Ungrouped: []ctype.ID{ownerID(31)}},
	})
	if !apperr.IsCode(err, apperr.CodeGroupOrderMismatch) {
		t.Fatalf("缺分组应整单拒绝，实际 %v", err)
	}

	// 提交里漏掉了一个条目。
	err = svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection,
		Tree: grouping.TreeOrder{Groups: []grouping.GroupOrder{{ID: g.ID()}}},
	})
	if !apperr.IsCode(err, apperr.CodeGroupOrderMismatch) {
		t.Fatalf("漏条目应整单拒绝，实际 %v", err)
	}
}

// TestGroupMutationsRequireAdmin 锁住三条写用例的管理员门禁。
func TestGroupMutationsRequireAdmin(t *testing.T) {
	svc, _, _ := newGroupFixture(t)
	ctx := context.Background()
	viewer := domain.Anonymous()

	if _, err := svc.SaveGroup(ctx, app.SaveGroupCmd{Actor: viewer, OwnerKind: outline.OwnerCollection, Title: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员新建应被拒绝，实际 %v", err)
	}
	if err := svc.DeleteGroup(ctx, app.DeleteGroupCmd{Actor: viewer, OwnerKind: outline.OwnerCollection, ID: ownerID(1)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员删除应被拒绝，实际 %v", err)
	}
	if err := svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{Actor: viewer, OwnerKind: outline.OwnerCollection}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员换序应被拒绝，实际 %v", err)
	}
	if _, err := svc.LoadGroupTree(ctx, viewer, outline.OwnerCollection); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员读分组树应被拒绝，实际 %v", err)
	}
}

// TestReplaceGroupOrderFirstDragWithNoAssignmentRows 是评审抓到的阻塞性 bug 的回归。
//
// 从来没有被拖进过分组的实体**没有归属行**。如果校验用的"全集"取自归属行，
// 后台第一次拖动就会因为"提交里多了一个库不知道的条目"被整单拒绝 ——
// 也就是这个功能在干净数据上根本用不了。
func TestReplaceGroupOrderFirstDragWithNoAssignmentRows(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	// 两个合集已经在库（ListAdminCards 能看到），但一条归属行都没有。
	seedCollectionForGrouping(t, repos, 901)
	seedCollectionForGrouping(t, repos, 902)

	g, err := svc.SaveGroup(ctx, app.SaveGroupCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection, Title: "第一次分组",
	})
	if err != nil {
		t.Fatalf("新建分组失败: %v", err)
	}

	// 第一次拖动：把一个合集拖进分组，另一个留在未分组。
	if err := svc.ReplaceGroupOrder(ctx, app.ReplaceGroupOrderCmd{
		Actor: admin, OwnerKind: outline.OwnerCollection,
		Tree: grouping.TreeOrder{
			Groups:    []grouping.GroupOrder{{ID: g.ID(), Items: []ctype.ID{ownerID(901)}}},
			Ungrouped: []ctype.ID{ownerID(902)},
		},
	}); err != nil {
		t.Fatalf("首次拖动不该被拒绝，实际: %v", err)
	}

	tree, err := svc.LoadGroupTree(ctx, admin, outline.OwnerCollection)
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(tree.Groups) != 1 || len(tree.Groups[0].Items) != 1 {
		t.Fatalf("分组里应有 1 个合集，实际 %+v", tree.Groups)
	}
	if len(tree.Ungrouped) != 1 {
		t.Fatalf("未分组应剩 1 个合集，实际 %d", len(tree.Ungrouped))
	}
}

// TestLoadGroupTreeIncludesEntitiesWithoutAssignmentRows 锁住读模型：
// 新建但还没进过分组的实体必须出现在未分组里（不能依赖前端兜底）。
func TestLoadGroupTreeIncludesEntitiesWithoutAssignmentRows(t *testing.T) {
	svc, repos, admin := newGroupFixture(t)
	ctx := context.Background()

	seedCollectionForGrouping(t, repos, 911)

	tree, err := svc.LoadGroupTree(ctx, admin, outline.OwnerCollection)
	if err != nil {
		t.Fatalf("读分组树失败: %v", err)
	}
	if len(tree.Ungrouped) != 1 || tree.Ungrouped[0] != ownerID(911) {
		t.Fatalf("没有归属行的合集应出现在未分组，实际 %+v", tree.Ungrouped)
	}
}

// seedCollectionForGrouping 往 fake 里放一个合集（分组用例只关心"它存在"）。
func seedCollectionForGrouping(t *testing.T, repos *apptest.Repos, id uint64) {
	t.Helper()
	c, err := collection.New(collection.NewParams{
		ID:          collection.ID(ctype.NewID(id)),
		Title:       "合集",
		Description: "描述",
		CreatorID:   domain.UserID(ctype.NewID(1)),
		Displayed:   true,
		Now:         apptest.DefaultTime,
	})
	if err != nil {
		t.Fatalf("构造合集失败: %v", err)
	}
	repos.FakeCollections().SeedCollection(c)
}
