package canalbase

import (
	"reflect"
	"testing"
)

// IncludeTableRegex 必须稳定：map 遍历顺序随机，若不排序会导致每次生成的配置不同。
func TestBuildIncludeRegexIsDeterministic(t *testing.T) {
	spec := Spec{Tables: map[string][]string{
		"blog":  {"user_models", "article_models"},
		"audit": {"log_models"},
	}}

	want := []string{`audit\.log_models`, `blog\.article_models`, `blog\.user_models`}

	// 跑多次，确认顺序稳定
	for i := 0; i < 20; i++ {
		got := buildIncludeRegex(spec)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("第 %d 次结果不一致: %v，期望 %v", i, got, want)
		}
	}
}

func TestBuildIncludeRegexEmpty(t *testing.T) {
	if got := buildIncludeRegex(Spec{}); got != nil {
		t.Fatalf("空 Spec 应返回 nil，实际 %v", got)
	}
	if got := buildIncludeRegex(Spec{Tables: map[string][]string{}}); got != nil {
		t.Fatalf("空表应返回 nil，实际 %v", got)
	}
}

// 单个 schema 按表转储；多个 schema 只能按库转储。
// PrepareDump 需要真实 canal 实例，这里只验证不 panic 的边界（nil 安全）。
func TestPrepareDumpNilSafe(t *testing.T) {
	PrepareDump(nil, Spec{Tables: map[string][]string{"blog": {"a"}}})
	PrepareDump(nil, Spec{})
}
