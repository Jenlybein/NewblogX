//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是「项目」管理端的端到端契约（#197 / #218）：增删改、展示开关、
// 目录树整树保存与失效行清理、鉴权失败。
//
// #218 之后，项目的收录与顺序**只经目录树编辑器**：旧的扁平收录 / 排序
// 接口已随 #225 删除，本文件也不再断言扁平 `articles[]`。
// 读面统一读 `/outline-tree` 的嵌套树。
//
// 走真实 HTTP + 真 MySQL，断言的是**外部行为**（响应里的业务码与字段、
// 读者侧读面），不是仓储方法被调了几次。读者侧公开读的独立契约在
// project_e2e_integration_test.go（#193），本文件复用那边的树响应类型与读取 helper。

// authorForbiddenMsg 是既有鉴权错误码 `article.forbidden` 的对外文案。
// 项目写接口全部挂管理员组，未授权必须落在这条既有错误上，不能另造文案。
const authorForbiddenMsg = "无权操作该文章"

// createProjectAndGetID 建一个项目并返回新 ID。
func createProjectAndGetID(t *testing.T, r *gin.Engine, body string) ctype.ID {
	t.Helper()
	raw := rawRequest(t, r, "POST", "/api/projects", []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("建项目应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("解析项目 ID 失败: %v（响应 %s）", err, raw)
	}
	if created.ID.IsZero() {
		t.Fatalf("建项目应返回 ID，原始: %s", raw)
	}
	return created.ID
}

// projectCard 是读者列表与管理端列表共用的最小结构。
type projectCard struct {
	ID            ctype.ID `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Cover         string   `json:"cover"`
	Displayed     bool     `json:"displayed"`
	ArticleCount  int      `json:"article_count"`
	SortOrder     int      `json:"sort_order"`
	FallbackCover string   `json:"fallback_cover"`
}

// adminProjects 读后台项目列表。
func adminProjects(t *testing.T, r *gin.Engine) []projectCard {
	t.Helper()
	raw := rawRequest(t, r, "GET", "/api/projects/admin", nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("后台项目列表应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var data struct {
		List []projectCard `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析后台项目列表失败: %v（响应 %s）", err, raw)
	}
	return data.List
}

// readerProjects 读读者项目列表。
func readerProjects(t *testing.T, r *gin.Engine) []projectCard {
	t.Helper()
	raw := rawRequest(t, r, "GET", "/api/projects", nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("读者项目列表应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var data struct {
		List []projectCard `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析读者项目列表失败: %v（响应 %s）", err, raw)
	}
	return data.List
}

// assertProjectWriteForbidden 断言非管理员写接口全部落到既有鉴权错误码。
//
// 旧的扁平收录 / 排序接口不在清单里：本批的写入口是目录树整树保存。
// #276 的具名 GET 是**公开读**（读者详情页与左栏要用），不在鉴权探针清单里；
// 非管理员读它只能拿到读者读面或 has_tree=false，不会拿到后台读面。
func assertProjectWriteForbidden(t *testing.T, r *gin.Engine, id ctype.ID) {
	t.Helper()
	cases := []struct {
		name, method, path, body string
	}{
		{"新建", "POST", "/api/projects", `{"title":"偷偷建","description":"","cover":"","displayed":true}`},
		{"改", "PUT", "/api/projects/" + string(id), `{"title":"偷偷改"}`},
		{"删", "DELETE", "/api/projects/" + string(id), ""},
		{"展示开关", "PATCH", "/api/projects/" + string(id) + "/display", `{"displayed":true}`},
		{"保存目录树", "PUT", "/api/projects/" + string(id) + "/outline-tree", `{"nodes":[]}`},
		{"项目之间排序", "PATCH", "/api/projects/order", `{"project_ids":[]}`},
		{"后台列表", "GET", "/api/projects/admin", ""},
	}
	for _, c := range cases {
		var body []byte
		if c.body != "" {
			body = []byte(c.body)
		}
		env := decode(t, rawRequest(t, r, c.method, c.path, body))
		if env.Code == 0 || env.Msg != authorForbiddenMsg {
			t.Fatalf("非管理员%s应落到既有鉴权错误（%q），实际 code=%d msg=%q",
				c.name, authorForbiddenMsg, env.Code, env.Msg)
		}
	}
}

// TestProjectAdminLoop 建项目 → 树里收录 → 读者可见 → 改内容 → 展示开关 →
// 删项目；同一条链路上验证写接口的鉴权线。
func TestProjectAdminLoop(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedProjectArticle(t, db, 31, 100, "https://cdn.example/first.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 32, 100, "https://cdn.example/second.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 33, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	// 已登录但非管理员：中间件放行（假鉴权固定 admin claims），由业务层拒绝。
	authorRouter := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 100)

	// ① 建项目：初始不展示，先编排后公布。
	createRaw := rawRequest(t, adminRouter, "POST", "/api/projects", []byte(`{
		"title": "重构实战",
		"description": "从零到上线",
		"cover": "",
		"displayed": false
	}`))
	createEnv := decode(t, createRaw)
	if createEnv.Code != 0 {
		t.Fatalf("建项目应成功，实际 code=%d msg=%s", createEnv.Code, createEnv.Msg)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(createEnv.Data, &created); err != nil {
		t.Fatalf("解析项目 ID 失败: %v", err)
	}
	if created.ID.IsZero() {
		t.Fatalf("建项目应返回 ID，原始: %s", createRaw)
	}

	// 未展示时读者读不到；后台看得到。
	if cards := readerProjects(t, readerRouter); len(cards) != 0 {
		t.Fatalf("未展示的项目不应出现在读者列表，实际 %+v", cards)
	}
	if cards := adminProjects(t, adminRouter); len(cards) != 1 || cards[0].Displayed {
		t.Fatalf("后台应看到未展示的项目，实际 %+v", cards)
	}

	// ② 目录树里收录三篇（整树一次性提交）。
	nodes := mustSaveProjectOutline(t, adminRouter, created.ID, `{"nodes":[
		{"kind":2,"article_id":"31"},
		{"kind":2,"article_id":"32"},
		{"kind":2,"article_id":"33"}
	]}`)
	if len(nodes) != 3 {
		t.Fatalf("树里应有 3 个文章节点，实际 %+v", nodes)
	}
	// 同一棵树内重复挂同一篇文章被拒（可读错误，不是数据库 1062）。
	dupRaw := rawRequest(t, adminRouter, "PUT",
		fmt.Sprintf("/api/projects/%s/outline-tree", created.ID),
		[]byte(`{"nodes":[{"kind":2,"article_id":"31"},{"kind":2,"article_id":"31"}]}`))
	if env := decode(t, dupRaw); env.Code == 0 || env.Msg != "该文章已在目录树中" {
		t.Fatalf("重复收录应报【该文章已在目录树中】，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	if afterDup := projectOutlineArticleIDs(readProjectAdminOutline(t, adminRouter, created.ID)); len(afterDup) != 3 {
		t.Fatalf("被拒绝的重复提交不应改动树，实际 %v", afterDup)
	}

	// ③ 打开展示后读者可见：详情按树的顺序、篇数 3。
	patchRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/projects/%s/display", created.ID), []byte(`{"displayed":true}`))
	if env := decode(t, patchRaw); env.Code != 0 {
		t.Fatalf("打开展示应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	cards := readerProjects(t, readerRouter)
	if len(cards) != 1 || cards[0].ID != created.ID {
		t.Fatalf("读者卡片不对: %+v", cards)
	}
	// 卡片读面已切目录树：树里 3 篇公开、旧中间表为空也要显示 3 篇；
	// 封面回退取 DFS 第一个公开文章节点（31）的封面。
	if cards[0].ArticleCount != 3 {
		t.Fatalf("读者卡片篇数应为 3（树里 3 篇公开），实际 %d", cards[0].ArticleCount)
	}
	if cards[0].Cover != "https://cdn.example/first.png" {
		t.Fatalf("读者卡片应回退到首个公开文章节点的封面，实际 %q", cards[0].Cover)
	}
	// 后台卡片同口径。
	adminCard := adminProjects(t, adminRouter)[0]
	if adminCard.ArticleCount != 3 || adminCard.FallbackCover != "https://cdn.example/first.png" {
		t.Fatalf("后台卡片口径不对: %+v", adminCard)
	}
	detail := readProjectDetail(t, readerRouter, created.ID)
	if detail.ArticleCount != 3 {
		t.Fatalf("读者详情篇数应为 3，实际 %d", detail.ArticleCount)
	}
	if got := projectOutlineArticleIDs(detail.Outline.Nodes); len(got) != 3 ||
		got[0] != 31 || got[1] != 32 || got[2] != 33 {
		t.Fatalf("读者详情顺序不对: %v", got)
	}
	for i, want := range []uint64{31, 32, 33} {
		node := detail.Outline.Nodes[i]
		if node.ArticleID.Uint64() != want || node.SortOrder != i+1 {
			t.Fatalf("第 %d 篇应为 (%d, %d)，实际 %+v", i+1, want, i+1, node)
		}
	}

	// ④ 改内容：展示开关不被顺手覆盖（这里仍是打开的）。
	updateRaw := rawRequest(t, adminRouter, "PUT",
		fmt.Sprintf("/api/projects/%s", created.ID), []byte(`{
		"title": "重构实战（第二版）",
		"description": "改过的描述",
		"cover": "https://cdn.example/cover.png"
	}`))
	if env := decode(t, updateRaw); env.Code != 0 {
		t.Fatalf("改项目应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	updated := adminProjects(t, adminRouter)[0]
	if updated.Title != "重构实战（第二版）" || updated.Description != "改过的描述" ||
		updated.Cover != "https://cdn.example/cover.png" {
		t.Fatalf("改项目后字段不对: %+v", updated)
	}
	if !updated.Displayed {
		t.Fatalf("改内容不应顺手关掉展示开关: %+v", updated)
	}
	// 显式封面优先，回退封面仍单独给出（树里首个公开文章节点的封面）。
	if updated.ArticleCount != 3 || updated.FallbackCover != "https://cdn.example/first.png" {
		t.Fatalf("改封面后后台卡片不对: %+v", updated)
	}

	// ⑤ 关闭展示：读者列表与详情都读不到（按不存在处理），树节点保留。
	hideRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/projects/%s/display", created.ID), []byte(`{"displayed":false}`))
	if env := decode(t, hideRaw); env.Code != 0 {
		t.Fatalf("关闭展示应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	if cards := readerProjects(t, readerRouter); len(cards) != 0 {
		t.Fatalf("隐藏的项目不应出现在读者列表，实际 %+v", cards)
	}
	hiddenRaw := rawRequest(t, readerRouter, "GET", fmt.Sprintf("/api/projects/%s", created.ID), nil)
	if env := decode(t, hiddenRaw); env.Code == 0 || env.Msg != "项目不存在" {
		t.Fatalf("读者读未展示项目应报【项目不存在】，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	if nodes := readProjectAdminOutline(t, adminRouter, created.ID); len(nodes) != 3 {
		t.Fatalf("隐藏项目后树节点必须保留，实际 %+v", nodes)
	}

	// ⑥ 鉴权：非管理员与匿名写接口全部落到既有鉴权错误上。
	assertProjectWriteForbidden(t, authorRouter, created.ID)
	assertProjectWriteForbidden(t, readerRouter, created.ID)

	// ⑦ 删项目：后台与读者都不再出现，但文章本身还在。
	deleteRaw := rawRequest(t, adminRouter, "DELETE",
		fmt.Sprintf("/api/projects/%s", created.ID), nil)
	if env := decode(t, deleteRaw); env.Code != 0 {
		t.Fatalf("删项目应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	if cards := adminProjects(t, adminRouter); len(cards) != 0 {
		t.Fatalf("删掉的项目不应出现在后台列表，实际 %+v", cards)
	}
	articleRaw := rawRequest(t, readerRouter, "GET", "/api/articles/31", nil)
	if env := decode(t, articleRaw); env.Code != 0 {
		t.Fatalf("删项目不应删掉里面的文章，读者读文章失败: code=%d msg=%s", env.Code, env.Msg)
	}
}

// TestProjectAdminOrdering 项目之间仍是全量换序；项目内部顺序改由目录树
// 整树提交维护（不再有第二个「项目内排序」入口）。
func TestProjectAdminOrdering(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{41, 42, 43} {
		seedProjectArticle(t, db, id, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	}

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	first := createProjectAndGetID(t, adminRouter, `{"title":"第一个","description":"","cover":"","displayed":true}`)
	second := createProjectAndGetID(t, adminRouter, `{"title":"第二个","description":"","cover":"","displayed":true}`)
	third := createProjectAndGetID(t, adminRouter, `{"title":"第三个","description":"","cover":"","displayed":true}`)

	// 收录进第一个项目：41、42、43。
	nodes := mustSaveProjectOutline(t, adminRouter, first, `{"nodes":[
		{"kind":2,"article_id":"41"},
		{"kind":2,"article_id":"42"},
		{"kind":2,"article_id":"43"}
	]}`)
	if got := projectOutlineArticleIDs(nodes); len(got) != 3 || got[0] != 41 || got[2] != 43 {
		t.Fatalf("初始树顺序不对: %v", got)
	}

	// 项目之间换序：第三个 → 第一个 → 第二个（主表 sort_order，仍然有效）。
	orderRaw := rawRequest(t, adminRouter, "PATCH", "/api/projects/order",
		[]byte(fmt.Sprintf(`{"project_ids":[%q,%q,%q]}`, third, first, second)))
	if env := decode(t, orderRaw); env.Code != 0 {
		t.Fatalf("项目换序应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	cards := readerProjects(t, readerRouter)
	wantOrder := []ctype.ID{third, first, second}
	if len(cards) != 3 {
		t.Fatalf("读者列表应有 3 个项目，实际 %d", len(cards))
	}
	for i, want := range wantOrder {
		if cards[i].ID != want {
			t.Fatalf("换序后第 %d 个项目应为 %s，实际 %s", i+1, want, cards[i].ID)
		}
	}
	// 篇数同样来自目录树：第一个项目树里 3 篇公开。
	if cards[1].ArticleCount != 3 {
		t.Fatalf("第一个项目的卡片篇数应为 3，实际 %d", cards[1].ArticleCount)
	}

	// 项目内换序：整树提交 43 → 41 → 42（带 ID 就地更新，不新插行）。
	reorderBody := fmt.Sprintf(`{"nodes":[
		{"id":%q,"kind":2,"article_id":"43"},
		{"id":%q,"kind":2,"article_id":"41"},
		{"id":%q,"kind":2,"article_id":"42"}
	]}`, string(nodes[2].ID), string(nodes[0].ID), string(nodes[1].ID))
	reordered := mustSaveProjectOutline(t, adminRouter, first, reorderBody)
	if got := projectOutlineArticleIDs(reordered); len(got) != 3 || got[0] != 43 || got[2] != 42 {
		t.Fatalf("整树提交后顺序不对: %v", got)
	}
	detail := readProjectDetail(t, readerRouter, first)
	if got := projectOutlineArticleIDs(detail.Outline.Nodes); len(got) != 3 ||
		got[0] != 43 || got[1] != 41 || got[2] != 42 {
		t.Fatalf("项目内换序后读者详情顺序不对: %v", got)
	}
	for i, want := range []uint64{43, 41, 42} {
		node := detail.Outline.Nodes[i]
		if node.ArticleID.Uint64() != want || node.SortOrder != i+1 {
			t.Fatalf("换序后第 %d 篇应为 %d，实际 %+v", i+1, want, node)
		}
	}

	// 项目之间集合不一致整单拒绝，且顺序不动。
	projectMismatchRaw := rawRequest(t, adminRouter, "PATCH", "/api/projects/order",
		[]byte(fmt.Sprintf(`{"project_ids":[%q,%q]}`, third, first)))
	if env := decode(t, projectMismatchRaw); env.Code == 0 || env.Msg != "排序列表与当前项目内容不一致，请刷新后重试" {
		t.Fatalf("项目集合不一致应被整单拒绝，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	// 非管理员不能排项目，也不能保存目录树。
	authorRouter := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 100)
	if env := decode(t, rawRequest(t, authorRouter, "PATCH", "/api/projects/order",
		[]byte(fmt.Sprintf(`{"project_ids":[%q,%q,%q]}`, second, first, third)))); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员换序应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	if env := decode(t, rawRequest(t, authorRouter, "PUT",
		fmt.Sprintf("/api/projects/%s/outline-tree", first),
		[]byte(`{"nodes":[]}`))); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员保存目录树应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
}

// TestProjectAdminStalePlaceholderAndCleanup 失效行：读者侧消失、后台树上标记、
// 整树提交清理后只剩在架节点（序号压实为 1）。
func TestProjectAdminStalePlaceholderAndCleanup(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{51, 52, 53, 54} {
		seedProjectArticle(t, db, id, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	}

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	projectID := createProjectAndGetID(t, adminRouter,
		`{"title":"待整理的项目","description":"","cover":"","displayed":true}`)
	nodes := mustSaveProjectOutline(t, adminRouter, projectID, `{"nodes":[
		{"kind":2,"article_id":"51"},
		{"kind":2,"article_id":"52"},
		{"kind":2,"article_id":"53"},
		{"kind":2,"article_id":"54"}
	]}`)
	if len(nodes) != 4 {
		t.Fatalf("树里应有 4 个节点，实际 %+v", nodes)
	}

	// 收录后文章变化：52 退回草稿、53 作者隐藏、54 软删。
	if err := db.Table("article_models").Where("id = ?", 52).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if err := db.Table("article_models").Where("id = ?", 53).
		Update("visibility_status", string(schema.VisibilityUserHidden)).Error; err != nil {
		t.Fatalf("隐藏文章失败: %v", err)
	}
	if err := db.Table("article_models").Where("id = ?", 54).
		Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 读者详情只剩 51；篇数也只算 1。
	detail := readProjectDetail(t, readerRouter, projectID)
	if detail.ArticleCount != 1 {
		t.Fatalf("读者详情篇数应只算公开文章（1），实际 %d", detail.ArticleCount)
	}
	if got := projectOutlineArticleIDs(detail.Outline.Nodes); len(got) != 1 || got[0] != 51 {
		t.Fatalf("读者详情应只剩仍公开的 51，实际 %v", got)
	}

	// 后台保留 4 个节点并给出失效原因（与合集同一组枚举）。
	adminNodes := flattenProjectOutline(readProjectAdminOutline(t, adminRouter, projectID))
	if len(adminNodes) != 4 {
		t.Fatalf("后台应保留 4 个节点，实际 %d：%+v", len(adminNodes), adminNodes)
	}
	checked := []struct {
		index      int
		articleID  ctype.ID
		wantReason string
	}{
		{0, ctype.NewID(51), ""},
		{1, ctype.NewID(52), "not_published"},
		{2, ctype.NewID(53), "not_visible"},
	}
	for _, c := range checked {
		row := adminNodes[c.index]
		if row.ArticleID != c.articleID || row.StaleReason != c.wantReason {
			t.Fatalf("后台第 %d 个节点不对: %+v，期望 article=%s reason=%q",
				c.index+1, row, c.articleID, c.wantReason)
		}
		if row.Stale != (c.wantReason != "") {
			t.Fatalf("后台第 %d 个节点的 stale 标记不对: %+v", c.index+1, row)
		}
	}
	// 54 已软删：节点必须仍保留、保留标题，并把原因区分为 deleted（不是 missing）。
	if deleted := adminNodes[3]; deleted.ArticleID.Uint64() != 54 ||
		!deleted.Stale || deleted.StaleReason != "deleted" || deleted.ArticleTitle == "" {
		t.Fatalf("已删除文章的占位应保留标题并标 deleted，实际 %+v", deleted)
	}

	// 卡片读面与详情同口径：失效行不进数。
	if cards := readerProjects(t, readerRouter); len(cards) != 1 ||
		cards[0].ID != projectID || cards[0].ArticleCount != 1 {
		t.Fatalf("读者卡片篇数应只数仍公开的文章（1），实际 %+v", cards)
	}
	if cards := adminProjects(t, adminRouter); len(cards) != 1 || cards[0].ArticleCount != 1 {
		t.Fatalf("后台卡片篇数应与读者同口径（1），实际 %+v", cards)
	}

	// 整树提交清理：只提交 51，其余三行被软删，剩余序号压实为 1。
	cleanupBody := fmt.Sprintf(
		`{"nodes":[{"id":%q,"kind":2,"article_id":"51"}]}`, string(nodes[0].ID))
	remaining := mustSaveProjectOutline(t, adminRouter, projectID, cleanupBody)
	if len(remaining) != 1 || remaining[0].ArticleID.Uint64() != 51 ||
		remaining[0].SortOrder != 1 || remaining[0].Stale {
		t.Fatalf("清理后树里应只剩在架的 51（序号 1），实际 %+v", remaining)
	}

	// 幂等：再提交同一棵树，结果不变。
	if again := mustSaveProjectOutline(t, adminRouter, projectID, cleanupBody); len(again) != 1 {
		t.Fatalf("重复提交同一棵树应幂等，实际 %+v", again)
	}

	after := flattenProjectOutline(readProjectAdminOutline(t, adminRouter, projectID))
	if len(after) != 1 || after[0].ArticleID.Uint64() != 51 || after[0].Stale ||
		after[0].SortOrder != 1 {
		t.Fatalf("清理后后台应只剩在架的 51（序号 1），实际 %+v", after)
	}
	if detail := readProjectDetail(t, readerRouter, projectID); detail.ArticleCount != 1 ||
		len(detail.Outline.Nodes) != 1 || detail.Outline.Nodes[0].ArticleID.Uint64() != 51 {
		t.Fatalf("清理后读者详情不对: %+v", detail)
	}

	// 非管理员不能清理（整树保存）。具名 GET 是公开读：普通用户只能拿到
	// 读者读面，不会拿到后台保留的失效行（后台读面是管理员有意例外）。
	authorRouter := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 100)
	if env := decode(t, rawRequest(t, authorRouter, "PUT",
		fmt.Sprintf("/api/projects/%s/outline-tree", projectID), []byte(cleanupBody))); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员清理应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	// 具名 GET 对普通用户是读者读面：能成功，但不含后台保留的失效行。
	readerView := decode(t, rawRequest(t, authorRouter, "GET",
		fmt.Sprintf("/api/projects/%s/outline-tree", projectID), nil))
	if readerView.Code != 0 {
		t.Fatalf("普通用户读具名树应是公开读面，实际 code=%d msg=%q", readerView.Code, readerView.Msg)
	}
}

// TestProjectAdminOutlineIncludeRules 整树提交的收录校验：同一棵树内重复、
// 不可收录（草稿）与不存在的文章都整单拒绝，成功的收录按提交顺序追加。
//
// 旧的扁平批量收录接口不再有前端入口；新编辑器一次提交整棵树，因此没有了
// included / duplicated / not_includable / missing 的逐篇结局。
func TestProjectAdminOutlineIncludeRules(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedProjectArticle(t, db, 61, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 62, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 63, 100, "", schema.ArticleStatusDraft, schema.VisibilityVisible, at)

	adminRouter := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	readerRouter := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	projectID := createProjectAndGetID(t, adminRouter,
		`{"title":"批量收录","description":"","cover":"","displayed":true}`)
	// 先收 61。
	mustSaveProjectOutline(t, adminRouter, projectID, `{"nodes":[{"kind":2,"article_id":"61"}]}`)
	if got := projectOutlineArticleIDs(readProjectDetail(t, readerRouter, projectID).Outline.Nodes); len(got) != 1 || got[0] != 61 {
		t.Fatalf("先收录 61 后读者详情不对: %v", got)
	}

	// 重复收录：同一棵树内一篇文章最多一个节点。
	dupRaw := rawRequest(t, adminRouter, "PUT", fmt.Sprintf("/api/projects/%s/outline-tree", projectID),
		[]byte(`{"nodes":[{"kind":2,"article_id":"61"},{"kind":2,"article_id":"61"}]}`))
	if env := decode(t, dupRaw); env.Code == 0 || env.Msg != "该文章已在目录树中" {
		t.Fatalf("重复收录应被拒，实际 code=%d msg=%q", env.Code, env.Msg)
	}

	// 草稿不可收录：整单拒绝，已有树不变。
	draftRaw := rawRequest(t, adminRouter, "PUT", fmt.Sprintf("/api/projects/%s/outline-tree", projectID),
		[]byte(`{"nodes":[{"kind":2,"article_id":"61"},{"kind":2,"article_id":"63"}]}`))
	if env := decode(t, draftRaw); env.Code == 0 || env.Msg != "只有已发布且可见的文章可以收录" {
		t.Fatalf("收录草稿应被拒，实际 code=%d msg=%q", env.Code, env.Msg)
	}

	// 不存在的文章同样整单拒绝。
	missingRaw := rawRequest(t, adminRouter, "PUT", fmt.Sprintf("/api/projects/%s/outline-tree", projectID),
		[]byte(`{"nodes":[{"kind":2,"article_id":"61"},{"kind":2,"article_id":"999"}]}`))
	if env := decode(t, missingRaw); env.Code == 0 || env.Msg != "只有已发布且可见的文章可以收录" {
		t.Fatalf("收录不存在的文章应被拒，实际 code=%d msg=%q", env.Code, env.Msg)
	}

	// 从失败状态恢复后仍只有 61。
	if got := projectOutlineArticleIDs(readProjectDetail(t, readerRouter, projectID).Outline.Nodes); len(got) != 1 || got[0] != 61 {
		t.Fatalf("被拒的提交不应改动树，实际 %v", got)
	}

	// 成功追加 62：顺序是提交顺序 61 → 62。
	mustSaveProjectOutline(t, adminRouter, projectID, `{"nodes":[
		{"kind":2,"article_id":"61"},
		{"kind":2,"article_id":"62"}
	]}`)
	detail := readProjectDetail(t, readerRouter, projectID)
	if got := projectOutlineArticleIDs(detail.Outline.Nodes); len(got) != 2 || got[0] != 61 || got[1] != 62 {
		t.Fatalf("批量收录后的顺序不对: %v", got)
	}
	for i, want := range []uint64{61, 62} {
		node := detail.Outline.Nodes[i]
		if node.ArticleID.Uint64() != want || node.SortOrder != i+1 {
			t.Fatalf("第 %d 篇应为 (%d, %d)，实际 %+v", i+1, want, i+1, node)
		}
	}
}
