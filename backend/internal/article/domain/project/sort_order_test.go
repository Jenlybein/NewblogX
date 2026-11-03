package project_test

import (
	"testing"

	"myblogx/internal/apperr"
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
