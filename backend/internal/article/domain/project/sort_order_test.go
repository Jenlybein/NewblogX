package project_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// TestCheckSortOrder 项目之间全量换序的输入必须是当前项目集合的一个排列。
//
// 与合集同一条语义：缺一个、多一个、重复都会整单拒绝——后台拿到的是上一次
// 读到的列表，宽松语义会把别人刚建的项目静默排丢。
func TestCheckSortOrder(t *testing.T) {
	t.Parallel()

	a := project.ID(ctype.NewID(1))
	b := project.ID(ctype.NewID(2))
	c := project.ID(ctype.NewID(3))
	current := []project.ID{a, b, c}

	if err := project.CheckSortOrder(current, []project.ID{c, a, b}); err != nil {
		t.Fatalf("合法换序应通过: %v", err)
	}

	cases := map[string][]project.ID{
		"缺一个":   {c, a},
		"多一个":   {c, a, b, project.ID(ctype.NewID(4))},
		"有重复":   {a, a, b},
		"未知 ID": {c, a, project.ID(ctype.NewID(9))},
	}
	for name, next := range cases {
		if err := project.CheckSortOrder(current, next); !apperr.IsCode(err, apperr.CodeProjectOrderMismatch) {
			t.Fatalf("%s 应被拒绝，实际 %v", name, err)
		}
	}
}

// TestCheckOrder 项目内全量换序用同一套集合一致性规则。
func TestCheckOrder(t *testing.T) {
	t.Parallel()

	a := project.ArticleID(ctype.NewID(11))
	b := project.ArticleID(ctype.NewID(12))
	current := []project.ArticleID{a, b}

	if err := project.CheckOrder(current, []project.ArticleID{b, a}); err != nil {
		t.Fatalf("合法换序应通过: %v", err)
	}
	if err := project.CheckOrder(current, []project.ArticleID{b}); !apperr.IsCode(err, apperr.CodeProjectOrderMismatch) {
		t.Fatalf("缺一篇应被拒绝，实际 %v", err)
	}
}

// TestAdminItemStale 后台收录行的失效判定与「热门置顶 / 合集」同一份领域函数：
// 前端不另算，后端也不另写。
func TestAdminItemStale(t *testing.T) {
	t.Parallel()

	live := project.AdminItem{Article: &domain.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}}
	if live.Stale() || live.StaleReason() != "" {
		t.Fatalf("仍公开的文章不该标失效: %v/%s", live.Stale(), live.StaleReason())
	}

	cases := map[string]*project.AdminItem{
		"文章物理删除": {Article: nil},
		"文章软删":   {Article: &domain.ArticleState{Deleted: true}},
		"退回草稿":   {Article: &domain.ArticleState{Status: domain.StatusDraft, Visibility: domain.VisibilityVisible}},
		"作者隐藏":   {Article: &domain.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityUserHidden}},
	}
	for name, item := range cases {
		if !item.Stale() {
			t.Fatalf("%s 应标为失效（判定函数与合集必须一致）", name)
		}
		if item.StaleReason() == "" {
			t.Fatalf("%s 应给出失效原因", name)
		}
	}
}
