//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是「合集最小闭环」的端到端契约：建合集 → 树里挂文章 → 读者侧可见。
//
// #219 契约破坏性变更后，合集详情返回**嵌套目录树**（`outline`），扁平
// `articles[]` 已退役。种子直接写 `content_outline_node_models`
// （schema.InsertOutlineNode），不再往已冻结的 `collection_article_models`
// 写行——旧中间表在本批不读不写、不迁移、不 DROP。

// migrateCollectionsHTTP 建出合集闭环需要的表（比共享的 migrate 多 schema.Collection
// 与 schema.Outline）。
func migrateCollectionsHTTP(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection, schema.Outline,
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

// seedCollectionOutlineNode 往合集树里播种一行。
func seedCollectionOutlineNode(t *testing.T, db *gorm.DB, spec schema.OutlineNodeSpec) {
	t.Helper()
	spec.OwnerKind = int(outline.OwnerCollection)
	schema.InsertOutlineNode(t, db, spec)
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

// collectionOutlineNode 是详情 / 后台树响应里的一行（嵌套）。
//
// 文章节点：kind=2、article_id 非空、title 为空、文章标题与封面在
// article_title / article_cover；文件夹节点：kind=1、title 是文件夹标题。
type collectionOutlineNode struct {
	ID           ctype.ID                `json:"id"`
	ParentID     ctype.ID                `json:"parent_id"`
	Kind         int                     `json:"kind"`
	Title        string                  `json:"title"`
	ArticleID    ctype.ID                `json:"article_id"`
	SortOrder    int                     `json:"sort_order"`
	ArticleTitle string                  `json:"article_title"`
	ArticleCover string                  `json:"article_cover"`
	Stale        bool                    `json:"stale"`
	StaleReason  string                  `json:"stale_reason"`
	Children     []collectionOutlineNode `json:"children"`
}

// flattenCollectionOutline 按 DFS 把嵌套树拍平成一行行，方便断言顺序与失效原因。
func flattenCollectionOutline(items []collectionOutlineNode) []collectionOutlineNode {
	var out []collectionOutlineNode
	var walk func([]collectionOutlineNode)
	walk = func(rows []collectionOutlineNode) {
		for _, row := range rows {
			out = append(out, row)
			walk(row.Children)
		}
	}
	walk(items)
	return out
}

// readCollectionDetail 读读者详情，返回目录树、篇数与最终封面。
func readCollectionDetail(
	t *testing.T, r *gin.Engine, id ctype.ID,
) (nodes []collectionOutlineNode, articleCount int, cover string) {
	t.Helper()
	raw := rawRequest(t, r, "GET", fmt.Sprintf("/api/collections/%s", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("读者详情应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var detail struct {
		Cover        string `json:"cover"`
		ArticleCount int    `json:"article_count"`
		Outline      struct {
			Nodes []collectionOutlineNode `json:"nodes"`
		} `json:"outline"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("解析详情失败: %v（响应 %s）", err, raw)
	}
	return detail.Outline.Nodes, detail.ArticleCount, detail.Cover
}

// readCollectionAdminOutline 读后台树编辑器的整棵树。
func readCollectionAdminOutline(
	t *testing.T, r *gin.Engine, id ctype.ID,
) []collectionOutlineNode {
	t.Helper()
	raw := rawRequest(t, r, "GET",
		fmt.Sprintf("/api/collections/%s/outline/admin", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("后台目录树应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var data struct {
		Nodes []collectionOutlineNode `json:"nodes"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析后台目录树失败: %v（响应 %s）", err, raw)
	}
	return data.Nodes
}

// saveCollectionOutline 整棵树一次性提交，返回保存后的根层节点。
func saveCollectionOutline(
	t *testing.T, r *gin.Engine, id ctype.ID, body string,
) []collectionOutlineNode {
	t.Helper()
	raw := rawRequest(t, r, "PUT",
		fmt.Sprintf("/api/collections/%s/outline", id), []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("保存目录树应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var data struct {
		Nodes []collectionOutlineNode `json:"nodes"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析保存结果失败: %v（响应 %s）", err, raw)
	}
	return data.Nodes
}

// TestCollectionStalePlaceholderAndCleanup 失效行：读者侧消失、后台树上标记、
// 整树提交清理后只剩在架节点（空文件夹保留）。
//
// 三种成因（草稿 / 私有 / 删除）一次验完；清理后重复提交同一棵树不产生变化。
func TestCollectionStalePlaceholderAndCleanup(t *testing.T) {
	t.Parallel()

	db := migrateCollectionsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{21, 22, 23, 24} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             id,
			Title:          fmt.Sprintf("系列文章 %d", id),
			Abstract:       "摘要",
			Content:        "正文",
			AuthorID:       100,
			ViewCount:      int(id),
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      at,
			UpdatedAt:      at,
		})
	}

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	// 建合集。
	createRaw := rawRequest(t, adminRouter, "POST", "/api/collections", []byte(`{
		"title": "待整理的专题",
		"description": "",
		"cover": "",
		"displayed": true
	}`))
	if env := decode(t, createRaw); env.Code != 0 {
		t.Fatalf("建合集应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(decode(t, createRaw).Data, &created); err != nil {
		t.Fatalf("解析合集 ID 失败: %v", err)
	}

	// 播种目录树：「第一章」下挂 21–24，根层再放一个空文件夹「空章节」。
	ownerID := created.ID.Uint64()
	seedCollectionOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 7001, OwnerID: ownerID, Kind: int(outline.NodeKindFolder),
		Title: "第一章", SortOrder: 1, At: at,
	})
	for i, articleID := range []uint64{21, 22, 23, 24} {
		seedCollectionOutlineNode(t, db, schema.OutlineNodeSpec{
			ID: uint64(7002 + i), OwnerID: ownerID, ParentID: 7001,
			Kind: int(outline.NodeKindArticle), ArticleID: articleID,
			SortOrder: i + 1, At: at,
		})
	}
	seedCollectionOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 7006, OwnerID: ownerID, Kind: int(outline.NodeKindFolder),
		Title: "空章节", SortOrder: 2, At: at,
	})

	// 收录（挂树）后文章变化：22 退回草稿、23 作者隐藏、24 软删。
	if err := db.Table("article_models").Where("id = ?", 22).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if err := db.Table("article_models").Where("id = ?", 23).
		Update("visibility_status", string(schema.VisibilityUserHidden)).Error; err != nil {
		t.Fatalf("隐藏文章失败: %v", err)
	}
	if err := db.Table("article_models").Where("id = ?", 24).
		Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 读者详情：失效文章节点被过滤，空文件夹保留，篇数只算公开文章。
	nodes, articleCount, _ := readCollectionDetail(t, readerRouter, created.ID)
	if articleCount != 1 {
		t.Fatalf("读者详情篇数应只算公开文章（1），实际 %d", articleCount)
	}
	if len(nodes) != 2 {
		t.Fatalf("读者根层应是【第一章 + 空章节】，实际 %+v", nodes)
	}
	if nodes[0].Kind != int(outline.NodeKindFolder) || nodes[0].Title != "第一章" {
		t.Fatalf("第一个节点应是文件夹「第一章」，实际 %+v", nodes[0])
	}
	if len(nodes[0].Children) != 1 || nodes[0].Children[0].ArticleID.Uint64() != 21 {
		t.Fatalf("「第一章」下应只剩仍公开的 21，实际 %+v", nodes[0].Children)
	}
	if nodes[1].Kind != int(outline.NodeKindFolder) || nodes[1].Title != "空章节" ||
		len(nodes[1].Children) != 0 {
		t.Fatalf("「空章节」应作为空文件夹保留，实际 %+v", nodes[1])
	}

	// 后台保留全部节点并标记失效原因（与旧收录行同一组枚举）。
	adminNodes := flattenCollectionOutline(readCollectionAdminOutline(t, adminRouter, created.ID))
	if len(adminNodes) != 6 {
		t.Fatalf("后台应保留 6 个节点（第一章节 + 4 篇 + 空章节），实际 %d：%+v",
			len(adminNodes), adminNodes)
	}
	checked := []struct {
		index      int
		articleID  uint64
		wantReason string
	}{
		{1, 21, ""},
		{2, 22, "not_published"},
		{3, 23, "not_visible"},
	}
	for _, c := range checked {
		row := adminNodes[c.index]
		if row.ArticleID.Uint64() != c.articleID || row.StaleReason != c.wantReason {
			t.Fatalf("后台第 %d 个节点不对: %+v，期望 article=%d reason=%q",
				c.index, row, c.articleID, c.wantReason)
		}
		if row.Stale != (c.wantReason != "") {
			t.Fatalf("后台第 %d 个节点的 stale 标记不对: %+v", c.index, row)
		}
	}
	// 24 已软删：节点必须仍保留且标失效；具体原因取值由文章快照读面决定，
	// 这里只钉住「失效 + 有原因」，不把实现细节写进契约（口径差异记在 #221 报告）。
	if deleted := adminNodes[4]; deleted.ArticleID.Uint64() != 24 ||
		!deleted.Stale || deleted.StaleReason == "" {
		t.Fatalf("已删除文章的占位应保留并标失效，实际 %+v", deleted)
	}

	// 整树提交清理：只保留第一章 + 21 + 空章节，其余三行被软删。
	cleanupBody := `{"nodes":[
		{"id":"7001","kind":1,"title":"第一章","children":[{"id":"7002","kind":2,"article_id":"21"}]},
		{"id":"7006","kind":1,"title":"空章节"}
	]}`
	remaining := saveCollectionOutline(t, adminRouter, created.ID, cleanupBody)
	if len(remaining) != 2 || len(remaining[0].Children) != 1 ||
		remaining[0].Children[0].ArticleID.Uint64() != 21 {
		t.Fatalf("清理后树里应剩【第一章 → 21】+【空章节】，实际 %+v", remaining)
	}

	// 幂等：再提交同一棵树，结果不变。
	again := saveCollectionOutline(t, adminRouter, created.ID, cleanupBody)
	if len(again) != 2 || len(again[0].Children) != 1 {
		t.Fatalf("重复提交同一棵树应幂等，实际 %+v", again)
	}

	// 后台读回与读者详情都只剩在架的 21。
	after := flattenCollectionOutline(readCollectionAdminOutline(t, adminRouter, created.ID))
	if len(after) != 3 || after[1].ArticleID.Uint64() != 21 || after[1].Stale {
		t.Fatalf("清理后后台应是【第一章, 21, 空章节】，实际 %+v", after)
	}
	afterNodes, afterCount, _ := readCollectionDetail(t, readerRouter, created.ID)
	if afterCount != 1 || len(afterNodes) != 2 ||
		afterNodes[0].Children[0].ArticleID.Uint64() != 21 {
		t.Fatalf("清理后读者详情不对: count=%d nodes=%+v", afterCount, afterNodes)
	}
}

// TestCollectionMinimalLoop 建合集 → 树上挂一篇 → 读者列表与详情可见 →
// 关展示后消失 → 改内容 → 删合集（文章本身还在）。
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

	// ② 树里挂一篇文章（直接播种目录树，不碰已冻结的扁平中间表）。
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedCollectionOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 7200, OwnerID: created.ID.Uint64(), Kind: int(outline.NodeKindArticle),
		ArticleID: 1, SortOrder: 1, At: at,
	})

	// ③ 读者列表能看到这张卡（卡片篇数仍由列表读面决定，属于既有实现；
	// 详情篇数由本批的目录树读模型负责，见下一条）。
	listRaw := rawRequest(t, readerRouter, "GET", "/api/collections", nil)
	listEnv := decode(t, listRaw)
	if listEnv.Code != 0 {
		t.Fatalf("读者列表应成功，实际 code=%d msg=%s", listEnv.Code, listEnv.Msg)
	}
	var list struct {
		List []struct {
			ID    ctype.ID `json:"id"`
			Title string   `json:"title"`
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

	// ④ 读者详情是一棵目录树：1 篇文章节点、篇数 1、文章标题来自文章快照。
	nodes, articleCount, _ := readCollectionDetail(t, readerRouter, created.ID)
	if articleCount != 1 {
		t.Fatalf("详情篇数应为 1，实际 %d", articleCount)
	}
	if len(nodes) != 1 || nodes[0].Kind != int(outline.NodeKindArticle) ||
		nodes[0].ArticleID.Uint64() != 1 || nodes[0].SortOrder != 1 {
		t.Fatalf("详情目录树条目不对: %+v", nodes)
	}
	if nodes[0].ArticleTitle != "系列文章 B" {
		t.Fatalf("文章节点标题应来自文章快照，实际 %q", nodes[0].ArticleTitle)
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

	// ⑥ 改合集内容：标题 / 描述 / 封面都能改，展示开关不受影响（这里仍是关的）。
	updateRaw := rawRequest(t, adminRouter, "PUT",
		fmt.Sprintf("/api/collections/%s", created.ID), []byte(`{
		"title": "Nuxt 部署实战（第二版）",
		"description": "改过的描述",
		"cover": "https://cdn.example.com/cover.png"
	}`))
	if env := decode(t, updateRaw); env.Code != 0 {
		t.Fatalf("改合集应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}

	adminListRaw := rawRequest(t, adminRouter, "GET", "/api/collections/admin", nil)
	var adminList struct {
		List []struct {
			ID          ctype.ID `json:"id"`
			Title       string   `json:"title"`
			Cover       string   `json:"cover"`
			Description string   `json:"description"`
			Displayed   bool     `json:"displayed"`
		} `json:"list"`
	}
	if err := json.Unmarshal(decode(t, adminListRaw).Data, &adminList); err != nil {
		t.Fatalf("解析后台列表失败: %v", err)
	}
	if len(adminList.List) != 1 {
		t.Fatalf("后台列表应有 1 个合集（未展示也要在），实际 %d（响应 %s）",
			len(adminList.List), adminListRaw)
	}
	updated := adminList.List[0]
	if updated.Title != "Nuxt 部署实战（第二版）" || updated.Description != "改过的描述" ||
		updated.Cover != "https://cdn.example.com/cover.png" {
		t.Fatalf("改合集后字段不对: %+v", updated)
	}
	if updated.Displayed {
		t.Fatalf("改内容不应顺手打开展示开关: %+v", updated)
	}

	// ⑦ 删合集：后台列表与读者列表都不再出现，但文章本身还在。
	deleteRaw := rawRequest(t, adminRouter, "DELETE",
		fmt.Sprintf("/api/collections/%s", created.ID), nil)
	if env := decode(t, deleteRaw); env.Code != 0 {
		t.Fatalf("删合集应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}

	afterDeleteRaw := rawRequest(t, adminRouter, "GET", "/api/collections/admin", nil)
	var afterDelete struct {
		List []struct{} `json:"list"`
	}
	if err := json.Unmarshal(decode(t, afterDeleteRaw).Data, &afterDelete); err != nil {
		t.Fatalf("解析后台列表失败: %v", err)
	}
	if len(afterDelete.List) != 0 {
		t.Fatalf("删掉的合集不应出现在后台列表，实际 %d（响应 %s）",
			len(afterDelete.List), afterDeleteRaw)
	}

	articleRaw := rawRequest(t, readerRouter, "GET", "/api/articles/1", nil)
	if env := decode(t, articleRaw); env.Code != 0 {
		t.Fatalf("删合集不应删掉里面的文章，读者读文章失败: code=%d msg=%s", env.Code, env.Msg)
	}
}
