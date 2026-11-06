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

// 本文件是 #276「拆树·迁移项目」的真 MySQL 契约用例：项目这条端到端路径只经
// 具名端点读写 project_outline_node_models；旧共享端点已随 #277 下线。
//
// 全部断言都跨 HTTP 接口这一条行为接缝：整树提交 → 项目详情页读同一棵树 →
// 阅读页左栏按 `?from=project:<id>` 找回同一棵树；同一棵项目树内一篇文章不
// 重复、允许与合集树跨树重复；管理员能打开未展示的项目（与合集口径一致），
// 匿名读者仍按「不存在」处理。不读源码文本，也不越过接口直接查树表断言实现。

// projectNamedTreePath 是 #274 实际选定的项目具名端点（契约路径不带 /api）。
func projectNamedTreePath(id ctype.ID) string {
	return fmt.Sprintf("/api/projects/%s/outline-tree", id)
}

// createDisplayedProjectForMigrate 通过后台接口建一个项目；displayed 由调用方给，
// 用来验「管理员能打开未展示项目」与「读者按不存在处理」两个口径。
func createDisplayedProjectForMigrate(t *testing.T, admin *gin.Engine, displayed bool) ctype.ID {
	t.Helper()
	body := fmt.Sprintf(`{
		"title":"迁移后的项目","description":"按项目成组阅读","cover":"","displayed":%t
	}`, displayed)
	raw := rawRequest(t, admin, "POST", "/api/projects", []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("建项目应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("解析项目 ID 失败: %v（响应 %s）", err, raw)
	}
	return created.ID
}

// requestProjectNamedTree 用项目具名端点整树提交（管理员）；响应只取后台读面节点。
func requestProjectNamedTree(t *testing.T, admin *gin.Engine, id ctype.ID, body string) []projectOutlineNode {
	t.Helper()
	raw := rawRequest(t, admin, "PUT", projectNamedTreePath(id), []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("项目具名整树提交应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var data struct {
		Nodes []projectOutlineNode `json:"nodes"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析项目具名树响应失败: %v（响应 %s）", err, raw)
	}
	return data.Nodes
}

// readProjectNamedTree 读项目具名端点的响应（匿名读者或管理员）。
func readProjectNamedTree(t *testing.T, r *gin.Engine, path string) namedOutlineTreeResponse {
	t.Helper()
	return requestNamedTree(t, r, "GET", path, nil)
}

// TestProjectNamedTreeEndToEndOverRealMySQL 验「后台编排 → 项目详情页 → 阅读页
// 左栏」找回的是同一棵项目树；篇数、封面回退、上一章/下一章、`?from=` 与失效
// 过滤口径与拆树前逐字一致，只是读写来源换成 project_outline_node_models。
func TestProjectNamedTreeEndToEndOverRealMySQL(t *testing.T) {
	t.Parallel()

	db := migrateNamedOutline(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedNamedOutlineArticles(t, db, at)

	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	projectID := createDisplayedProjectForMigrate(t, admin, true)
	treePath := projectNamedTreePath(projectID)

	// ① 后台编辑器整树提交：文件夹 + 两篇文章 + 根层一篇文章，写进项目具名表。
	saved := requestProjectNamedTree(t, admin, projectID, `{"nodes":[
		{"kind":1,"title":"第一章","children":[
			{"kind":2,"article_id":"21"},
			{"kind":2,"article_id":"22"}
		]},
		{"kind":2,"article_id":"23"}
	]}`)
	if len(flattenProjectOutline(saved)) != 4 {
		t.Fatalf("整树提交应落 4 个节点，实际 %d: %+v", len(flattenProjectOutline(saved)), saved)
	}

	// ② 项目详情页从具名端点拿到的树与后台编排一致；文章 21 退回草稿后读者
	// 读面过滤它、篇数只数仍公开的文章、封面回退到 DFS 第一篇公开文章。
	if err := db.Table("article_models").Where("id = ?", 21).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	detailTree := readProjectNamedTree(t, reader, treePath)
	if !detailTree.HasTree || detailTree.OwnerKind != 2 || detailTree.OwnerID != projectID {
		t.Fatalf("项目具名形状不对: %+v", detailTree)
	}
	if got := namedOutlineArticleIDs(detailTree.Nodes); len(got) != 2 || got[0] != 22 || got[1] != 23 {
		t.Fatalf("详情页应只读到仍公开的 [22,23]，实际 %+v", got)
	}
	if detailTree.ArticleCount != 2 || detailTree.Cover != "https://cdn.example.com/first-visible.png" {
		t.Fatalf("篇数/封面回退应来自读者读面，实际 count=%d cover=%q",
			detailTree.ArticleCount, detailTree.Cover)
	}

	// ④ 阅读页左栏按 `?from=project:<id>` 找回同一棵树，带当前文章 ID 求相邻文章；
	// 来路错指到别的专题时不串树。
	from := fmt.Sprintf("project:%s", projectID)
	fromPath := treePath + "?from=" + from + "&article_id=23"
	aside := readProjectNamedTree(t, reader, fromPath)
	if !aside.HasTree || aside.OwnerKind != 2 || aside.OwnerID != projectID {
		t.Fatalf("左栏应按来路找回项目树，实际 %+v", aside)
	}
	if aside.Current == nil || aside.Current.ArticleID.Uint64() != 23 ||
		aside.Previous == nil || aside.Previous.ArticleID.Uint64() != 22 || aside.Next != nil {
		t.Fatalf("左栏相邻文章应出自同一棵项目树（22 → 23 → 无），实际 %+v %+v %+v",
			aside.Previous, aside.Current, aside.Next)
	}
	otherPath := treePath + "?from=project:" + string(ctype.NewID(999999)) + "&article_id=23"
	if other := readProjectNamedTree(t, reader, otherPath); other.HasTree {
		t.Fatalf("来路指向别的项目时不应串树，实际 %+v", other)
	}
}

// TestProjectNamedTreeArticleUniquenessOverRealMySQL 验同一棵项目树内一篇文章
// 不重复出现（重复提交由服务端拒绝），且允许同一篇文章与合集树跨树重复。
func TestProjectNamedTreeArticleUniquenessOverRealMySQL(t *testing.T) {
	t.Parallel()

	db := migrateNamedOutline(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedNamedOutlineArticles(t, db, at)

	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	projectID := createDisplayedProjectForMigrate(t, admin, true)
	collectionID := createNamedOwner(t, admin, "/api/collections")

	// 一篇文章同时在项目树与合集树里：两张具名表互不顶掉。
	requestProjectNamedTree(t, admin, projectID,
		`{"nodes":[{"kind":2,"article_id":"22"},{"kind":2,"article_id":"24"}]}`)
	requestNamedTree(t, admin, "PUT",
		fmt.Sprintf("/api/collections/%s/outline-tree", collectionID),
		[]byte(`{"nodes":[{"kind":2,"article_id":"22"}]}`))

	projectTree := readProjectNamedTree(t, reader, projectNamedTreePath(projectID))
	collectionTree := readProjectNamedTree(t, reader,
		fmt.Sprintf("/api/collections/%s/outline-tree", collectionID))
	if len(projectTree.Nodes) != 2 || len(collectionTree.Nodes) != 1 {
		t.Fatalf("跨树重复应各自保留，实际项目 %+v 合集 %+v", projectTree.Nodes, collectionTree.Nodes)
	}
	if projectTree.ArticleCount != 2 || collectionTree.ArticleCount != 1 {
		t.Fatalf("跨树重复不应影响各自篇数，实际 %d / %d", projectTree.ArticleCount, collectionTree.ArticleCount)
	}

	// 同一棵项目树内重复挂同一篇文章：外部行为是整单被既有错误码拒绝，
	// 树仍是上一次成功编排的形状（没有第二份节点，也没有半套写入）。
	duplicated := `{"nodes":[{"kind":2,"article_id":"22"},{"kind":2,"article_id":"22"}]}`
	raw := rawRequest(t, admin, "PUT", projectNamedTreePath(projectID), []byte(duplicated))
	env := decode(t, raw)
	if env.Code == 0 {
		t.Fatalf("同一棵项目树内重复文章应被拒绝，实际成功（响应 %s）", raw)
	}
	if env.Msg != "该文章已在目录树中" {
		t.Fatalf("重复文章应落到既有错误码文案，实际 %q（响应 %s）", env.Msg, raw)
	}
	after := readProjectNamedTree(t, reader, projectNamedTreePath(projectID))
	if got := namedOutlineArticleIDs(after.Nodes); len(got) != 2 || got[0] != 22 || got[1] != 24 {
		t.Fatalf("被拒绝的整树提交不应改动项目树，实际 %+v", got)
	}
}

// TestProjectDetailAdminCanOpenUndisplayedOverRealMySQL 验复制漂移修复：管理员
// 经 GET /api/projects/{id} 能打开未展示的项目，且仍能从项目具名端点编排它的树；
// 匿名读者打开同一项目按「不存在」处理，不留「未发布 / 未展示」这类内部状态。
func TestProjectDetailAdminCanOpenUndisplayedOverRealMySQL(t *testing.T) {
	t.Parallel()

	db := migrateNamedOutline(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedNamedOutlineArticles(t, db, at)

	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	projectID := createDisplayedProjectForMigrate(t, admin, false)

	adminDetailRaw := rawRequest(t, admin, "GET", "/api/projects/"+string(projectID), nil)
	adminDetail := decode(t, adminDetailRaw)
	if adminDetail.Code != 0 {
		t.Fatalf("管理员应能打开未展示的项目，实际 code=%d msg=%s（响应 %s）",
			adminDetail.Code, adminDetail.Msg, adminDetailRaw)
	}

	// 后台编辑器读回的是「整棵后台读面」：未展示项目不能变成空树，失效行仍在。
	saved := requestProjectNamedTree(t, admin, projectID,
		`{"nodes":[{"kind":1,"title":"准备中","children":[{"kind":2,"article_id":"21"}]}]}`)
	if err := db.Table("article_models").Where("id = ?", 21).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	backstage := readProjectNamedTree(t, admin, projectNamedTreePath(projectID))
	if !backstage.HasTree || len(flattenCollectionOutline(backstage.Nodes)) != 2 {
		t.Fatalf("管理员应读回含失效行的整棵树，实际 %+v", backstage)
	}
	if stale := flattenCollectionOutline(backstage.Nodes)[1]; !stale.Stale || stale.StaleReason != "not_published" {
		t.Fatalf("失效行口径应保留 stale_reason，实际 %+v", stale)
	}
	if len(flattenProjectOutline(saved)) != 2 {
		t.Fatalf("整树提交响应应是后台读面，实际 %+v", saved)
	}
	if backstage.ArticleCount != 0 || backstage.Cover != "" || backstage.Current != nil {
		t.Fatalf("篇数/封面/相邻文章只对读者读面有意义，管理员读面应留零值，实际 %+v", backstage)
	}

	readerDetailRaw := rawRequest(t, reader, "GET", "/api/projects/"+string(projectID), nil)
	readerDetail := decode(t, readerDetailRaw)
	if readerDetail.Code == 0 || readerDetail.Msg != "项目不存在" {
		t.Fatalf("匿名读者打开未展示项目应按不存在处理，实际 code=%d msg=%q（响应 %s）",
			readerDetail.Code, readerDetail.Msg, readerDetailRaw)
	}
	readerTree := readProjectNamedTree(t, reader, projectNamedTreePath(projectID))
	if readerTree.HasTree {
		t.Fatalf("匿名读者不应读到未展示项目的树，实际 %+v", readerTree)
	}
}

// TestProjectNamedTreeFeedsListCardsOverRealMySQL 验项目这条端到端路径不半截：
// 后台编排项目具名树后，读者项目列表卡片的篇数与封面回退也来自同一张表，
// 不能因为卡片仍读旧共用表而显示「0 篇」。
func TestProjectNamedTreeFeedsListCardsOverRealMySQL(t *testing.T) {
	t.Parallel()

	db := migrateNamedOutline(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedNamedOutlineArticles(t, db, at)

	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	projectID := createDisplayedProjectForMigrate(t, admin, true)

	// 项目具名树：21 无封面（草稿口径之外它是公开文章），22 有封面，24 有封面。
	requestProjectNamedTree(t, admin, projectID, `{"nodes":[
		{"kind":1,"title":"第一章","children":[
			{"kind":2,"article_id":"21"},
			{"kind":2,"article_id":"22"}
		]},
		{"kind":2,"article_id":"24"}
	]}`)

	raw := rawRequest(t, reader, "GET", "/api/projects", nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("项目列表应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var list struct {
		List []struct {
			ID           ctype.ID `json:"id"`
			ArticleCount int      `json:"article_count"`
			Cover        string   `json:"cover"`
		} `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析项目列表失败: %v（响应 %s）", err, raw)
	}
	if len(list.List) != 1 || list.List[0].ID != projectID {
		t.Fatalf("项目列表应只有刚建的展示中项目，实际 %+v（响应 %s）", list.List, raw)
	}
	if list.List[0].ArticleCount != 3 {
		t.Fatalf("卡片篇数应来自项目具名树（3 篇），实际 %d（响应 %s）",
			list.List[0].ArticleCount, raw)
	}
	// 封面回退取 DFS 第一篇公开文章：21 没有封面，不能借用后一篇。
	if list.List[0].Cover != "" {
		t.Fatalf("卡片封面回退应逐字沿用读者读面（首篇 21 无封面 → 空），实际 %q",
			list.List[0].Cover)
	}
}

// namedOutlineArticleIDs 按 DFS 顺序取具名响应里的文章 ID（跨接口的外部形状）。
func namedOutlineArticleIDs(items []collectionOutlineNode) []uint64 {
	var out []uint64
	for _, row := range flattenCollectionOutline(items) {
		if row.Kind == 2 {
			out = append(out, row.ArticleID.Uint64())
		}
	}
	return out
}
