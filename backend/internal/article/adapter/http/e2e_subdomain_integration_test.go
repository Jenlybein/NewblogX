//go:build integration

package http_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// TestSubdomainEndpointsEndToEnd 逐个子域端点走真实 HTTP + 真 MySQL。
//
// golden 只覆盖读接口；这里补上写路径与副作用：
// 分类/标签/收藏夹增删、收藏文章、置顶、浏览历史、点赞、更新、
// 管理员改可见性、审核任务通过、删除文章。
func TestSubdomainEndpointsEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "sd_author", "作者甲", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "sd_reader", "读者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 900, "sd_admin", "管理员", "", "", schema.RoleAdmin)

	author := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 1000)
	reader := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(200)), 2000)
	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 3000)
	anon := newRouterWithIDs(t, db, domain.Anonymous(), 4000)

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

	// ── 标签的「包含停用」只对管理员生效（票 323）──
	// 标签公开读保留（发布页要选标签），但停用项属于管理视图：匿名与普通用户
	// 即使显式传 include_disabled 也只拿到启用项，且不报错。
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(`{"title":"端到端启用标签","sort":8}`)), "建启用标签")
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(`{"title":"端到端停用标签","sort":9,"is_enabled":false}`)), "建停用标签")
	const disabledTitle = "端到端停用标签"
	tagsWithDisabledURL := "/api/articles/tags?include_disabled=1"

	// 管理员：停用项可见。
	if findIDByTitle(listOf(t, do(t, admin, http.MethodGet, tagsWithDisabledURL, nil)), disabledTitle) == "" {
		t.Fatal("管理员带 include_disabled 应看到停用标签")
	}
	// 匿名：降级成只返回启用项（不报错）。
	if findIDByTitle(listOf(t, do(t, anon, http.MethodGet, tagsWithDisabledURL, nil)), disabledTitle) != "" {
		t.Fatal("匿名带 include_disabled 不应看到停用标签")
	}
	// 普通用户：同样降级。
	if findIDByTitle(listOf(t, do(t, reader, http.MethodGet, tagsWithDisabledURL, nil)), disabledTitle) != "" {
		t.Fatal("普通用户带 include_disabled 不应看到停用标签")
	}

	// 公开读本身不能被这次收紧弄坏：匿名不带参数照样读到启用项。
	anonTags := listOf(t, do(t, anon, http.MethodGet, "/api/articles/tags", nil))
	if findIDByTitle(anonTags, "端到端启用标签") == "" {
		t.Fatal("匿名默认标签列表缺启用项")
	}
	if findIDByTitle(anonTags, disabledTitle) != "" {
		t.Fatal("匿名默认标签列表不应含停用项")
	}
	// 分类公开读同理（个人主页要展示分类）。
	respCode(t, do(t, anon, http.MethodGet, "/api/articles/category?user_id=100", nil), "匿名读分类")

	// 清理标签，避免影响后续子域断言。
	allTags := listOf(t, do(t, admin, http.MethodGet, tagsWithDisabledURL, nil))
	cleanupIDs := make([]string, 0, 2)
	for _, title := range []string{"端到端启用标签", disabledTitle} {
		if id := findIDByTitle(allTags, title); id != "" {
			cleanupIDs = append(cleanupIDs, id)
		}
	}
	respCode(t, do(t, admin, http.MethodDelete, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, strings.Join(cleanupIDs, ",")))), "删标签（鉴权用例）")

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
	// create 的 status=2 在新发布口径下会落「已发布 + pending_review」（票 302/ADR-0035）：
	// 干净文章立刻发布、但等模型判定通过才对读者可见。这里专注验证子域端点，
	// 所以把可见性一并清成 visible，模拟"模型已放行"之后的真发布态。
	if err := db.Exec("UPDATE article_models SET publish_status = 3, visibility_status = 'visible' WHERE id = ?", articleID).Error; err != nil {
		t.Fatalf("置为已发布失败: %v", err)
	}
	if got := queryInt(t, db, "SELECT publish_status FROM article_models WHERE id = ?", articleID); got != 3 {
		t.Fatalf("置为已发布未生效: %d", got)
	}
	if got := queryString(t, db, "SELECT visibility_status FROM article_models WHERE id = ?", articleID); got != "visible" {
		t.Fatalf("visibility 异常: %q", got)
	}

	// ── 点赞（读者，目标态接口）──
	respCode(t, do(t, reader, http.MethodPut, "/api/articles/"+articleID+"/digg",
		[]byte(`{"liked":true}`)), "点赞")

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

	// ── 审核任务：列表 → 通过 ──
	// 票 302/ADR-0035 新规则下，status=2 的干净提交不再进人工队列（落
	// Published + pending_review），人工任务只由异步模型副作用产生。
	// 这个 e2e 专注验证审核端点，所以显式模拟一次"模型存疑转人工"：
	// 把文章改回审核中，并插一条 pending 任务。
	reviewCreated := do(t, author, http.MethodPost, "/api/articles",
		[]byte(`{"title":"待审文章","content":"正文","status":2}`))
	respCode(t, reviewCreated, "建待审文章")
	reviewArticleID := dataOf(t, reviewCreated)["id"].(string)
	if err := db.Exec(`UPDATE article_models
		SET publish_status = 2, visibility_status = 'visible' WHERE id = ?`, reviewArticleID).Error; err != nil {
		t.Fatalf("模拟转人工失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO article_review_task_models
		(id, created_at, updated_at, article_id, author_id, article_title, author_name,
		 article_publish_status, stage, source, status)
		SELECT ?, NOW(3), NOW(3), id, author_id, '待审文章', '作者', 2, 'manual', 'model_suspect', 'pending'
		FROM article_models WHERE id = ?`,
		queryInt(t, db, "SELECT IFNULL(MAX(id),0)+1000 FROM article_review_task_models"),
		reviewArticleID).Error; err != nil {
		t.Fatalf("插入复盘任务失败: %v", err)
	}

	taskID := ""
	for _, task := range listOf(t, do(t, admin, http.MethodGet, "/api/article-review", nil)) {
		if task["article_id"] == reviewArticleID {
			taskID, _ = task["id"].(string)
			// 机审前两层的字段必须出现在每一条任务上（票 311）：端到端装配没有
			// 注入评分/词表，所以这里是 none + 空数组，但形状不能缺。
			if task["risk_level"] != "none" {
				t.Fatalf("无词表、无评分时风险等级应为 none，实际 %v", task["risk_level"])
			}
			if _, ok := task["risk_labels"].([]any); !ok {
				t.Fatalf("risk_labels 应是数组，实际 %T", task["risk_labels"])
			}
		}
	}
	if taskID == "" {
		t.Fatal("审核任务列表缺任务")
	}
	// 两种排序口径都必须被受理（risk 在内存里排，created_at 走数据库默认序）。
	respCode(t, do(t, admin, http.MethodGet, "/api/article-review?sort=risk", nil), "按风险排序")
	respCode(t, do(t, admin, http.MethodGet, "/api/article-review?sort=created_at", nil), "按提交时间排序")
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

// TestTagAdminLifecycleEndToEnd 是后台「标签管理」页依赖的三条语义（票 322），
// 走真路由 + 真 MySQL 验证：
//
//  1. 删除门禁：被文章引用的标签删不掉，提示里带篇数并建议改用停用；
//  2. 编辑是读—改—写：只传标题时排序 / 描述 / 启用状态保持原值
//     （停用的标签不会被悄悄放回全站可选）；
//  3. 软删后同名重建能成功（title 唯一索引不含 deleted_at，裸 INSERT 会 1062）。
func TestTagAdminLifecycleEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "tag_author", "标签作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 900, "tag_admin", "标签管理员", "", "", schema.RoleAdmin)

	author := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 1000)
	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 3000)

	// ── 新建：排序与描述一起写入 ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(`{"title":"端到端标签","sort":7,"description":"标签描述"}`)), "建标签")
	tagID := findIDByTitle(listOf(t, do(t, admin, http.MethodGet, "/api/articles/tags", nil)), "端到端标签")
	if tagID == "" {
		t.Fatal("标签列表缺新标签")
	}

	// ── 建一篇草稿并打上这个标签：它就成了"被引用的标签" ──
	created := do(t, author, http.MethodPost, "/api/articles",
		[]byte(fmt.Sprintf(`{"title":"带标签的文章","content":"正文","status":1,"tag_ids":[%s]}`, tagID)))
	respCode(t, created, "建带标签的文章")
	articleID := dataOf(t, created)["id"].(string)

	// ── 删除被拒：可读文案（篇数 + 建议停用），标签原封不动 ──
	rejected := do(t, admin, http.MethodDelete, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, tagID)))
	if code, _ := rejected["code"].(float64); code == 0 {
		t.Fatalf("被文章引用的标签不应允许删除: %v", rejected)
	}
	msg, _ := rejected["msg"].(string)
	for _, want := range []string{"1 篇文章在用", "停用"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("删除被拒的提示应包含 %q，实际 %q", want, msg)
		}
	}
	if findIDByTitle(listOf(t, do(t, admin, http.MethodGet, "/api/articles/tags", nil)), "端到端标签") == "" {
		t.Fatal("被拒的删除不该把标签删掉")
	}

	// ── 编辑只传标题：排序与描述保持原值，仍然启用 ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id":%s,"title":"端到端标签改"}`, tagID))), "只改标题")
	row := findRowByTitle(t, listOf(t, do(t, admin, http.MethodGet,
		"/api/articles/tags?include_disabled=true", nil)), "端到端标签改")
	if row == nil {
		t.Fatal("改名后列表里找不到标签")
	}
	if got, _ := row["sort"].(float64); got != 7 {
		t.Fatalf("没传排序时应保留 7，实际 %v", row["sort"])
	}
	if row["description"] != "标签描述" {
		t.Fatalf("没传描述时应保留「标签描述」，实际 %v", row["description"])
	}
	if enabled, _ := row["is_enabled"].(bool); !enabled {
		t.Fatal("没传启用状态时不该把启用标签改成停用")
	}

	// ── 停用后从发布页下拉（只含启用项）里消失 ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id":%s,"title":"端到端标签改","is_enabled":false}`, tagID))), "停用标签")
	if id := findIDByTitle(listOf(t, do(t, admin, http.MethodGet,
		"/api/articles/tags/options", nil)), "端到端标签改"); id != "" {
		t.Fatalf("停用标签不该出现在发布页下拉里（%s）", id)
	}

	// ── 再编辑一次（只传标题）：仍是停用 ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id":%s,"title":"端到端标签改2"}`, tagID))), "编辑停用标签")
	row = findRowByTitle(t, listOf(t, do(t, admin, http.MethodGet,
		"/api/articles/tags?include_disabled=true", nil)), "端到端标签改2")
	if row == nil {
		t.Fatal("编辑停用标签后列表里找不到它")
	}
	if enabled, _ := row["is_enabled"].(bool); enabled {
		t.Fatal("编辑停用标签不该把它悄悄放回全站可选")
	}

	// ── 删掉文章之后它就零引用了：此时删除应当放行 ──
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/"+articleID, nil), "删文章")
	respCode(t, do(t, admin, http.MethodDelete, "/api/articles/tags",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, tagID))), "删零引用标签")

	// ── 软删后同名重建：成功，且列表里只有一条（恢复软删行，不新增） ──
	respCode(t, do(t, admin, http.MethodPut, "/api/articles/tags",
		[]byte(`{"title":"端到端标签改2","sort":1}`)), "同名重建标签")
	rows := listOf(t, do(t, admin, http.MethodGet, "/api/articles/tags?include_disabled=true", nil))
	same := 0
	for _, it := range rows {
		if it["title"] == "端到端标签改2" {
			same++
		}
	}
	if same != 1 {
		t.Fatalf("同名重建后列表里应只有一条，实际 %d 条", same)
	}

	// ── 标题重复：可读文案，不是 MySQL 1062 原文 ──
	dup := do(t, admin, http.MethodPut, "/api/articles/tags", []byte(`{"title":"端到端标签改2"}`))
	if code, _ := dup["code"].(float64); code == 0 {
		t.Fatalf("重复标题应被拒绝，实际成功: %v", dup)
	}
	if dup["msg"] != "名称重复" {
		t.Fatalf("重复标题的提示应为「名称重复」，实际 %v", dup["msg"])
	}
}

// TestCategoryManageLifecycleEndToEnd 是票 325 的端到端验收，走真路由 + 真 MySQL：
//
//	"建分类 → 挂文章 → 改名 → 删分类 → 文章变未分类 → 同名重建成功"
//
// 外加三条边界：两人各自的同名分类互不影响、改 / 删他人的分类被拒绝、
// 分类名首尾空白被忽略。
func TestCategoryManageLifecycleEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "cat_author", "分类作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "cat_other", "另一个作者", "", "", schema.RoleUser)

	author := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(100)), 1000)
	other := newRouterWithIDs(t, db, domain.NormalUser(ctype.NewID(200)), 2000)
	anon := newRouterWithIDs(t, db, domain.Anonymous(), 4000)

	categoryPath := "/api/articles/category?user_id=100"

	// ── 新建：首尾空白被忽略，列表里是去空白后的名字 ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/category",
		[]byte(`{"title":"  端到端分类  "}`)), "建分类")
	catID := findIDByTitle(listOf(t, do(t, author, http.MethodGet, categoryPath, nil)), "端到端分类")
	if catID == "" {
		t.Fatal("分类列表缺新分类（或标题没去空白）")
	}

	// ── 挂一篇文章上去 ──
	created := do(t, author, http.MethodPost, "/api/articles",
		[]byte(fmt.Sprintf(`{"title":"带分类的文章","content":"正文","status":1,"category_id":%s}`, catID)))
	respCode(t, created, "建带分类的文章")
	articleID := dataOf(t, created)["id"].(string)
	detailPath := "/api/articles/" + articleID

	detail := dataOf(t, do(t, author, http.MethodGet, detailPath, nil))
	if detail["category_name"] != "端到端分类" {
		t.Fatalf("详情应显示分类「端到端分类」，实际 %v", detail["category_name"])
	}

	// ── 改名：列表与文章详情都跟着变 ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/category",
		[]byte(fmt.Sprintf(`{"id":%s,"title":"端到端分类-改名"}`, catID))), "分类改名")
	if id := findIDByTitle(listOf(t, do(t, author, http.MethodGet, categoryPath, nil)), "端到端分类-改名"); id != catID {
		t.Fatalf("改名后列表应显示新名字且是同一条（%s），实际 %q", catID, id)
	}
	detail = dataOf(t, do(t, author, http.MethodGet, detailPath, nil))
	if detail["category_name"] != "端到端分类-改名" {
		t.Fatalf("改名后文章详情应显示新名字，实际 %v", detail["category_name"])
	}

	// ── 删分类：文章不消失，变成未分类 ──
	respCode(t, do(t, author, http.MethodDelete, "/api/articles/category",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, catID))), "删分类")
	if id := findIDByTitle(listOf(t, do(t, author, http.MethodGet, categoryPath, nil)), "端到端分类-改名"); id != "" {
		t.Fatalf("删除后分类列表里不该再有它（%s）", id)
	}
	detailResp := do(t, author, http.MethodGet, detailPath, nil)
	respCode(t, detailResp, "删除分类后读文章详情")
	detail = dataOf(t, detailResp)
	if detail["category_id"] != nil {
		t.Fatalf("删除分类后文章应变成未分类，实际 category_id=%v", detail["category_id"])
	}
	if detail["category_name"] != "" {
		t.Fatalf("删除分类后详情不该再显示分类名，实际 %v", detail["category_name"])
	}

	// ── 同名重建：成功，且恢复的是原来那条（保留原 ID） ──
	respCode(t, do(t, author, http.MethodPost, "/api/articles/category",
		[]byte(`{"title":"端到端分类-改名"}`)), "同名重建分类")
	rows := listOf(t, do(t, author, http.MethodGet, categoryPath, nil))
	same := make([]string, 0, 1)
	for _, it := range rows {
		if it["title"] == "端到端分类-改名" {
			same = append(same, it["id"].(string))
		}
	}
	if len(same) != 1 {
		t.Fatalf("同名重建后列表里应只有一条，实际 %d 条", len(same))
	}
	if same[0] != catID {
		t.Fatalf("恢复应保留原 ID %s，实际 %s", catID, same[0])
	}

	// ── 两个用户各有同名分类：互不冲突，也互相看不到 ──
	respCode(t, do(t, other, http.MethodPost, "/api/articles/category",
		[]byte(`{"title":"端到端分类-改名"}`)), "另一个作者用同名分类")
	otherID := findIDByTitle(listOf(t, do(t, other, http.MethodGet,
		"/api/articles/category?user_id=200", nil)), "端到端分类-改名")
	if otherID == "" || otherID == catID {
		t.Fatalf("另一位作者应拥有自己的一条同名分类，实际 %q", otherID)
	}
	if id := findIDByTitle(listOf(t, do(t, anon, http.MethodGet, categoryPath, nil)), "端到端分类-改名"); id != catID {
		t.Fatalf("匿名读作者主页分类应看到作者自己那条（%s），实际 %q", catID, id)
	}

	// ── 改 / 删他人的分类被拒绝，且什么都没发生 ──
	renamed := do(t, other, http.MethodPost, "/api/articles/category",
		[]byte(fmt.Sprintf(`{"id":%s,"title":"偷改别人的分类"}`, catID)))
	if code, _ := renamed["code"].(float64); code == 0 {
		t.Fatalf("改别人的分类应被拒绝，实际成功: %v", renamed)
	}
	removed := do(t, other, http.MethodDelete, "/api/articles/category",
		[]byte(fmt.Sprintf(`{"id_list":[%s]}`, catID)))
	if code, _ := removed["code"].(float64); code == 0 {
		t.Fatalf("删别人的分类应被拒绝，实际成功: %v", removed)
	}
	if id := findIDByTitle(listOf(t, do(t, author, http.MethodGet, categoryPath, nil)), "端到端分类-改名"); id != catID {
		t.Fatalf("被拒的改 / 删不该动作者自己的分类，实际 %q", id)
	}

	// ── 匿名看不到写操作（写接口都在登录组）：直接调会被拒 ──
	anonSave := do(t, anon, http.MethodPost, "/api/articles/category",
		[]byte(`{"title":"匿名分类"}`))
	if code, _ := anonSave["code"].(float64); code == 0 {
		t.Fatalf("匿名建分类应被拒绝，实际成功: %v", anonSave)
	}
}

// findRowByTitle 在列表里按 title 找整行（找不到返回 nil）。
func findRowByTitle(t *testing.T, items []map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items {
		if it["title"] == title {
			return it
		}
	}
	return nil
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
// containsNumericID 判断列表里是否含指定 ID。
//
// 三种形态都要认：裸数字（旧接口）、裸字符串、以及对象里的 "id" 字段。
// `/articles/top` 已从"ID 数组"改成"卡片数组"（首页要渲染标题/封面），
// 所以这里必须能读对象；保留前两种是为了其它仍返回 ID 列表的接口还能复用。
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
		case map[string]any:
			if got, ok := n["id"].(string); ok && got == id {
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
