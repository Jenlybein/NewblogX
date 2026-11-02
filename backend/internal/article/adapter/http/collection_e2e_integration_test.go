//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是「合集最小闭环」的端到端契约：建合集 → 收录一篇 → 读者侧可见。
//
// 走真实 HTTP + 真 MySQL，断言的是**外部行为**（响应里的业务码与条目、顺序），
// 不是仓储方法被调了几次。这是 #102 的四条验收里唯一能一次验完的路径。

// migrateCollections 建出合集闭环需要的表（比共享的 migrate 多 schema.Collection）。
func migrateCollectionsHTTP(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection,
	)
	return db
}

// seedCollectionArticle 插入一篇已发布且可见的文章。
func seedCollectionArticle(t *testing.T, db *gorm.DB, id, authorID uint64) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          fmt.Sprintf("系列文章 %c", 'A'+rune(id)),
		Abstract:       "摘要",
		Content:        "正文",
		AuthorID:       authorID,
		ViewCount:      10,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
}

// envelope 是响应外壳（只看 code / data）。
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// decode 解一层外壳。
func decode(t *testing.T, raw []byte) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("解析响应失败: %v，原始: %s", err, raw)
	}
	return env
}

// TestCollectionMinimalLoop 建合集 → 收录一篇 → 读者列表与详情可见 → 关展示后消失。
func TestCollectionMinimalLoop(t *testing.T) {
	t.Parallel()

	db := migrateCollectionsHTTP(t)
	seedAuthor(t, db, 100)
	seedCollectionArticle(t, db, 1, 100)

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	// 读者路由：匿名视图，用来证明"未展示的合集对读者不可见"。
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	// ① 建合集。
	createRaw := rawRequest(t, adminRouter, "POST", "/api/collections", []byte(`{
		"title": "Nuxt 部署实战",
		"description": "从零到上线",
		"cover": "",
		"displayed": true
	}`))
	createEnv := decode(t, createRaw)
	if createEnv.Code != 0 {
		t.Fatalf("建合集应成功，实际 code=%d msg=%s", createEnv.Code, createEnv.Msg)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(createEnv.Data, &created); err != nil {
		t.Fatalf("解析合集 ID 失败: %v", err)
	}
	if created.ID.IsZero() {
		t.Fatalf("建合集应返回 ID，原始: %s", createRaw)
	}

	// ② 收录一篇文章。
	includeRaw := rawRequest(t, adminRouter, "POST",
		fmt.Sprintf("/api/collections/%s/articles", created.ID),
		[]byte(`{"article_id":"1"}`))
	includeEnv := decode(t, includeRaw)
	if includeEnv.Code != 0 {
		t.Fatalf("收录文章应成功，实际 code=%d msg=%s", includeEnv.Code, includeEnv.Msg)
	}

	// ③ 读者列表能看到这张卡。
	listRaw := rawRequest(t, readerRouter, "GET", "/api/collections", nil)
	listEnv := decode(t, listRaw)
	if listEnv.Code != 0 {
		t.Fatalf("读者列表应成功，实际 code=%d msg=%s", listEnv.Code, listEnv.Msg)
	}
	var list struct {
		List []struct {
			ID           ctype.ID `json:"id"`
			Title        string   `json:"title"`
			ArticleCount int      `json:"article_count"`
			Cover        string   `json:"cover"`
		} `json:"list"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listEnv.Data, &list); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	if list.Count != 1 || len(list.List) != 1 {
		t.Fatalf("读者列表应有 1 个合集，实际 count=%d len=%d（响应 %s）",
			list.Count, len(list.List), listRaw)
	}
	if list.List[0].ID != created.ID || list.List[0].Title != "Nuxt 部署实战" {
		t.Fatalf("卡片字段不对: %+v", list.List[0])
	}
	if list.List[0].ArticleCount != 1 {
		t.Fatalf("卡片收录数应为 1，实际 %d", list.List[0].ArticleCount)
	}

	// ④ 读者详情能看到按顺序排列的文章。
	detailRaw := rawRequest(t, readerRouter, "GET",
		fmt.Sprintf("/api/collections/%s", created.ID), nil)
	detailEnv := decode(t, detailRaw)
	if detailEnv.Code != 0 {
		t.Fatalf("读者详情应成功，实际 code=%d msg=%s", detailEnv.Code, detailEnv.Msg)
	}
	var detail struct {
		ID       ctype.ID `json:"id"`
		Title    string   `json:"title"`
		Articles []struct {
			ArticleID ctype.ID `json:"article_id"`
			SortOrder int      `json:"sort_order"`
			Title     string   `json:"title"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(detailEnv.Data, &detail); err != nil {
		t.Fatalf("解析详情失败: %v", err)
	}
	if len(detail.Articles) != 1 {
		t.Fatalf("详情应有 1 篇，实际 %d（响应 %s）", len(detail.Articles), detailRaw)
	}
	if detail.Articles[0].SortOrder != 1 || detail.Articles[0].ArticleID.Uint64() != 1 {
		t.Fatalf("详情条目不对: %+v", detail.Articles[0])
	}

	// ⑤ 关掉展示：读者列表变空、详情按不存在处理。
	hideRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/collections/%s/display", created.ID), []byte(`{"displayed":false}`))
	if env := decode(t, hideRaw); env.Code != 0 {
		t.Fatalf("关闭展示应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}

	afterRaw := rawRequest(t, readerRouter, "GET", "/api/collections", nil)
	var afterList struct {
		List  []struct{} `json:"list"`
		Count int        `json:"count"`
	}
	if err := json.Unmarshal(decode(t, afterRaw).Data, &afterList); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	if afterList.Count != 0 || len(afterList.List) != 0 {
		t.Fatalf("未展示的合集不应出现在读者列表，实际 count=%d（响应 %s）",
			afterList.Count, afterRaw)
	}

	hiddenDetailRaw := rawRequest(t, readerRouter, "GET",
		fmt.Sprintf("/api/collections/%s", created.ID), nil)
	if env := decode(t, hiddenDetailRaw); env.Code == 0 {
		t.Fatalf("读者读未展示的合集应按不存在处理，实际成功（响应 %s）", hiddenDetailRaw)
	}
}

