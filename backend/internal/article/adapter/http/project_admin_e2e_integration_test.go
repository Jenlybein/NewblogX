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

// 本文件是「项目」管理端的端到端契约（#197）：增删改、展示开关、收录、
// 两级排序、失效行占位与一键清理、鉴权失败。
//
// 走真实 HTTP + 真 MySQL，断言的是**外部行为**（响应里的业务码与字段、
// 读者侧读面），不是仓储方法被调了几次。读者侧公开读的独立契约在
// project_e2e_integration_test.go（#193）。

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

// projectArticleItem 是读者详情里的一篇文章。
type projectArticleItem struct {
	ArticleID ctype.ID `json:"article_id"`
	SortOrder int      `json:"sort_order"`
	Title     string   `json:"title"`
}

// adminProjectItem 是后台收录行。
type adminProjectItem struct {
	ArticleID   ctype.ID `json:"article_id"`
	SortOrder   int      `json:"sort_order"`
	Title       string   `json:"title"`
	Stale       bool     `json:"stale"`
	StaleReason string   `json:"stale_reason"`
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

// readerProjectArticles 读读者项目详情里的文章（公开读，不含失效行）。
func readerProjectArticles(t *testing.T, r *gin.Engine, id ctype.ID) []projectArticleItem {
	t.Helper()
	raw := rawRequest(t, r, "GET", fmt.Sprintf("/api/projects/%s", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("读者项目详情应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var data struct {
		Articles []projectArticleItem `json:"articles"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析项目详情失败: %v（响应 %s）", err, raw)
	}
	return data.Articles
}

// adminProjectItems 读后台收录行（含失效占位）。
func adminProjectItems(t *testing.T, r *gin.Engine, id ctype.ID) []adminProjectItem {
	t.Helper()
	raw := rawRequest(t, r, "GET",
		fmt.Sprintf("/api/projects/%s/articles/admin", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("后台收录行应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	var data struct {
		List []adminProjectItem `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析后台收录行失败: %v（响应 %s）", err, raw)
	}
	return data.List
}

// assertProjectWriteForbidden 断言非管理员写接口全部落到既有鉴权错误码。
func assertProjectWriteForbidden(t *testing.T, r *gin.Engine, id ctype.ID) {
	t.Helper()
	cases := []struct {
		name, method, path, body string
	}{
		{"新建", "POST", "/api/projects", `{"title":"偷偷建","description":"","cover":"","displayed":true}`},
		{"改", "PUT", "/api/projects/" + string(id), `{"title":"偷偷改"}`},
		{"删", "DELETE", "/api/projects/" + string(id), ""},
		{"展示开关", "PATCH", "/api/projects/" + string(id) + "/display", `{"displayed":true}`},
		{"收录", "POST", "/api/projects/" + string(id) + "/articles", `{"article_id":"31"}`},
		{"移除", "DELETE", "/api/projects/" + string(id) + "/articles", `{"article_id":"31"}`},
		{"批量收录", "POST", "/api/projects/" + string(id) + "/articles/batch", `{"article_ids":["31"]}`},
		{"项目内排序", "PATCH", "/api/projects/" + string(id) + "/articles/order", `{"article_ids":[]}`},
		{"项目之间排序", "PATCH", "/api/projects/order", `{"project_ids":[]}`},
		{"后台列表", "GET", "/api/projects/admin", ""},
		{"后台收录行", "GET", "/api/projects/" + string(id) + "/articles/admin", ""},
		{"清理失效", "DELETE", "/api/projects/" + string(id) + "/admin/stale", ""},
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

// TestProjectAdminLoop 建项目 → 收录 → 读者可见 → 改内容 → 展示开关 →
// 删项目；同一条链路上验证写接口的鉴权线。
func TestProjectAdminLoop(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{31, 32, 33} {
		seedProjectArticle(t, db, id, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	}

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

	// ② 收录三篇。
	for _, id := range []string{"31", "32", "33"} {
		raw := rawRequest(t, adminRouter, "POST",
			fmt.Sprintf("/api/projects/%s/articles", created.ID),
			[]byte(fmt.Sprintf(`{"article_id":%q}`, id)))
		if env := decode(t, raw); env.Code != 0 {
			t.Fatalf("收录 %s 应成功，实际 code=%d msg=%s", id, env.Code, env.Msg)
		}
	}
	// 重复收录被拒（可读错误，不是 500）。
	dupRaw := rawRequest(t, adminRouter, "POST",
		fmt.Sprintf("/api/projects/%s/articles", created.ID), []byte(`{"article_id":"31"}`))
	if env := decode(t, dupRaw); env.Code == 0 || env.Msg != "该文章已在项目中" {
		t.Fatalf("重复收录应报【已在项目中】，实际 code=%d msg=%q", env.Code, env.Msg)
	}

	// ③ 打开展示后读者可见：卡片篇数 3、详情按收录顺序。
	patchRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/projects/%s/display", created.ID), []byte(`{"displayed":true}`))
	if env := decode(t, patchRaw); env.Code != 0 {
		t.Fatalf("打开展示应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	cards := readerProjects(t, readerRouter)
	if len(cards) != 1 || cards[0].ID != created.ID || cards[0].ArticleCount != 3 {
		t.Fatalf("读者卡片不对: %+v", cards)
	}
	detailArticles := readerProjectArticles(t, readerRouter, created.ID)
	if len(detailArticles) != 3 {
		t.Fatalf("读者详情应有 3 篇，实际 %d", len(detailArticles))
	}
	for i, want := range []uint64{31, 32, 33} {
		if detailArticles[i].ArticleID.Uint64() != want || detailArticles[i].SortOrder != i+1 {
			t.Fatalf("第 %d 篇应为 (%d, %d)，实际 %+v", i+1, want, i+1, detailArticles[i])
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

	// ⑤ 关闭展示：读者列表与详情都读不到（按不存在处理），收录行保留。
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
	if items := adminProjectItems(t, adminRouter, created.ID); len(items) != 3 {
		t.Fatalf("隐藏项目后收录行必须保留，实际 %d", len(items))
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

// TestProjectAdminOrdering 两级排序都是全量替换：失败整单拒绝、成功后读者
// 列表（项目之间）与读者详情（项目内）按新顺序返回。
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
	for _, id := range []string{"41", "42", "43"} {
		raw := rawRequest(t, adminRouter, "POST",
			fmt.Sprintf("/api/projects/%s/articles", first),
			[]byte(fmt.Sprintf(`{"article_id":%q}`, id)))
		if env := decode(t, raw); env.Code != 0 {
			t.Fatalf("收录 %s 应成功，实际 %s", id, raw)
		}
	}

	// 项目之间换序：第三个 → 第一个 → 第二个。
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

	// 项目内换序：43 → 41 → 42。
	innerRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/projects/%s/articles/order", first),
		[]byte(`{"article_ids":["43","41","42"]}`))
	if env := decode(t, innerRaw); env.Code != 0 {
		t.Fatalf("项目内换序应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}
	articles := readerProjectArticles(t, readerRouter, first)
	for i, want := range []uint64{43, 41, 42} {
		if articles[i].ArticleID.Uint64() != want || articles[i].SortOrder != i+1 {
			t.Fatalf("项目内换序后第 %d 篇应为 %d，实际 %+v", i+1, want, articles[i])
		}
	}

	// 集合不一致整单拒绝，且顺序不动。
	mismatchRaw := rawRequest(t, adminRouter, "PATCH",
		fmt.Sprintf("/api/projects/%s/articles/order", first),
		[]byte(`{"article_ids":["41"]}`))
	if env := decode(t, mismatchRaw); env.Code == 0 || env.Msg != "排序列表与当前项目内容不一致，请刷新后重试" {
		t.Fatalf("缺项排序应被整单拒绝，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	if articles := readerProjectArticles(t, readerRouter, first); articles[0].ArticleID.Uint64() != 43 {
		t.Fatalf("被拒绝的换序不应改动数据，实际 %+v", articles)
	}

	projectMismatchRaw := rawRequest(t, adminRouter, "PATCH", "/api/projects/order",
		[]byte(fmt.Sprintf(`{"project_ids":[%q,%q]}`, third, first)))
	if env := decode(t, projectMismatchRaw); env.Code == 0 || env.Msg != "排序列表与当前项目内容不一致，请刷新后重试" {
		t.Fatalf("项目集合不一致应被整单拒绝，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	// 非管理员不能排。
	authorRouter := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 100)
	if env := decode(t, rawRequest(t, authorRouter, "PATCH", "/api/projects/order",
		[]byte(fmt.Sprintf(`{"project_ids":[%q,%q,%q]}`, second, first, third)))); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员换序应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
}

// TestProjectAdminStalePlaceholderAndCleanup 失效行：读者侧消失、后台保留占位、
// 一键清理幂等（第二次为 0）。
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
	for _, id := range []string{"51", "52", "53", "54"} {
		raw := rawRequest(t, adminRouter, "POST",
			fmt.Sprintf("/api/projects/%s/articles", projectID),
			[]byte(fmt.Sprintf(`{"article_id":%q}`, id)))
		if env := decode(t, raw); env.Code != 0 {
			t.Fatalf("收录 %s 应成功，实际 %s", id, raw)
		}
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

	// 读者详情只剩 51；卡片计数也只算 1。
	if articles := readerProjectArticles(t, readerRouter, projectID); len(articles) != 1 || articles[0].ArticleID.Uint64() != 51 {
		t.Fatalf("读者详情应只剩仍公开的 51，实际 %+v", articles)
	}
	if cards := readerProjects(t, readerRouter); len(cards) != 1 || cards[0].ArticleCount != 1 {
		t.Fatalf("卡片收录数应只算公开文章（1），实际 %+v", cards)
	}

	// 后台保留 4 行占位并给出失效原因（与合集同一组枚举）。
	items := adminProjectItems(t, adminRouter, projectID)
	if len(items) != 4 {
		t.Fatalf("后台应保留 4 个占位，实际 %d", len(items))
	}
	wantReasons := []string{"", "not_published", "not_visible", "deleted"}
	for i, reason := range wantReasons {
		if items[i].Stale != (reason != "") || items[i].StaleReason != reason {
			t.Fatalf("第 %d 行失效标记不符: %+v，期望 reason=%q", i+1, items[i], reason)
		}
	}
	if items[3].Title == "" {
		t.Fatalf("已删除文章的占位丢了标题: %+v", items[3])
	}

	// 一键清理：只删 3 条失效行。
	var cleanup struct {
		Removed int `json:"removed"`
	}
	cleanupRaw := rawRequest(t, adminRouter, "DELETE",
		fmt.Sprintf("/api/projects/%s/admin/stale", projectID), nil)
	if err := json.Unmarshal(decode(t, cleanupRaw).Data, &cleanup); err != nil {
		t.Fatalf("解析清理结果失败: %v", err)
	}
	if cleanup.Removed != 3 {
		t.Fatalf("应清理 3 条失效行，实际 %d（响应 %s）", cleanup.Removed, cleanupRaw)
	}

	// 幂等：第二次清理返回 0 且不动在架文章。
	againRaw := rawRequest(t, adminRouter, "DELETE",
		fmt.Sprintf("/api/projects/%s/admin/stale", projectID), nil)
	if err := json.Unmarshal(decode(t, againRaw).Data, &cleanup); err != nil {
		t.Fatalf("解析二次清理结果失败: %v", err)
	}
	if cleanup.Removed != 0 {
		t.Fatalf("第二次清理应为 0，实际 %d", cleanup.Removed)
	}
	after := adminProjectItems(t, adminRouter, projectID)
	if len(after) != 1 || after[0].ArticleID.Uint64() != 51 || after[0].Stale {
		t.Fatalf("清理后后台应只剩在架的 51，实际 %+v", after)
	}
	if after[0].SortOrder != 1 {
		t.Fatalf("清理后剩余序号应压实为 1，实际 %d", after[0].SortOrder)
	}

	// 非管理员不能清理、也不能读后台收录行。
	authorRouter := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 100)
	if env := decode(t, rawRequest(t, authorRouter, "DELETE",
		fmt.Sprintf("/api/projects/%s/admin/stale", projectID), nil)); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员清理应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
	if env := decode(t, rawRequest(t, authorRouter, "GET",
		fmt.Sprintf("/api/projects/%s/articles/admin", projectID), nil)); env.Code == 0 || env.Msg != authorForbiddenMsg {
		t.Fatalf("非管理员读后台收录行应落到既有鉴权错误，实际 code=%d msg=%q", env.Code, env.Msg)
	}
}

// TestProjectAdminBatchInclude 批量收录逐篇返回结局：重复 / 不可收录 /
// 不存在都不让整单失败，成功的按提交顺序排到末尾。
func TestProjectAdminBatchInclude(t *testing.T) {
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
	// 先单篇收 61，批量里它会被认成重复。
	if env := decode(t, rawRequest(t, adminRouter, "POST",
		fmt.Sprintf("/api/projects/%s/articles", projectID), []byte(`{"article_id":"61"}`))); env.Code != 0 {
		t.Fatalf("单篇收录应成功，实际 code=%d msg=%s", env.Code, env.Msg)
	}

	batchRaw := rawRequest(t, adminRouter, "POST",
		fmt.Sprintf("/api/projects/%s/articles/batch", projectID),
		[]byte(`{"article_ids":["62","61","63","999"]}`))
	batchEnv := decode(t, batchRaw)
	if batchEnv.Code != 0 {
		t.Fatalf("批量收录应成功，实际 code=%d msg=%s", batchEnv.Code, batchEnv.Msg)
	}
	var batch struct {
		Results []struct {
			ArticleID ctype.ID `json:"article_id"`
			Outcome   string   `json:"outcome"`
		} `json:"results"`
		Included   int `json:"included"`
		Duplicated int `json:"duplicated"`
		Rejected   int `json:"rejected"`
	}
	if err := json.Unmarshal(batchEnv.Data, &batch); err != nil {
		t.Fatalf("解析批量结果失败: %v", err)
	}
	wantOutcomes := []string{"included", "duplicated", "not_includable", "missing"}
	for i, want := range wantOutcomes {
		if batch.Results[i].Outcome != want {
			t.Fatalf("第 %d 篇结局应为 %s，实际 %+v", i+1, want, batch.Results[i])
		}
	}
	if batch.Included != 1 || batch.Duplicated != 1 || batch.Rejected != 2 {
		t.Fatalf("批量汇总不对: %+v", batch)
	}

	if articles := readerProjectArticles(t, readerRouter, projectID); len(articles) != 2 ||
		articles[0].ArticleID.Uint64() != 61 || articles[1].ArticleID.Uint64() != 62 {
		t.Fatalf("批量收录后的顺序不对: %+v", articles)
	}
}
