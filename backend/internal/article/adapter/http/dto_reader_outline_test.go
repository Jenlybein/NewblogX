package http_test

import (
	"encoding/json"
	"testing"

	articlehttp "myblogx/internal/article/adapter/http"
)

// 读者详情契约接缝：详情响应里不再有扁平 articles[]，而是嵌套 outline 树。
// 断言的是**外部可观察的 JSON 形状**（swagger.json 与两端 openapi.d.ts 都从它派生）。

func TestCollectionDetailResponseIsNestedOutline(t *testing.T) {
	t.Parallel()

	payload := articlehttp.CollectionDetailResponse{
		ID:    "c1",
		Title: "专题",
		Cover: "https://img.example/first.png",
		Outline: articlehttp.OutlineTreeResponse{
			OwnerKind: 1,
			OwnerID:   "c1",
			Nodes: []articlehttp.OutlineNodeResponse{
				{
					ID:    "n1",
					Kind:  1,
					Title: "第一章",
					Children: []articlehttp.OutlineNodeResponse{
						{ID: "n2", Kind: 2, ArticleID: "a1", ArticleTitle: "第一篇"},
					},
				},
			},
		},
		ArticleCount: 1,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化详情失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("解析详情 JSON 失败: %v", err)
	}
	if _, exists := got["articles"]; exists {
		t.Fatalf("扁平 articles[] 应已退役，实际 JSON: %s", raw)
	}
	outline, ok := got["outline"].(map[string]any)
	if !ok || outline["owner_kind"] != float64(1) || outline["owner_id"] != "c1" {
		t.Fatalf("详情应带 OutlineTreeResponse 形状: %s", raw)
	}
	nodes, ok := outline["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("根节点应是数组且保留文件夹，实际: %s", raw)
	}
	children, ok := nodes[0].(map[string]any)["children"].([]any)
	if !ok || len(children) != 1 || children[0].(map[string]any)["article_id"] != "a1" {
		t.Fatalf("文章节点应嵌套在文件夹里，实际: %s", raw)
	}
	if got["article_count"] != float64(1) || got["cover"] != "https://img.example/first.png" {
		t.Fatalf("篇数与封面回退应在详情里，实际: %s", raw)
	}
}

func TestProjectDetailResponseIsNestedOutline(t *testing.T) {
	t.Parallel()

	payload := articlehttp.ProjectDetailResponse{
		ID:    "p1",
		Title: "项目",
		Outline: articlehttp.OutlineTreeResponse{
			OwnerKind: 2,
			OwnerID:   "p1",
			Nodes: []articlehttp.OutlineNodeResponse{
				{ID: "n1", Kind: 2, ArticleID: "a1", ArticleTitle: "第一篇"},
			},
		},
		ArticleCount: 1,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化详情失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("解析详情 JSON 失败: %v", err)
	}
	if _, exists := got["articles"]; exists {
		t.Fatalf("扁平 articles[] 应已退役，实际 JSON: %s", raw)
	}
	outline, ok := got["outline"].(map[string]any)
	if !ok || outline["owner_kind"] != float64(2) {
		t.Fatalf("项目详情应带项目归属的树，实际: %s", raw)
	}
	if got["article_count"] != float64(1) {
		t.Fatalf("篇数只数公开文章节点，实际: %s", raw)
	}
}
