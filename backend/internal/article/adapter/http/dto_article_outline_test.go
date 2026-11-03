package http

import (
	"encoding/json"
	"testing"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain/outline"
)

// contextTestNow 是节点构造时间的固定值（domain 不取 time.Now）。
var contextTestNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// 文章上下文（阅读页左栏）的契约接缝：断言**外部可观察的 JSON 形状**
// （swagger.json 与两端 openapi.d.ts 都从它派生）。

func TestArticleOutlineContextResponseJSONShape(t *testing.T) {
	t.Parallel()

	payload := ArticleOutlineContextResponse{
		HasTree:   true,
		OwnerKind: int(outline.OwnerProject),
		OwnerID:   "p1",
		Nodes: []OutlineNodeResponse{
			{
				ID:    "n1",
				Kind:  int(outline.NodeKindFolder),
				Title: "第一章",
				Children: []OutlineNodeResponse{
					{ID: "n2", Kind: int(outline.NodeKindArticle), ArticleID: "a1", ArticleTitle: "第一篇"},
				},
			},
		},
		Current:  &OutlineArticleRefResponse{ID: "n2", ArticleID: "a1", ArticleTitle: "第一篇"},
		Previous: nil,
		Next:     nil,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化文章上下文失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("解析文章上下文 JSON 失败: %v", err)
	}
	if got["has_tree"] != true || got["owner_kind"] != float64(2) || got["owner_id"] != "p1" {
		t.Fatalf("归属字段不对: %s", raw)
	}
	if _, ok := got["nodes"].([]any); !ok {
		t.Fatalf("nodes 应是数组: %s", raw)
	}
	current, ok := got["current"].(map[string]any)
	if !ok || current["id"] != "n2" || current["article_id"] != "a1" || current["article_title"] != "第一篇" {
		t.Fatalf("current 应是文章节点引用: %s", raw)
	}
	if _, exists := got["previous"]; !exists {
		t.Fatalf("previous 字段必须出现（可为 null），前端按它渲染上一篇: %s", raw)
	}
	if _, exists := got["next"]; !exists {
		t.Fatalf("next 字段必须出现（可为 null）: %s", raw)
	}
}

func TestToArticleOutlineContextResponseMapsNeighbours(t *testing.T) {
	t.Parallel()

	a1, err := outline.New(outline.NewParams{
		ID: "n1", OwnerKind: outline.OwnerCollection, OwnerID: "c1",
		Kind: outline.NodeKindArticle, ArticleID: "a1", SortOrder: 1, Now: contextTestNow,
	})
	if err != nil {
		t.Fatalf("构造节点失败: %v", err)
	}
	a2, err := outline.New(outline.NewParams{
		ID: "n2", OwnerKind: outline.OwnerCollection, OwnerID: "c1",
		Kind: outline.NodeKindArticle, ArticleID: "a2", SortOrder: 2, Now: contextTestNow,
	})
	if err != nil {
		t.Fatalf("构造节点失败: %v", err)
	}

	current := outline.ViewNode{Node: a2, ArticleTitle: "第二篇", ArticleCover: "https://img.example/2.png"}
	previous := outline.ViewNode{Node: a1, ArticleTitle: "第一篇"}
	src := &app.ArticleOutlineContext{
		HasTree:   true,
		OwnerKind: outline.OwnerCollection,
		OwnerID:   "c1",
		Tree: []outline.ViewNode{
			{Node: a1, ArticleTitle: "第一篇"},
			{Node: a2, ArticleTitle: "第二篇", ArticleCover: "https://img.example/2.png"},
		},
		Current:  &current,
		Previous: &previous,
		Next:     nil,
	}

	got := toArticleOutlineContextResponse(src)
	if !got.HasTree || got.OwnerKind != int(outline.OwnerCollection) || got.OwnerID != "c1" {
		t.Fatalf("归属没有映射过去: %+v", got)
	}
	if got.Current == nil || got.Current.ArticleID != "a2" || got.Current.ArticleTitle != "第二篇" {
		t.Fatalf("当前节点映射不对: %+v", got.Current)
	}
	if got.Previous == nil || got.Previous.ArticleID != "a1" || got.Previous.ArticleTitle != "第一篇" {
		t.Fatalf("上一篇映射不对: %+v", got.Previous)
	}
	if got.Next != nil {
		t.Fatalf("最后一篇的下一篇应为空: %+v", got.Next)
	}
}
