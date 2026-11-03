//go:build integration

package http_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是浏览历史**契约**的端到端用例（#241）：
//   - `GET /api/articles/history` 的 keyword 与游标（before_updated_at + before_id）；
//   - `DELETE /api/articles/history/all` 清空；
//   - 半游标 / 坏时间戳必须报"参数错误"（1001），而不是静默退回第一页。
//
// 必须走真 HTTP + 真 MySQL：游标是 SQL 比较、keyword 是 LIKE、清空是按 user_id 软删。

// TestHistoryCursorKeywordAndClearEndToEnd 覆盖读契约与清空入口。
func TestHistoryCursorKeywordAndClearEndToEnd(t *testing.T) {
	db := migrate(t)

	// 历史的主人（200）+ 另一个用户（100），清空只能动自己的。
	seedGoldenUser(t, db, 100, "e2e_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "e2e_reader", "读者", "", "", schema.RoleUser)

	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, spec := range []struct {
		id, author uint64
		title      string
	}{
		{1001, 100, "Go 并发模型"},
		{1002, 100, "Rust 所有权"},
		{1003, 100, "Go 泛型"},
	} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             spec.id,
			Title:          spec.title,
			AuthorID:       spec.author,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      base,
			UpdatedAt:      base,
		})
	}

	// 读者 200 看过三篇（1003 最新）；作者 100 也有一条，用于验证清空不越界。
	schema.InsertViewHistory(t, db, 1, 200, 1001, base)
	schema.InsertViewHistory(t, db, 2, 200, 1002, base.Add(10*time.Minute))
	schema.InsertViewHistory(t, db, 3, 200, 1003, base.Add(20*time.Minute))
	schema.InsertViewHistory(t, db, 4, 100, 1002, base)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))
	author := newRouter(t, db, domain.NormalUser(ctype.NewID(100)))

	// ── 第一页：limit=2，has_more 为真，倒序 ──
	page1 := do(t, reader, http.MethodGet, "/api/articles/history?limit=2", nil)
	if code, _ := page1["code"].(float64); code != 0 {
		t.Fatalf("列历史失败: %v", page1)
	}
	data, ok := page1["data"].(map[string]any)
	if !ok {
		t.Fatalf("历史响应缺 data: %v", page1)
	}
	if more, _ := data["has_more"].(bool); !more {
		t.Fatalf("还有第 3 条时 has_more 应为 true: %v", data)
	}
	items := listOf(t, page1)
	if len(items) != 2 {
		t.Fatalf("limit=2 应返回 2 条，实际 %d", len(items))
	}
	if items[0]["article_id"] != "1003" || items[0]["title"] != "Go 泛型" {
		t.Fatalf("第 1 页第 1 条应是最新的 1003，实际 %v", items[0])
	}
	// 卡片只用封面/标题/浏览时间——分类/作者/阅读时长不该出现（#241 明确不加列）。
	for _, banned := range []string{"category", "category_name", "author", "author_name", "read_duration"} {
		if _, ok := items[0][banned]; ok {
			t.Fatalf("历史项不应带字段 %q: %v", banned, items[0])
		}
	}

	// ── 游标续页：用上一条的 updated_at + article_id ──
	before := url.QueryEscape(items[1]["updated_at"].(string))
	page2 := do(t, reader, http.MethodGet,
		"/api/articles/history?limit=2&before_updated_at="+before+"&before_id="+items[1]["article_id"].(string), nil)
	if code, _ := page2["code"].(float64); code != 0 {
		t.Fatalf("游标续页失败: %v", page2)
	}
	if more, _ := page2["data"].(map[string]any)["has_more"].(bool); more {
		t.Fatalf("末页 has_more 应为 false: %v", page2["data"])
	}
	rest := listOf(t, page2)
	if len(rest) != 1 || rest[0]["article_id"] != "1001" {
		t.Fatalf("续页应只剩 1001，实际 %v", rest)
	}

	// ── keyword 按标题过滤（与游标同口径） ──
	hits := listOf(t, do(t, reader, http.MethodGet, "/api/articles/history?keyword=Go", nil))
	if len(hits) != 2 || hits[0]["article_id"] != "1003" || hits[1]["article_id"] != "1001" {
		t.Fatalf("keyword=Go 应命中 [1003 1001]，实际 %v", hits)
	}
	// 未命中：空列表 + 成功
	empty := do(t, reader, http.MethodGet, "/api/articles/history?keyword=Kotlin", nil)
	if code, _ := empty["code"].(float64); code != 0 || len(listOf(t, empty)) != 0 {
		t.Fatalf("未命中应返回空列表，实际 %v", empty)
	}

	// ── 半游标 / 坏时间戳：1001 参数错误，而不是静默退回第一页 ──
	if code, _ := do(t, reader, http.MethodGet, "/api/articles/history?before_id=1003", nil)["code"].(float64); code != 1001 {
		t.Fatalf("只传 before_id 应报参数错误(1001): %v", do(t, reader, http.MethodGet, "/api/articles/history?before_id=1003", nil))
	}
	if code, _ := do(t, reader, http.MethodGet,
		"/api/articles/history?before_updated_at=not-a-time&before_id=1003", nil)["code"].(float64); code != 1001 {
		t.Fatalf("坏时间戳应报参数错误(1001)")
	}

	// ── 清空历史：只清自己的 ──
	clear := do(t, reader, http.MethodDelete, "/api/articles/history/all", nil)
	if code, _ := clear["code"].(float64); code != 0 {
		t.Fatalf("清空历史失败: %v", clear)
	}
	if got := len(listOf(t, do(t, reader, http.MethodGet, "/api/articles/history", nil))); got != 0 {
		t.Fatalf("清空后应为空列表，实际 %d 条", got)
	}
	if got := len(listOf(t, do(t, author, http.MethodGet, "/api/articles/history", nil))); got != 1 {
		t.Fatalf("别人的历史不应被清空，作者应仍有 1 条，实际 %d", got)
	}
}
