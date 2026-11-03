package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「项目」管理端的用例：增删改、收录 / 批量收录 / 移除、两级排序、
// 失效行读面与一键清理、权限线。
//
// 断言的都是**外部可观察的行为**：读者读面看到什么、后台读面看到什么、
// 关联行还在不在、第二次清理返回几。与合集同一批断言，因为两者是镜像实现；
// 两套用例各自独立，不靠"合集绿了所以项目也绿"。

// createProject 是"管理员建一个项目"的简写。
func createProject(t *testing.T, svc *app.Service, title string) project.ID {
	t.Helper()
	id, err := svc.CreateProject(context.Background(), app.CreateProjectCmd{
		Actor:       adminViewer(),
		Title:       title,
		Description: "项目描述",
		Cover:       "",
		Displayed:   true,
	})
	if err != nil {
		t.Fatalf("创建项目失败: %v", err)
	}
	return id
}

// includeProjectArticle 是"管理员把一篇文章收录进项目"的简写。
func includeProjectArticle(t *testing.T, svc *app.Service, projectID project.ID, articleID domain.ArticleID) {
	t.Helper()
	if err := svc.IncludeProjectArticle(context.Background(), app.IncludeProjectArticleCmd{
		Actor:     adminViewer(),
		ProjectID: projectID,
		ArticleID: articleID,
	}); err != nil {
		t.Fatalf("收录文章失败: %v", err)
	}
}

// cleanupStaleProject 是"管理员一键清理失效收录"的简写。
func cleanupStaleProject(t *testing.T, svc *app.Service, projectID project.ID) int {
	t.Helper()
	removed, err := svc.CleanupStaleProjectArticles(context.Background(), adminViewer(), projectID)
	if err != nil {
		t.Fatalf("清理失效行失败: %v", err)
	}
	return removed
}

// TestCreateProjectRequiresAdmin 项目是管理员专属运营产物，作者不参与创建。
func TestCreateProjectRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		_, err := svc.CreateProject(context.Background(), app.CreateProjectCmd{
			Actor:     viewer,
			Title:     "普通用户想建项目",
			Displayed: true,
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 建项目应被拒绝，实际 %v", name, err)
		}
	}
}

// TestCreateProjectRejectsEmptyTitle 标题非空是项目唯一的不变量。
func TestCreateProjectRejectsEmptyTitle(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	_, err := svc.CreateProject(context.Background(), app.CreateProjectCmd{
		Actor:     adminViewer(),
		Title:     "   ",
		Displayed: true,
	})
	if !apperr.IsCode(err, apperr.CodeProjectTitleRequired) {
		t.Fatalf("空标题应被拒绝，实际 %v", err)
	}
}

// TestCreateProjectAppendsToEnd 新建的项目排在末尾，列表按顺序返回。
func TestCreateProjectAppendsToEnd(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	first := createProject(t, svc, "重构实战")
	second := createProject(t, svc, "分布式入门")

	cards, total, err := svc.ListProjectCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 2 || len(cards) != 2 || cards[0].ID != first || cards[1].ID != second {
		t.Fatalf("项目顺序不对: total=%d cards=%+v", total, cards)
	}

	adminCards, err := svc.ListAdminProjectCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	if len(adminCards) != 2 || adminCards[0].SortOrder != 1 || adminCards[1].SortOrder != 2 {
		t.Fatalf("后台顺序号应从 1 连续编号，实际 %+v", adminCards)
	}
}

// TestUpdateProject 改内容不顺手覆盖展示开关。
func TestUpdateProject(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	id := createProject(t, svc, "旧标题")
	if err := svc.SetProjectDisplay(context.Background(), app.SetProjectDisplayCmd{
		Actor:     adminViewer(),
		ProjectID: id,
		Displayed: false,
	}); err != nil {
		t.Fatalf("关闭展示失败: %v", err)
	}

	if err := svc.UpdateProject(context.Background(), app.UpdateProjectCmd{
		Actor:       adminViewer(),
		ProjectID:   id,
		Title:       "  新标题  ",
		Description: "新描述",
		Cover:       "https://cdn.example/cover.png",
	}); err != nil {
		t.Fatalf("改项目失败: %v", err)
	}

	adminCards, err := svc.ListAdminProjectCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	if len(adminCards) != 1 {
		t.Fatalf("后台应看到 1 个项目，实际 %d", len(adminCards))
	}
	got := adminCards[0]
	if got.Title != "新标题" || got.Description != "新描述" || got.Cover != "https://cdn.example/cover.png" {
		t.Fatalf("改项目后字段不对: %+v", got)
	}
	if got.Displayed {
		t.Fatalf("改内容不应顺手打开展示开关: %+v", got)
	}

	// 非管理员不能改。
	if err := svc.UpdateProject(context.Background(), app.UpdateProjectCmd{
		Actor: authorViewer(), ProjectID: id, Title: "偷偷改",
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员改项目应被拒绝，实际 %v", err)
	}
}

// TestDeleteProjectHidesFromAdminList 软删：后台列表与读者列表都不再出现，收录行保留。
func TestDeleteProjectHidesFromAdminList(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "待删项目")
	a := seedPublished(t, repos, 71)
	includeProjectArticle(t, svc, id, a)

	if err := svc.DeleteProject(context.Background(), app.DeleteProjectCmd{
		Actor: adminViewer(), ProjectID: id,
	}); err != nil {
		t.Fatalf("删项目失败: %v", err)
	}

	adminCards, err := svc.ListAdminProjectCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	if len(adminCards) != 0 {
		t.Fatalf("删掉的项目不应出现在后台列表，实际 %+v", adminCards)
	}
	cards, total, err := svc.ListProjectCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("删掉的项目不应出现在读者列表，实际 total=%d", total)
	}
	// 文章本身不受影响。
	if _, err := repos.FakeArticles().Find(context.Background(), a); err != nil {
		t.Fatalf("删项目不应删文章，实际 %v", err)
	}
	// 改 / 删已删项目按不存在处理。
	if err := svc.DeleteProject(context.Background(), app.DeleteProjectCmd{
		Actor: adminViewer(), ProjectID: id,
	}); !apperr.IsCode(err, apperr.CodeProjectNotFound) {
		t.Fatalf("重复删应返回项目不存在，实际 %v", err)
	}
}

// TestIncludeProjectArticleAppendAndIdempotent 收录排在末尾；重复收录不产生第二行、不改序号。
func TestIncludeProjectArticleAppendAndIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "重构实战")

	a1 := seedPublished(t, repos, 11)
	a2 := seedPublished(t, repos, 12)
	includeProjectArticle(t, svc, id, a1)
	includeProjectArticle(t, svc, id, a2)

	// 再收一次第一篇：既不能报重复，也不能把它挪到末尾。
	if err := svc.IncludeProjectArticle(context.Background(), app.IncludeProjectArticleCmd{
		Actor:     adminViewer(),
		ProjectID: id,
		ArticleID: a1,
	}); !apperr.IsCode(err, apperr.CodeProjectArticleDuplicated) {
		t.Fatalf("重复收录应报【已在项目中】，实际 %v", err)
	}

	ids, err := repos.FakeProjects().ListItemIDs(context.Background(), id)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != a1 || ids[1] != a2 {
		t.Fatalf("收录顺序不对: %v", ids)
	}
}

// TestIncludeProjectRejectsNonPublicArticle 只有"已发布且可见"的文章能被收录。
func TestIncludeProjectRejectsNonPublicArticle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status domain.Status
		vis    domain.Visibility
	}{
		{"草稿", domain.StatusDraft, domain.VisibilityVisible},
		{"审核中", domain.StatusExamining, domain.VisibilityVisible},
		{"作者隐藏", domain.StatusPublished, domain.VisibilityUserHidden},
		{"管理员隐藏", domain.StatusPublished, domain.VisibilityAdminHidden},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			svc, repos, _ := newService(t)
			id := createProject(t, svc, "主题项目")
			articleID := domain.ArticleID(ctype.NewID(21))
			repos.FakeArticles().Seed(apptest.NewArticle().
				ID(articleID).Author(author).Status(c.status).Visibility(c.vis).Build())

			err := svc.IncludeProjectArticle(context.Background(), app.IncludeProjectArticleCmd{
				Actor:     adminViewer(),
				ProjectID: id,
				ArticleID: articleID,
			})
			if !apperr.IsCode(err, apperr.CodeProjectArticleNotIncludable) {
				t.Fatalf("%s 的文章应不可收录，实际 %v", c.name, err)
			}
			ids, err := repos.FakeProjects().ListItemIDs(context.Background(), id)
			if err != nil {
				t.Fatalf("读收录失败: %v", err)
			}
			if len(ids) != 0 {
				t.Fatalf("被拒绝的文章不应落库: %v", ids)
			}
		})
	}
}

// TestIncludeProjectBatchReportsPerArticleOutcome 批量收录一次提交多篇，逐篇分类。
func TestIncludeProjectBatchReportsPerArticleOutcome(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "分布式入门六讲")

	first := seedPublished(t, repos, 101)
	second := seedPublished(t, repos, 102)
	third := seedPublished(t, repos, 103)
	hidden := domain.ArticleID(ctype.NewID(104))
	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(hidden).Author(author).Status(domain.StatusPublished).
		Visibility(domain.VisibilityUserHidden).Build())
	missing := domain.ArticleID(ctype.NewID(999))

	includeProjectArticle(t, svc, id, first)

	results, err := svc.IncludeProjectArticles(context.Background(), app.IncludeProjectArticlesCmd{
		Actor:     adminViewer(),
		ProjectID: id,
		ArticleIDs: []domain.ArticleID{
			second, first, hidden, missing, third, first,
		},
	})
	if err != nil {
		t.Fatalf("批量收录失败: %v", err)
	}

	want := []struct {
		id      domain.ArticleID
		outcome project.IncludeOutcome
	}{
		{second, project.IncludeOutcomeIncluded},
		{first, project.IncludeOutcomeDuplicated},
		{hidden, project.IncludeOutcomeNotIncludable},
		{missing, project.IncludeOutcomeMissing},
		{third, project.IncludeOutcomeIncluded},
		{first, project.IncludeOutcomeDuplicated},
	}
	if len(results) != len(want) {
		t.Fatalf("结果应逐篇返回（%d 条），实际 %d 条: %+v", len(want), len(results), results)
	}
	for i, w := range want {
		if results[i].ArticleID != w.id || results[i].Outcome != w.outcome {
			t.Fatalf("第 %d 条结果应为 (%s, %s)，实际 (%s, %s)",
				i, w.id, w.outcome, results[i].ArticleID, results[i].Outcome)
		}
	}

	ids, err := repos.FakeProjects().ListItemIDs(context.Background(), id)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 3 || ids[0] != first || ids[1] != second || ids[2] != third {
		t.Fatalf("批量收录后的顺序不对: %v", ids)
	}
}

// TestIncludeProjectBatchRequiresAdmin 批量收录与单篇同一条权限线，被拒绝时一篇都不能落库。
func TestIncludeProjectBatchRequiresAdmin(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "主题项目")
	a1 := seedPublished(t, repos, 121)
	a2 := seedPublished(t, repos, 122)

	_, err := svc.IncludeProjectArticles(context.Background(), app.IncludeProjectArticlesCmd{
		Actor:      authorViewer(),
		ProjectID:  id,
		ArticleIDs: []domain.ArticleID{a1, a2},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员批量收录应被拒绝，实际 %v", err)
	}
	if ids, _ := repos.FakeProjects().ListItemIDs(context.Background(), id); len(ids) != 0 {
		t.Fatalf("被拒绝的批量收录不应落库: %v", ids)
	}
}

// TestRemoveProjectArticleCompactsOrder 移除后剩余序号压实成 1..n，不留洞。
// 移除幂等：不在项目里也成功。
func TestRemoveProjectArticleCompactsOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "主题项目")
	a1 := seedPublished(t, repos, 41)
	a2 := seedPublished(t, repos, 42)
	a3 := seedPublished(t, repos, 43)
	includeProjectArticle(t, svc, id, a1)
	includeProjectArticle(t, svc, id, a2)
	includeProjectArticle(t, svc, id, a3)

	if err := svc.RemoveProjectArticle(context.Background(), app.RemoveProjectArticleCmd{
		Actor: adminViewer(), ProjectID: id, ArticleID: a2,
	}); err != nil {
		t.Fatalf("移除收录失败: %v", err)
	}

	ids, err := repos.FakeProjects().ListItemIDs(context.Background(), id)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != project.ArticleID(a1) || ids[1] != project.ArticleID(a3) {
		t.Fatalf("移除后剩余顺序不对: %v", ids)
	}

	// 幂等 + 权限。
	if err := svc.RemoveProjectArticle(context.Background(), app.RemoveProjectArticleCmd{
		Actor: adminViewer(), ProjectID: id, ArticleID: domain.ArticleID(ctype.NewID(999)),
	}); err != nil {
		t.Fatalf("移除不存在的收录应幂等成功，实际 %v", err)
	}
	if err := svc.RemoveProjectArticle(context.Background(), app.RemoveProjectArticleCmd{
		Actor: authorViewer(), ProjectID: id, ArticleID: a1,
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员移除应被拒绝，实际 %v", err)
	}
}

// TestReaderListHidesUndisplayedProject 未展示的项目对读者不可见，后台仍可见。
func TestReaderListHidesUndisplayedProject(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	id := createProject(t, svc, "还没公布的项目")
	if err := svc.SetProjectDisplay(context.Background(), app.SetProjectDisplayCmd{
		Actor: adminViewer(), ProjectID: id, Displayed: false,
	}); err != nil {
		t.Fatalf("关闭展示失败: %v", err)
	}

	cards, total, err := svc.ListProjectCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读项目列表失败: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("未展示的项目不应出现在读者列表，实际 total=%d len=%d", total, len(cards))
	}
	if _, err := svc.ProjectDetail(context.Background(), id, false); !apperr.IsCode(err, apperr.CodeProjectNotFound) {
		t.Fatalf("未展示的项目对读者应按不存在处理，实际 %v", err)
	}

	adminCards, err := svc.ListAdminProjectCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	if len(adminCards) != 1 || adminCards[0].Displayed {
		t.Fatalf("后台应看到未展示的项目，实际 %+v", adminCards)
	}

	if err := svc.SetProjectDisplay(context.Background(), app.SetProjectDisplayCmd{
		Actor: authorViewer(), ProjectID: id, Displayed: true,
	}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员切展示应被拒绝，实际 %v", err)
	}
}

// TestReplaceProjectArticleOrder 项目内手工排序：读者详情按新顺序返回，
// 集合不一致整单拒绝，非管理员被拒。
func TestReplaceProjectArticleOrder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "主题项目")
	a1 := seedPublished(t, repos, 51)
	a2 := seedPublished(t, repos, 52)
	a3 := seedPublished(t, repos, 53)
	includeProjectArticle(t, svc, id, a1)
	includeProjectArticle(t, svc, id, a2)
	includeProjectArticle(t, svc, id, a3)
	seedReaderArticleNodes(t, repos, outline.OwnerProject, outline.ID(id),
		ctype.ID(a1), ctype.ID(a2), ctype.ID(a3))

	if err := svc.ReplaceProjectArticleOrder(context.Background(), adminViewer(), id,
		[]project.ArticleID{project.ArticleID(a3), project.ArticleID(a1), project.ArticleID(a2)}); err != nil {
		t.Fatalf("项目内换序失败: %v", err)
	}

	// 集合不一致（漏一篇）整单拒绝，顺序不动。
	before, _ := repos.FakeProjects().ListItemIDs(context.Background(), id)
	if err := svc.ReplaceProjectArticleOrder(context.Background(), adminViewer(), id,
		[]project.ArticleID{project.ArticleID(a3), project.ArticleID(a1)}); !apperr.IsCode(err, apperr.CodeProjectOrderMismatch) {
		t.Fatalf("集合不一致应报顺序冲突，实际 %v", err)
	}
	if after, _ := repos.FakeProjects().ListItemIDs(context.Background(), id); !sameIDs(before, after) {
		t.Fatalf("被拒绝的换序不应改动数据: %v → %v", before, after)
	}

	if err := svc.ReplaceProjectArticleOrder(context.Background(), authorViewer(), id,
		[]project.ArticleID{project.ArticleID(a1), project.ArticleID(a3), project.ArticleID(a2)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员项目内换序应被拒绝，实际 %v", err)
	}
}

// TestReplaceProjectSortOrderReordersReaderList 项目之间的手工排序：
// 后台读面与读者列表都按新的 sort_order 返回。
func TestReplaceProjectSortOrderReordersReaderList(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	first := createProject(t, svc, "第一个")
	second := createProject(t, svc, "第二个")
	third := createProject(t, svc, "第三个")

	if err := svc.ReplaceProjectSortOrder(context.Background(), adminViewer(),
		[]project.ID{third, first, second}); err != nil {
		t.Fatalf("项目换序失败: %v", err)
	}

	cards, total, err := svc.ListProjectCards(context.Background(), false, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("读读者列表失败: %v", err)
	}
	if total != 3 || len(cards) != 3 {
		t.Fatalf("应有 3 个项目，实际 total=%d len=%d", total, len(cards))
	}
	got := []project.ID{cards[0].ID, cards[1].ID, cards[2].ID}
	if got[0] != third || got[1] != first || got[2] != second {
		t.Fatalf("读者列表顺序不对: %v", got)
	}

	adminCards, err := svc.ListAdminProjectCards(context.Background(), adminViewer())
	if err != nil {
		t.Fatalf("读后台项目列表失败: %v", err)
	}
	for i, id := range []project.ID{third, first, second} {
		if adminCards[i].ID != id || adminCards[i].SortOrder != i+1 {
			t.Fatalf("后台第 %d 行应为 (%s, %d)，实际 (%s, %d)",
				i+1, id, i+1, adminCards[i].ID, adminCards[i].SortOrder)
		}
	}

	// 集合不一致 / 非管理员整单拒绝。
	if err := svc.ReplaceProjectSortOrder(context.Background(), adminViewer(),
		[]project.ID{second}); !apperr.IsCode(err, apperr.CodeProjectOrderMismatch) {
		t.Fatalf("集合不一致应报顺序冲突，实际 %v", err)
	}
	if ids, _ := repos.FakeProjects().ListIDs(context.Background()); len(ids) != 3 || ids[0] != third {
		t.Fatalf("被拒绝的换序不应改动数据，实际 %v", ids)
	}
	if err := svc.ReplaceProjectSortOrder(context.Background(), authorViewer(),
		[]project.ID{second, first, third}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员换序应被拒绝，实际 %v", err)
	}
}

// TestListProjectAdminItemsIncludesOrderAndStale 后台收录行读面：
// 按顺序返回、带上标题、并标出已失效的收录（排序要拿完整集合去提交，失效行不能漏）。
func TestListProjectAdminItemsIncludesOrderAndStale(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "主题项目")
	a1 := seedPublished(t, repos, 131)
	a2 := seedPublished(t, repos, 132)
	includeProjectArticle(t, svc, id, a1)
	includeProjectArticle(t, svc, id, a2)

	// 收录后文章变私有：收录行保留（后台要能看见并清理），读者侧由仓储过滤。
	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(a2).Author(author).Status(domain.StatusPublished).Visibility(domain.VisibilityUserHidden).Build())

	items, err := svc.ListProjectAdminItems(context.Background(), adminViewer(), id)
	if err != nil {
		t.Fatalf("读后台收录行失败: %v", err)
	}
	if len(items) != 2 || items[0].ArticleID != a1 || items[1].ArticleID != a2 {
		t.Fatalf("收录行不对: %+v", items)
	}
	if items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应连续: %d / %d", items[0].SortOrder, items[1].SortOrder)
	}
	if items[0].Stale() || !items[1].Stale() {
		t.Fatalf("失效标记不对: %v / %v", items[0].Stale(), items[1].Stale())
	}
	if items[1].StaleReason() == "" {
		t.Fatalf("失效行应给出原因: %+v", items[1])
	}

	if _, err := svc.ListProjectAdminItems(context.Background(), authorViewer(), id); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("非管理员读后台收录行应被拒绝，实际 %v", err)
	}
}

// TestCleanupStaleProjectRemovesOnlyStaleAndIsIdempotent 一键清理只删失效行，可重复调用。
func TestCleanupStaleProjectRemovesOnlyStaleAndIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "主题项目")

	live := seedPublished(t, repos, 101)
	draft := seedPublished(t, repos, 102)
	hidden := seedPublished(t, repos, 103)
	deleted := seedPublished(t, repos, 104)
	for _, articleID := range []domain.ArticleID{live, draft, hidden, deleted} {
		includeProjectArticle(t, svc, id, articleID)
	}

	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(draft).Author(author).Status(domain.StatusDraft).
		Visibility(domain.VisibilityVisible).Build())
	repos.FakeArticles().Seed(apptest.NewArticle().
		ID(hidden).Author(author).Status(domain.StatusPublished).
		Visibility(domain.VisibilityUserHidden).Build())
	if err := repos.FakeArticles().SoftDelete(context.Background(), deleted); err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	if removed := cleanupStaleProject(t, svc, id); removed != 3 {
		t.Fatalf("应清理 3 条失效行，实际 %d", removed)
	}

	ids, err := repos.FakeProjects().ListItemIDs(context.Background(), id)
	if err != nil {
		t.Fatalf("读收录失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != live {
		t.Fatalf("清理后应只剩仍公开的那篇 %s，实际 %v", live, ids)
	}
	items, err := svc.ListProjectAdminItems(context.Background(), adminViewer(), id)
	if err != nil {
		t.Fatalf("清理后读后台收录行失败: %v", err)
	}
	if len(items) != 1 || items[0].Stale() || items[0].SortOrder != 1 {
		t.Fatalf("清理后后台应只剩 1 行在架文章且序号压实为 1，实际 %+v", items)
	}

	// 幂等：再点一次不会删到生效行，返回 0。
	if removed := cleanupStaleProject(t, svc, id); removed != 0 {
		t.Fatalf("第二次清理应为 0，实际 %d", removed)
	}
	if ids, _ := repos.FakeProjects().ListItemIDs(context.Background(), id); len(ids) != 1 || ids[0] != live {
		t.Fatalf("重复清理不应改动在架文章，实际 %v", ids)
	}

	// 权限：匿名 / 普通用户不能清理；不存在的项目按不存在处理。
	for name, viewer := range map[string]domain.Viewer{
		"匿名":   domain.Anonymous(),
		"普通用户": authorViewer(),
	} {
		if _, err := svc.CleanupStaleProjectArticles(context.Background(), viewer, id); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%s 清理失效行应被拒绝，实际 %v", name, err)
		}
	}
	if _, err := svc.CleanupStaleProjectArticles(context.Background(), adminViewer(),
		project.ID(ctype.NewID(9999))); !apperr.IsCode(err, apperr.CodeProjectNotFound) {
		t.Fatalf("不存在的项目清理应返回不存在，实际 %v", err)
	}
}

// sameIDs 比较两个 ID 序列（大小写/顺序都要求一致）。
func sameIDs(a, b []project.ArticleID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
