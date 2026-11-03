package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「项目」管理端的用例：增删改、展示开关、项目之间排序、权限线。
//
// 断言的都是**外部可观察的行为**：读者读面与后台列表看到什么、顺序怎么变。
// 与合集同一批断言，因为两者是镜像实现；两套用例各自独立，
// 不靠"合集绿了所以项目也绿"。
//
// 收录（含批量收录与移除）自 #218 起只经目录树整树提交，相关用例在
// outline_test.go；旧扁平收录用例随接口一起退役（#225）。

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

// TestDeleteProjectHidesFromAdminList 软删：后台列表与读者列表都不再出现，文章本身保留。
func TestDeleteProjectHidesFromAdminList(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	id := createProject(t, svc, "待删项目")
	a := seedPublished(t, repos, 71)
	seedReaderArticleNodes(t, repos, outline.OwnerProject, outline.ID(id), ctype.ID(a))

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
