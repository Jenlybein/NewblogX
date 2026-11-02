package collection_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel/ctype"
)

// TestCheckSortOrder 合集之间全量换序的输入必须是当前合集集合的一个排列。
func TestCheckSortOrder(t *testing.T) {
	t.Parallel()

	a := collection.ID(ctype.NewID(1))
	b := collection.ID(ctype.NewID(2))
	c := collection.ID(ctype.NewID(3))
	current := []collection.ID{a, b, c}

	if err := collection.CheckSortOrder(current, []collection.ID{c, a, b}); err != nil {
		t.Fatalf("合法换序应通过: %v", err)
	}

	cases := map[string][]collection.ID{
		"缺一个":  {c, a},
		"多一个":  {c, a, b, collection.ID(ctype.NewID(4))},
		"有重复":  {a, a, b},
		"未知 ID": {c, a, collection.ID(ctype.NewID(9))},
	}
	for name, next := range cases {
		if err := collection.CheckSortOrder(current, next); !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
			t.Fatalf("%s 应被拒绝，实际 %v", name, err)
		}
	}
}
