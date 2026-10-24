//go:build integration

package http_test

import (
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/models/enum"
)

// TestSubdomainEndpointsEndToEnd 逐个子域端点走真实 HTTP + 真 MySQL。
//
// golden 只覆盖读接口；这里补上写路径与副作用：
// 分类/标签/收藏夹增删、收藏文章、置顶、浏览历史、点赞、更新、
// 管理员改可见性、审核任务通过、删除文章。
func TestSubdomainEndpointsEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "sd_author", "作者甲", "", "", enum.RoleUser)
	seedGoldenUser(t, db, 200, "sd_reader", "读者", "", "", enum.RoleUser)
	seedGoldenUser(t, db, 900, "sd_admin", "管理员", "", "", enum.RoleAdmin)

	author := newRouterWithIDs(t, db, domain.NormalUser(100), 1000)
	reader := newRouterWithIDs(t, db, domain.NormalUser(200), 2000)
	admin := newRouterWithIDs(t, db, domain.AdminUser(900), 3000)

	// ── 分类：建 → 列表 → 删 ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/category",
		[]byte(`{"title":"端到端分类"}`)), "建分类")
	catID := findIDByTitle(listOf(t, do(t, author, http.MethodGet,
		"/api/articles/category?user_id=100", nil)), "端到端分类")
	if catID == "" {
		t.Fatal("分类列表缺新分类")
	}
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/category",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, catID))), "删分类")

	// ── 标签（管理员）：建 → 列表 → 删 ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(`{"title":"端到端标签","sort":9}`)), "建标签")
	tagID := findIDByTitle(listOf(t, do(t, admin, http.MethodGet,
		"/api/articles/tags", nil)), "端到端标签")
	if tagID == "" {
		t.Fatal("标签列表缺新标签")
	}
	respCode(t, do(t, admin, http.MethodDelete, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, tagID))), "删标签")

	// ── 收藏夹：建 → 列表 → 删 ──
	respCode(t, do(t, author, http.MethodPut, "/api/articles/favorite",
		[]byte(`{"title":"端到端收藏夹"}`)), "建收藏夹")
	folderID := findIDByTitle(listOf(t, do(t, author, http.MethodGet,
		"/api/articles/favorite?user_id=100", nil)), "端到端收藏夹")
	if folderID == "" {
		t.Fatal("收藏夹列表缺新夹")
	}
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/favorite",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, folderID))), "删收藏夹")

	// ── 建一篇已发布文章（后续子域操作的对象）──
	created := do(t, author, http.MethodPost, "/api/articles",
		[]byte(`{"title":"端到端文章","content":"正文","status":2}`))
	respCode(t, created, "建文章")
	articleID := dataOf(t, created)["id"].(string)
	// create 的 status=2 是"审核中"；这里直接置为已发布，专注验证子域端点。
	if err := db.Exec("UPDATE article_models SET publish_status = 3 WHERE id = ?", articleID).Error; err != nil {
		t.Fatalf("置为已发布失败: %v", err)
	}
	if got := queryInt(t, db, "SELECT publish_status FROM article_models WHERE id = ?", articleID); got != 3 {
		t.Fatalf("置为已发布未生效: %d", got)
	}
	if got := queryString(t, db, "SELECT visibility_status FROM article_models WHERE id = ?", articleID); got != "visible" {
		t.Fatalf("visibility 异常: %q", got)
	}

	// ── 点赞（读者）──
	respCode(t, do(t, reader, http.MethodPut, "/api/articles/"+articleID+"/digg", nil), "点赞")

	// ── 浏览上报 + 浏览历史 ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/view",
		[]byte(fmt.Sprintf(`{"article_id":%s}`, articleID))), "浏览上报")
	if !containsArticleID(listOf(t, do(t, author, http.MethodGet, "/api/articles/history", nil)), articleID) {
		t.Fatal("历史列表缺刚浏览的文章")
	}
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/history",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, articleID))), "删历史")

	// ── 收藏（读者收藏作者文章）→ 收藏内容 → 取消收藏 ──
	respCode(t, do(t, reader, http.MethodPost, "/api/articles/favorite",
		[]byte(fmt.Sprintf(`{"article_id":%s}`, articleID))), "收藏文章")
	readerFolderID := defaultFolderID(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite?user_id=200", nil)))
	if readerFolderID == "" {
		t.Fatal("读者默认收藏夹未创建")
	}
	if !containsArticleID(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite/contents?favorite_id="+readerFolderID, nil)), articleID) {
		t.Fatal("收藏内容缺刚收藏的文章")
	}
	respCode(t, do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
		[]byte(fmt.Sprintf(`{"articles":[%s],"favorite_id":%s}`, articleID, readerFolderID))), "取消收藏")

	// ── 置顶：设 → 列表 → 取消 ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/top",
		[]byte(fmt.Sprintf(`{"article_ids":[%s]}`, articleID))), "置顶")
	if !containsNumericID(t, do(t, author, http.MethodGet, "/api/articles/top?user_id=100", nil), articleID) {
		t.Fatal("置顶列表缺文章")
	}
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/top",
		[]byte(fmt.Sprintf(`{"article_ids":[%s]}`, articleID))), "取消置顶")

	// ── 管理员改可见性（用 DB 断言，避免被可见性规则干扰）──
	respCode(t, do(t, admin, http.MethodPost, "/api/articles/"+articleID+"/admin/hide", nil), "隐藏文章")
	if got := queryString(t, db, "SELECT visibility_status FROM article_models WHERE id = ?", articleID); got != "admin_hidden" {
		t.Fatalf("隐藏未生效: %q", got)
	}
	respCode(t, do(t, admin, http.MethodPost, "/api/articles/"+articleID+"/admin/show", nil), "恢复文章")
	if got := queryString(t, db, "SELECT visibility_status FROM article_models WHERE id = ?", articleID); got == "admin_hidden" {
		t.Fatalf("恢复未生效: %q", got)
	}

	// ── 更新（放在交互之后：发布后编辑会按规则打回审核中）──
	respCode(t, do(t, author, http.MethodPut, "/api/articles/"+articleID,
		[]byte(`{"title":"端到端文章-改名"}`)), "改文章")
	detail := dataOf(t, do(t, author, http.MethodGet, "/api/articles/"+articleID, nil))
	if detail["title"] != "端到端文章-改名" {
		t.Fatalf("改标题未生效: %v", detail["title"])
	}

	// ── 审核任务：新建审核中文章 → 列表 → 通过 ──
	reviewCreated := do(t, author, http.MethodPost, "/api/articles",
		[]byte(`{"title":"待审文章","content":"正文","status":2}`))
	respCode(t, reviewCreated, "建待审文章")
	reviewArticleID := dataOf(t, reviewCreated)["id"].(string)

	taskID := ""
	for _, task := range listOf(t, do(t, admin, http.MethodGet, "/api/article-review", nil)) {
		if task["article_id"] == reviewArticleID {
			taskID, _ = task["id"].(string)
		}
	}
	if taskID == "" {
		t.Fatal("审核任务列表缺任务")
	}
	respCode(t, do(t, admin, http.MethodPost, "/api/article-review-tasks/"+taskID+"/review",
		[]byte(`{"status":3}`)), "审核通过")
	if got := queryString(t, db, "SELECT publish_status FROM article_models WHERE id = ?", reviewArticleID); got != "3" {
		t.Fatalf("审核通过后文章未发布: %q", got)
	}

	// ── 删除文章（作者）──
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/"+articleID, nil), "删文章")
	gone := do(t, author, http.MethodGet, "/api/articles/"+articleID, nil)
	if code, _ := gone["code"].(float64); code == 0 {
		t.Fatalf("删除后仍能读到文章: %v", gone)
	}
}

// respCode 断言业务 code 为 0。
func respCode(t *testing.T, resp map[string]any, label string) {
	t.Helper()
	if code, _ := resp["code"].(float64); code != 0 {
		t.Fatalf("%s 失败: %v", label, resp)
	}
}

// dataOf 取响应 data。
func dataOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	d, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺 data: %v", resp)
	}
	return d
}

// listOf 取 data.list。
func listOf(t *testing.T, resp map[string]any) []map[string]any {
	t.Helper()
	raw, _ := dataOf(t, resp)["list"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// findIDByTitle 在列表里按 title 找 id（ID 序列化成字符串）。
func findIDByTitle(items []map[string]any, title string) string {
	for _, it := range items {
		if it["title"] == title {
			if id, ok := it["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

// defaultFolderID 取默认收藏夹 ID。
func defaultFolderID(items []map[string]any) string {
	for _, it := range items {
		if v, ok := it["is_default"].(bool); ok && v {
			if id, ok := it["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

// containsArticleID 判断列表里是否有该 article_id。
func containsArticleID(items []map[string]any, id string) bool {
	for _, it := range items {
		if it["article_id"] == id {
			return true
		}
	}
	return false
}

// containsNumericID 判断 list 里是否有该数字 ID（置顶接口返回的是 ID 数组）。
func containsNumericID(t *testing.T, resp map[string]any, id string) bool {
	t.Helper()
	raw, _ := dataOf(t, resp)["list"].([]any)
	for _, v := range raw {
		switch n := v.(type) {
		case float64:
			if fmt.Sprint(int64(n)) == id {
				return true
			}
		case string:
			if n == id {
				return true
			}
		}
	}
	return false
}

// queryString 读单列字符串，用于断言 DB 副作用。
func queryString(t *testing.T, db *gorm.DB, query string, args ...any) string {
	t.Helper()
	var out string
	if err := db.Raw(query, args...).Scan(&out).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	return out
}

// queryInt 读单列整数。
func queryInt(t *testing.T, db *gorm.DB, query string, args ...any) int {
	t.Helper()
	var out int
	if err := db.Raw(query, args...).Scan(&out).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	return out
}
