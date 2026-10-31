//go:build integration

package http_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是「热门置顶」的端到端契约：真 MySQL + 真路由 + 真 service。
//
// 只验三件普通单测覆盖不到的事：
//  1. 契约形状（路径、方法、信封、字段名）与前端将要消费的一致；
//  2. 管理路由真的挂在 AdminMiddleware 后面（普通用户进不去）；
//  3. 幂等/上限/失效这些语义穿透整条链路时仍然成立。

// itoa 把测试用的数值 ID 写成可读标题。
func itoa(v uint64) string { return strconv.FormatUint(v, 10) }

func mustData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺 data: %v", resp)
	}
	return data
}

func mustList(t *testing.T, resp map[string]any) []any {
	t.Helper()
	list, ok := mustData(t, resp)["list"].([]any)
	if !ok {
		t.Fatalf("响应缺 data.list: %v", resp)
	}
	return list
}

// hotTopIDs 读公开热门置顶，返回按顺序的 ID 字符串。
func hotTopIDs(t *testing.T, r *gin.Engine) []string {
	t.Helper()
	resp := do(t, r, http.MethodGet, "/api/articles/hot-top", nil)
	if code, _ := resp["code"].(float64); code != 0 {
		t.Fatalf("读热门置顶失败: %v", resp)
	}
	list := mustList(t, resp)
	out := make([]string, 0, len(list))
	for _, item := range list {
		card, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("卡片不是对象: %v", item)
		}
		id, _ := card["id"].(string)
		out = append(out, id)
	}
	return out
}

// TestHotTopEndToEnd 走完"设置 → 公开读 → 幂等 → 排序 → 取消 → 失效"全链路。
func TestHotTopEndToEnd(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedAuthor(t, db, 100)

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{11, 12} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:            id,
			Title:         "运营位文章 " + itoa(id),
			Abstract:      "摘要",
			Content:       "正文",
			AuthorID:      100,
			PublishStatus: schema.ArticleStatusPublished,
			Visibility:    schema.VisibilityVisible,
			CreatedAt:     at,
			UpdatedAt:     at,
		})
	}
	// 13 是草稿：不能置顶，用来验"只有已发布且可见"。
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            13,
		Title:         "草稿文章",
		Content:       "正文",
		AuthorID:      100,
		PublishStatus: schema.ArticleStatusDraft,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     at,
		UpdatedAt:     at,
	})

	adminRouter := newRouter(t, db, domain.AdminUser(ctype.NewID(900)))
	anonRouter := newRouter(t, db, domain.Anonymous())

	// 初始：空列表仍然是一个合法响应（信封齐全、list 为空数组）。
	if ids := hotTopIDs(t, anonRouter); len(ids) != 0 {
		t.Fatalf("初始应为空，实际 %v", ids)
	}

	// 设置 11、12。
	for _, id := range []string{"11", "12"} {
		resp := do(t, adminRouter, http.MethodPost, "/api/articles/"+id+"/admin/hot-top", nil)
		if code, _ := resp["code"].(float64); code != 0 {
			t.Fatalf("设置 %s 失败: %v", id, resp)
		}
	}
	if ids := hotTopIDs(t, anonRouter); len(ids) != 2 || ids[0] != "11" || ids[1] != "12" {
		t.Fatalf("公开列表应按设置顺序返回 [11 12]，实际 %v", ids)
	}

	// 幂等：重复设置不产生第二条，也不改顺序。
	if resp := do(t, adminRouter, http.MethodPost, "/api/articles/11/admin/hot-top", nil); resp["code"].(float64) != 0 {
		t.Fatalf("重复设置应成功: %v", resp)
	}
	if ids := hotTopIDs(t, anonRouter); len(ids) != 2 || ids[0] != "11" {
		t.Fatalf("重复设置不应改动列表，实际 %v", ids)
	}

	// 草稿不能置顶：可读错误。
	draftResp := do(t, adminRouter, http.MethodPost, "/api/articles/13/admin/hot-top", nil)
	if code, _ := draftResp["code"].(float64); code == 0 {
		t.Fatalf("草稿不应可置顶: %v", draftResp)
	}
	if msg, _ := draftResp["msg"].(string); msg != "只有已发布且可见的文章可以设为热门置顶" {
		t.Fatalf("草稿置顶的错误文案不符: %q", msg)
	}

	// 卡片字段：前端渲染首页块所需的都必须在。
	card := mustList(t, do(t, anonRouter, http.MethodGet, "/api/articles/hot-top", nil))[0].(map[string]any)
	for _, key := range []string{"id", "title", "abstract", "cover", "view_count", "author_nickname", "category_title"} {
		if _, ok := card[key]; !ok {
			t.Fatalf("卡片缺字段 %q: %v", key, card)
		}
	}

	// 后台列表含序号与失效标记。
	adminItems := mustList(t, do(t, adminRouter, http.MethodGet, "/api/articles/hot-top/admin", nil))
	if len(adminItems) != 2 {
		t.Fatalf("后台列表应有 2 行，实际 %d", len(adminItems))
	}
	first := adminItems[0].(map[string]any)
	if first["article_id"] != "11" || first["sort_order"].(float64) != 1 || first["stale"].(bool) {
		t.Fatalf("后台首行不符: %v", first)
	}

	// 全量顺序替换。
	patch := do(t, adminRouter, http.MethodPatch, "/api/articles/hot-top/order",
		[]byte(`{"article_ids":["12","11"]}`))
	if code, _ := patch["code"].(float64); code != 0 {
		t.Fatalf("重排失败: %v", patch)
	}
	if ids := hotTopIDs(t, anonRouter); ids[0] != "12" || ids[1] != "11" {
		t.Fatalf("重排后顺序应为 [12 11]，实际 %v", ids)
	}

	// 集合不一致：整单拒绝，且错误可读。
	bad := do(t, adminRouter, http.MethodPatch, "/api/articles/hot-top/order",
		[]byte(`{"article_ids":["12"]}`))
	if code, _ := bad["code"].(float64); code == 0 {
		t.Fatalf("集合不一致应失败: %v", bad)
	}
	if msg, _ := bad["msg"].(string); msg != "排序列表与当前热门置顶不一致，请刷新后重试" {
		t.Fatalf("重排失败文案不符: %q", msg)
	}

	// 取消一条：序号压实。
	if resp := do(t, adminRouter, http.MethodDelete, "/api/articles/11/admin/hot-top", nil); resp["code"].(float64) != 0 {
		t.Fatalf("取消置顶失败: %v", resp)
	}
	if ids := hotTopIDs(t, anonRouter); len(ids) != 1 || ids[0] != "12" {
		t.Fatalf("取消后应只剩 12，实际 %v", ids)
	}

	// 文章退回草稿：置顶行保留、首页消失、后台标失效。
	if err := db.Table("article_models").Where("id = ?", 12).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if ids := hotTopIDs(t, anonRouter); len(ids) != 0 {
		t.Fatalf("文章不再公开后首页应过滤掉，实际 %v", ids)
	}
	adminItems = mustList(t, do(t, adminRouter, http.MethodGet, "/api/articles/hot-top/admin", nil))
	if len(adminItems) != 1 {
		t.Fatalf("后台应仍能看到 1 条失效置顶，实际 %d", len(adminItems))
	}
	if stale, _ := adminItems[0].(map[string]any)["stale"].(bool); !stale {
		t.Fatalf("应当标为已失效: %v", adminItems[0])
	}

	// 失效行可以清理（文章还在，但已不公开）。
	if resp := do(t, adminRouter, http.MethodDelete, "/api/articles/12/admin/hot-top", nil); resp["code"].(float64) != 0 {
		t.Fatalf("清理失效置顶失败: %v", resp)
	}
	if ids := hotTopIDs(t, anonRouter); len(ids) != 0 {
		t.Fatalf("清理后应为空，实际 %v", ids)
	}
}

// TestHotTopAdminRoutesRejectOrdinaryUser 管理动作必须挂在管理员门禁后面。
//
// 文章详情路由用的是新链路：普通用户即使拿到令牌，写操作也只能停在 service 的
// IsAdmin 判定上（返回"无权操作该文章"），绝不能真的改动运营位。
func TestHotTopAdminRoutesRejectOrdinaryUser(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            11,
		Title:         "运营位文章 11",
		Content:       "正文",
		AuthorID:      100,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     at,
		UpdatedAt:     at,
	})

	userRouter := newRouter(t, db, domain.NormalUser(ctype.NewID(100)))
	adminRouter := newRouter(t, db, domain.AdminUser(ctype.NewID(900)))

	// 普通用户设置：被拒。
	resp := do(t, userRouter, http.MethodPost, "/api/articles/11/admin/hot-top", nil)
	if code, _ := resp["code"].(float64); code == 0 {
		t.Fatalf("普通用户不应能设置热门置顶: %v", resp)
	}
	if ids := hotTopIDs(t, adminRouter); len(ids) != 0 {
		t.Fatalf("被拒后不应落库，实际 %v", ids)
	}

	// 普通用户重排：被拒（空集合也不该被当成合法变更）。
	resp = do(t, userRouter, http.MethodPatch, "/api/articles/hot-top/order", []byte(`{"article_ids":[]}`))
	if code, _ := resp["code"].(float64); code == 0 {
		t.Fatalf("普通用户不应能重排: %v", resp)
	}

	// 普通用户读后台列表：被拒。
	resp = do(t, userRouter, http.MethodGet, "/api/articles/hot-top/admin", nil)
	if code, _ := resp["code"].(float64); code == 0 {
		t.Fatalf("普通用户不应能读后台列表: %v", resp)
	}
}
