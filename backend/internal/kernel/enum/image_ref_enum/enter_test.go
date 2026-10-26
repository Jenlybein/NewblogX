package image_ref_enum_test

import (
	"testing"

	"myblogx/internal/kernel/enum/image_ref_enum"
)

// allRefTypes 是 RefType 的**全部**取值。
//
// 新增一种引用类型时，必须同时加到这里、String() 与 ParseRefType() 三处；
// 漏了任何一处，下面这条 round-trip 用例会立刻红。
//
// 为什么值得一条专门用例：String() 与 ParseRefType() 是两个手写 switch，
// 各写一遍同一份词表 —— 编译器不会因为它们不一致而报错。将来加第 5 种类型时，
// 症状会是"死信重投报未知类型"，而不是编译失败，排查成本远高于这三行代码。
func allRefTypes() []image_ref_enum.RefType {
	return []image_ref_enum.RefType{
		image_ref_enum.RefTypeArticle,
		image_ref_enum.RefTypeUser,
		image_ref_enum.RefTypeBanner,
		image_ref_enum.RefTypeFavorite,
	}
}

// 枚举 → 字符串 → 枚举，必须回到原值。
func TestRefTypeStringParseRoundTrip(t *testing.T) {
	for _, want := range allRefTypes() {
		raw := want.String()
		if raw == "" || raw == "unknown" {
			t.Fatalf("RefType(%d).String() 应给出可解析的名字，实际 %q", uint(want), raw)
		}
		got, err := image_ref_enum.ParseRefType(raw)
		if err != nil {
			t.Fatalf("%q 应能解析回枚举，实际报错: %v", raw, err)
		}
		if got != want {
			t.Fatalf("round-trip 不一致: %d → %q → %d", uint(want), raw, uint(got))
		}
	}
}

// 认不出来的字符串必须报错，绝不能静默返回 0（否则重投会拿着 ref_type=0 去删数据）。
func TestParseRefTypeRejectsUnknown(t *testing.T) {
	for _, raw := range []string{"", "unknown", "article2", "1"} {
		if _, err := image_ref_enum.ParseRefType(raw); err == nil {
			t.Fatalf("%q 应报错", raw)
		}
	}
}
