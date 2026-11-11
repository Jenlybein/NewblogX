package taxonomy_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
)

// TestTagCheckSortOrder 标签之间全量换序的输入必须是当前标签全集的一个排列。
//
// 语义与项目 / 热门置顶的全量换序同一条：后台拿到的是上一次读到的列表，
// 别人刚建或刚删一个标签后，宽松的"缺几个也照写"会把那条静默排丢。
// 因此缺一个、多一个、重复、未知 ID 全部整单拒绝。
func TestTagCheckSortOrder(t *testing.T) {
	t.Parallel()

	a := domain.TagID(ctype.NewID(1))
	b := domain.TagID(ctype.NewID(2))
	c := domain.TagID(ctype.NewID(3))
	current := []domain.TagID{a, b, c}

	if err := taxonomy.CheckSortOrder(current, []domain.TagID{c, a, b}); err != nil {
		t.Fatalf("合法换序应通过: %v", err)
	}
	if err := taxonomy.CheckSortOrder(nil, nil); err != nil {
		t.Fatalf("空集合换空集合应通过: %v", err)
	}

	cases := map[string][]domain.TagID{
		"缺一个":   {c, a},
		"多一个":   {c, a, b, domain.TagID(ctype.NewID(4))},
		"有重复":   {a, a, b},
		"未知 ID": {c, a, domain.TagID(ctype.NewID(9))},
	}
	for name, next := range cases {
		if err := taxonomy.CheckSortOrder(current, next); !apperr.IsCode(err, apperr.CodeTagOrderMismatch) {
			t.Fatalf("%s 应被拒绝，实际 %v", name, err)
		}
	}
}
