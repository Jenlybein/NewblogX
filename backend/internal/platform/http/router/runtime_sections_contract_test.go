package router_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// 九段运行时配置的具名契约（#283）。
//
// 唯一行为接缝 = 后端 HTTP 接口 + 契约（开放接口契约）。这里只读已生成的契约，
// 不读 Go / TS 源码文本：九段每段都必须有具名的 GET / PUT 端点，且响应与
// 请求体各自指向该段自己的具名 schema，而不是一个随 name 变形的自由键值块。
// 参数化 /site/admin/{name} 与 /site/{name} 一旦仍在，就意味着契约没有形状
// 可派生，两端只能继续手抄视图模型。
var runtimeSectionNames = []string{
	"site", "ai", "mail", "storage", "oauth", "log", "upload", "deco", "image_gc",
}

func TestNineRuntimeSectionsHaveNamedContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "swagger.json"))
	if err != nil {
		t.Fatalf("读取契约失败: %v", err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析契约失败: %v", err)
	}

	for _, name := range runtimeSectionNames {
		adminRead := "/site/admin/" + name
		write := "/site/" + name
		for _, path := range []string{adminRead, write} {
			item, ok := doc.Paths[path]
			if !ok {
				t.Errorf("段 %q 缺少具名端点 %s（不能再用参数化 /site/{name} 代替）", name, path)
				continue
			}
			for method, op := range item {
				refs, ok := operationSchemaRefs(op)
				if !ok {
					t.Errorf("段 %q 的 %s %s 没有可解析的 schema 引用", name, method, path)
					continue
				}
				for _, ref := range refs {
					if strings.Contains(ref, "SiteConfigData") || strings.Contains(ref, "map[string]any") {
						t.Errorf("段 %q 的 %s %s 仍在用自由键值块 %s", name, method, path, ref)
					}
				}
			}
		}
	}

	// 旧参数化出入口必须整体退场：留着它等于「九段仍可绕过具名形状」。
	for _, path := range []string{"/site/admin/{name}", "/site/{name}"} {
		if _, ok := doc.Paths[path]; ok {
			t.Errorf("参数化端点 %s 仍在契约里，九段没有真正具名化", path)
		}
	}
	for key := range doc.Components.Schemas {
		if strings.Contains(key, "SiteConfigData") {
			t.Errorf("自由键值块 schema %s 仍在契约里", key)
		}
	}

	// 具名形状必须在场：九段各自的响应 schema 一个都不能少。
	var missing []string
	for _, name := range runtimeSectionNames {
		want := "internal_site_adapter_http.SiteSection" + sectionTypeName(name) + "Response"
		if _, ok := doc.Components.Schemas[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("缺少九段具名响应 schema:\n  %s", strings.Join(missing, "\n  "))
	}
}

func sectionTypeName(name string) string {
	parts := strings.Split(name, "_")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// operationSchemaRefs 取一个 operation 的 200 响应与请求体里出现的所有 $ref。
func operationSchemaRefs(op json.RawMessage) ([]string, bool) {
	var parsed struct {
		Responses   map[string]json.RawMessage `json:"responses"`
		RequestBody json.RawMessage            `json:"requestBody"`
	}
	if err := json.Unmarshal(op, &parsed); err != nil {
		return nil, false
	}
	refs := make([]string, 0, 2)
	collectRefs := func(raw json.RawMessage) {
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			return
		}
		var walk func(any)
		walk = func(v any) {
			switch typed := v.(type) {
			case map[string]any:
				if ref, ok := typed["$ref"].(string); ok {
					refs = append(refs, ref)
				}
				for _, child := range typed {
					walk(child)
				}
			case []any:
				for _, child := range typed {
					walk(child)
				}
			}
		}
		walk(node)
	}
	if resp, ok := parsed.Responses["200"]; ok {
		collectRefs(resp)
	}
	if len(parsed.RequestBody) > 0 {
		collectRefs(parsed.RequestBody)
	}
	return refs, len(refs) > 0
}
