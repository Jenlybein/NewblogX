package app

import (
	"encoding/json"
	"testing"
)

// 锁定"ES 响应 → 搜索响应行"的映射。
//
// ADR-0017 决策 6 删掉了 top 一族派生字段：文档里没有、_source 不再要、
// 响应结构里也不再出现 top 对象。这里喂一份**故意还带着旧字段**的 _source，
// 证明它们既不会被读出、也不会漏到响应里（旧索引残留不会污染契约）；
// 同时证明新增的 author_is_admin 会被读进响应。
func TestExtractResponseShape(t *testing.T) {
	source := map[string]any{
		"id":              "1001",
		"title":           "标题",
		"author_id":       "9",
		"author":          map[string]any{"id": "9", "nickname": "作者A", "avatar": "/a.png"},
		"author_is_admin": true,
		// 旧索引里的残留字段：不应被读出、也不应出现在响应里。
		"top":        map[string]any{"user": true, "admin": true},
		"admin_top":  true,
		"author_top": true,
	}
	data := map[string]any{
		"hits": []any{
			map[string]any{"_source": source, "_score": 1.5},
		},
	}

	list := extract(nil, data)
	if len(list) != 1 {
		t.Fatalf("应解析出一行, got=%d", len(list))
	}
	if !list[0].AuthorIsAdmin {
		t.Fatalf("author_is_admin 应从 _source 读入, got=%#v", list[0].AuthorIsAdmin)
	}

	raw, err := json.Marshal(list[0])
	if err != nil {
		t.Fatalf("序列化响应行失败: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("反序列化响应行失败: %v", err)
	}
	if _, ok := payload["top"]; ok {
		t.Fatalf("响应里不应再出现 top 对象: %s", raw)
	}
	if got, ok := payload["author_is_admin"].(bool); !ok || !got {
		t.Fatalf("响应里应带 author_is_admin=true: %s", raw)
	}
}
