package config

import (
	"testing"
)

// 合并语义是"加字段零迁移"的地基，所以这里按规则逐条钉住。
// 文档写了什么就测什么，避免实现与注释各说一套。

func TestMergeJSON_空载荷返回默认值原样(t *testing.T) {
	type sample struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	base := sample{A: "默认", B: 7}

	for _, payload := range []string{"", "   ", "null"} {
		got, err := mergeJSON(base, payload)
		if err != nil {
			t.Fatalf("payload=%q 不应报错: %v", payload, err)
		}
		if got["a"] != "默认" || got["b"] != float64(7) {
			t.Fatalf("payload=%q 应原样返回默认值，实际 %v", payload, got)
		}
	}
}

func TestMergeJSON_缺字段用默认值补齐(t *testing.T) {
	// 场景：库里那段 JSON 是旧版本，只有 a 没有 b。
	// 期望 b 用默认值，而不是退化成零值 —— 这就是"加字段不写迁移"的关键。
	type sample struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	base := sample{A: "默认", B: 7}

	got, err := mergeJSON(base, `{"a":"改过"}`)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}
	if got["a"] != "改过" {
		t.Fatalf("a 应被覆盖为 改过，实际 %v", got["a"])
	}
	if got["b"] != float64(7) {
		t.Fatalf("b 缺失时应保留默认值 7，实际 %v", got["b"])
	}
}

func TestMergeJSON_嵌套对象递归合并(t *testing.T) {
	type inner struct {
		X string `json:"x"`
		Y string `json:"y"`
	}
	type outer struct {
		Sub  inner    `json:"sub"`
		List []string `json:"list"`
	}
	base := outer{Sub: inner{X: "dx", Y: "dy"}, List: []string{"a", "b"}}

	got, err := mergeJSON(base, `{"sub":{"x":"ox"}}`)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}
	sub, ok := got["sub"].(map[string]any)
	if !ok {
		t.Fatalf("sub 应是对象，实际 %T", got["sub"])
	}
	if sub["x"] != "ox" {
		t.Fatalf("sub.x 应被覆盖，实际 %v", sub["x"])
	}
	if sub["y"] != "dy" {
		t.Fatalf("sub.y 未出现载荷中，应保留默认值 dy，实际 %v", sub["y"])
	}
}

func TestMergeMaps_数组整体替换不做逐元素合并(t *testing.T) {
	// 逐元素合并会让"删掉一个白名单项"永远删不掉。
	base := map[string]any{"list": []any{"a", "b", "c"}}
	got := mergeMaps(base, map[string]any{"list": []any{"z"}})

	list, ok := got["list"].([]any)
	if !ok {
		t.Fatalf("list 应是数组，实际 %T", got["list"])
	}
	if len(list) != 1 || list[0] != "z" {
		t.Fatalf("数组应整体替换为 [z]，实际 %v", list)
	}
}

func TestMergeMaps_null不覆盖(t *testing.T) {
	// null 在 JSON 里表示"没填"，让它清空一个有默认值的字段没有意义。
	base := map[string]any{"level": "info", "keep": "v"}
	got := mergeMaps(base, map[string]any{"level": nil})

	if got["level"] != "info" {
		t.Fatalf("null 不应覆盖掉默认值，实际 %v", got["level"])
	}
	if got["keep"] != "v" {
		t.Fatalf("未提及的键应保留，实际 %v", got["keep"])
	}
}

func TestMergeMaps_空字符串可以清空字段(t *testing.T) {
	// 与 null 相对：后台把输入框清空后存的是 ""，必须能真的清掉。
	base := map[string]any{"secret": "旧密钥"}
	got := mergeMaps(base, map[string]any{"secret": ""})

	if got["secret"] != "" {
		t.Fatalf(`空字符串应能清空字段，实际 %v`, got["secret"])
	}
}

func TestMergeMaps_类型不匹配时载荷胜出(t *testing.T) {
	// 默认值是对象、载荷给了标量（或反过来），不做补救，一律以载荷为准。
	base := map[string]any{"cfg": map[string]any{"a": 1}}
	got := mergeMaps(base, map[string]any{"cfg": "scalar"})

	if got["cfg"] != "scalar" {
		t.Fatalf("类型不匹配时载荷应胜出，实际 %v", got["cfg"])
	}
}

func TestMergeJSON_非法JSON返回错误(t *testing.T) {
	type sample struct {
		A string `json:"a"`
	}
	if _, err := mergeJSON(sample{}, `{不是 JSON`); err == nil {
		t.Fatal("非法 JSON 应该返回错误，而不是被静默忽略")
	}
}

func TestMergeJSON_不修改传入的base(t *testing.T) {
	// base 是默认值结构体（值传递），但仍要确认合并没有写回底座，
	// 否则默认值会被第一次合并"污染"，后续所有段都读到脏数据。
	type inner struct {
		X string `json:"x"`
	}
	type outer struct {
		Sub inner `json:"sub"`
	}
	base := outer{Sub: inner{X: "dx"}}

	if _, err := mergeJSON(base, `{"sub":{"x":"写坏"}}`); err != nil {
		t.Fatalf("合并失败: %v", err)
	}
	if base.Sub.X != "dx" {
		t.Fatalf("合并不应修改传入的 base，实际 %q", base.Sub.X)
	}
}
