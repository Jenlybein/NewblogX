package grouping_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/kernel/ctype"
)

// TestCheckOrder 整树顺序提交的输入必须是当前集合的排列。
//
// 语义与标签 / 项目 / 热门置顶的全量换序同一条：后台拿到的是上一次读到的列表，
// 别人刚建或刚删一个分组后，宽松的"缺几个也照写"会把那条静默排丢。
// 因此缺分组、多分组、漏条目、重复条目、未知条目全部整单拒绝。
func TestCheckOrder(t *testing.T) {
	t.Parallel()

	g1 := grouping.ID(ctype.NewID(1))
	g2 := grouping.ID(ctype.NewID(2))
	o1 := ctype.NewID(11)
	o2 := ctype.NewID(12)
	o3 := ctype.NewID(13)

	currentGroups := []grouping.ID{g1, g2}
	currentOwners := []ctype.ID{o1, o2, o3}

	ok := grouping.TreeOrder{
		Groups: []grouping.GroupOrder{
			{ID: g2, Items: []ctype.ID{o3, o1}},
			{ID: g1, Items: []ctype.ID{o2}},
		},
	}
	if err := grouping.CheckOrder(currentGroups, currentOwners, ok); err != nil {
		t.Fatalf("合法换序应通过: %v", err)
	}

	// 空集合对空集合也应通过（两个实体都还没有内容时后台不该报错）。
	if err := grouping.CheckOrder(nil, nil, grouping.TreeOrder{}); err != nil {
		t.Fatalf("空集合换空集合应通过: %v", err)
	}

	cases := map[string]struct {
		groups []grouping.ID
		owners []ctype.ID
		tree   grouping.TreeOrder
	}{
		"缺分组": {
			currentGroups, currentOwners,
			grouping.TreeOrder{Groups: []grouping.GroupOrder{{ID: g1, Items: []ctype.ID{o1, o2, o3}}}},
		},
		"多分组": {
			currentGroups, currentOwners,
			grouping.TreeOrder{Groups: []grouping.GroupOrder{
				{ID: g1, Items: []ctype.ID{o1, o2, o3}},
				{ID: g2},
				{ID: grouping.ID(ctype.NewID(9))},
			}},
		},
		"漏条目": {
			currentGroups, currentOwners,
			grouping.TreeOrder{Groups: []grouping.GroupOrder{
				{ID: g1, Items: []ctype.ID{o1, o2}},
				{ID: g2},
			}},
		},
		"重复条目": {
			currentGroups, currentOwners,
			grouping.TreeOrder{Groups: []grouping.GroupOrder{
				{ID: g1, Items: []ctype.ID{o1, o2}},
				{ID: g2, Items: []ctype.ID{o1, o3}},
			}},
		},
		"未知条目": {
			currentGroups, currentOwners,
			grouping.TreeOrder{Groups: []grouping.GroupOrder{
				{ID: g1, Items: []ctype.ID{o1, o2, o3}},
				{ID: g2, Items: []ctype.ID{ctype.NewID(99)}},
			}},
		},
	}
	for name, tc := range cases {
		if err := grouping.CheckOrder(tc.groups, tc.owners, tc.tree); !apperr.IsCode(err, apperr.CodeGroupOrderMismatch) {
			t.Fatalf("%s 应被拒绝，实际 %v", name, err)
		}
	}
}

// TestNormalizeTitle 标题按字符数校验（中文按字节算会被提前截断）。
func TestNormalizeTitle(t *testing.T) {
	t.Parallel()

	if _, err := grouping.New(grouping.NewParams{Title: "   "}); !apperr.IsCode(err, apperr.CodeGroupTitleRequired) {
		t.Fatalf("空白标题应被拒绝，实际 %v", err)
	}

	// 128 个汉字：按字节算会超（384 字节），按字符算正好等于上限。
	title := ""
	for i := 0; i < grouping.MaxTitleLen; i++ {
		title += "字"
	}
	g, err := grouping.New(grouping.NewParams{Title: title})
	if err != nil {
		t.Fatalf("128 个汉字应通过: %v", err)
	}
	if len([]rune(g.Title())) != grouping.MaxTitleLen {
		t.Fatalf("标题应完整保留，实际 %d 个字符", len([]rune(g.Title())))
	}

	if _, err := grouping.New(grouping.NewParams{Title: title + "字"}); !apperr.IsCode(err, apperr.CodeGroupTitleTooLong) {
		t.Fatalf("超长标题应被拒绝，实际 %v", err)
	}
}
