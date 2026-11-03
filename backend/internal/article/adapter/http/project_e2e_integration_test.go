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

// 本文件是「项目」读者侧公开读的端到端契约（#193 / #219）：列表 / 详情 /
// 未展示 / 封面回退 / 篇数只算仍公开的文章。
//
// #219 契约破坏性变更后，项目详情返回**嵌套目录树**（`outline`），扁平
// `articles[]` 已退役。种子直接写 `content_outline_node_models`
// （schema.InsertOutlineNode）；旧扁平收录中间表已随 #225 从代码里删除。
//
// 走真实 HTTP + 真 MySQL，断言的是**外部行为**（响应里的业务码与字段），
// 不是仓储方法被调了几次。管理端写接口属于 #197，这里直接往表里播种。

// migrateProjectsHTTP 建出项目闭环需要的表。
func migrateProjectsHTTP(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Project, schema.Outline)
	return db
}

func seedProjectArticle(
	t *testing.T, db *gorm.DB, id, authorID uint64, cover string,
	status schema.ArticleStatus, visibility schema.ArticleVisibility, at time.Time,
) {
	t.Helper()
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          fmt.Sprintf("项目文章 %d", id),
		Abstract:       "摘要",
		Content:        "正文",
		AuthorID:       authorID,
		ViewCount:      int(id),
		CommentsToggle: true,
		PublishStatus:  status,
		Visibility:     visibility,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
	if cover != "" {
		if err := db.Table("article_models").Where("id = ?", id).Update("cover", cover).Error; err != nil {
			t.Fatalf("设置文章 %d 封面失败: %v", id, err)
		}
	}
}

// seedProjectOutlineNode 往项目树里播种一行。
func seedProjectOutlineNode(t *testing.T, db *gorm.DB, spec schema.OutlineNodeSpec) {
	t.Helper()
	spec.OwnerKind = int(outline.OwnerProject)
	schema.InsertOutlineNode(t, db, spec)
}

// projectOutlineNode 是项目详情 / 后台树响应里的一行（嵌套）。
//
// 与合集共用 OutlineNodeResponse 形状：文件夹节点带 title，文章节点的标题与
// 封面是 article_title / article_cover 快照；stale 只在后台读面有意义。
type projectOutlineNode struct {
	ID           ctype.ID             `json:"id"`
	ParentID     ctype.ID             `json:"parent_id"`
	Kind         int                  `json:"kind"`
	Title        string               `json:"title"`
	ArticleID    ctype.ID             `json:"article_id"`
	SortOrder    int                  `json:"sort_order"`
	ArticleTitle string               `json:"article_title"`
	ArticleCover string               `json:"article_cover"`
	Stale        bool                 `json:"stale"`
	StaleReason  string               `json:"stale_reason"`
	Children     []projectOutlineNode `json:"children"`
}

// flattenProjectOutline 按 DFS 把嵌套树拍平。
func flattenProjectOutline(items []projectOutlineNode) []projectOutlineNode {
	var out []projectOutlineNode
	var walk func([]projectOutlineNode)
	walk = func(rows []projectOutlineNode) {
		for _, row := range rows {
			out = append(out, row)
			walk(row.Children)
		}
	}
	walk(items)
	return out
}

// projectOutlineArticleIDs 按 DFS 顺序取树里的文章 ID。
func projectOutlineArticleIDs(items []projectOutlineNode) []uint64 {
	var out []uint64
	for _, row := range flattenProjectOutline(items) {
		if row.Kind == int(outline.NodeKindArticle) {
			out = append(out, row.ArticleID.Uint64())
		}
	}
	return out
}

// projectDetailData 是读者项目详情的读模型（本批只有嵌套树，没有扁平 articles）。
type projectDetailData struct {
	ID           ctype.ID `json:"id"`
	Title        string   `json:"title"`
	Cover        string   `json:"cover"`
	ArticleCount int      `json:"article_count"`
	Outline      struct {
		Nodes []projectOutlineNode `json:"nodes"`
	} `json:"outline"`
}

// readProjectDetail 读读者项目详情。
func readProjectDetail(
	t *testing.T, r *gin.Engine, id ctype.ID,
) projectDetailData {
	t.Helper()
	raw := rawRequest(t, r, "GET", fmt.Sprintf("/api/projects/%s", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("项目详情应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var detail projectDetailData
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("解析项目详情失败: %v（响应 %s）", err, raw)
	}
	return detail
}

// readProjectAdminOutline 读后台项目树编辑器的整棵树。
func readProjectAdminOutline(
	t *testing.T, r *gin.Engine, id ctype.ID,
) []projectOutlineNode {
	t.Helper()
	raw := rawRequest(t, r, "GET",
		fmt.Sprintf("/api/projects/%s/outline/admin", id), nil)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("后台项目目录树应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var data struct {
		Nodes []projectOutlineNode `json:"nodes"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析后台项目目录树失败: %v（响应 %s）", err, raw)
	}
	return data.Nodes
}

// mustSaveProjectOutline 整棵树一次性提交，断言成功后返回保存后的根层节点。
func mustSaveProjectOutline(
	t *testing.T, r *gin.Engine, id ctype.ID, body string,
) []projectOutlineNode {
	t.Helper()
	raw := rawRequest(t, r, "PUT",
		fmt.Sprintf("/api/projects/%s/outline", id), []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("保存项目目录树应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var data struct {
		Nodes []projectOutlineNode `json:"nodes"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("解析保存结果失败: %v（响应 %s）", err, raw)
	}
	return data.Nodes
}

// TestProjectPublicRead 读者侧只看到展示中的项目：详情是嵌套目录树、失效文章
// 节点被过滤、空文件夹保留、篇数只算公开文章、封面空时回退首篇公开文章封面。
func TestProjectPublicRead(t *testing.T) {
	t.Parallel()

	db := migrateProjectsHTTP(t)
	seedAuthor(t, db, 100)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// 文章：21 草稿、22/23 公开、24 公开后被软删、25 属于未展示项目、
	// 26 属于有显式封面的项目、27 属于已软删项目、28 是 0 篇项目里唯一的草稿。
	seedProjectArticle(t, db, 21, 100, "https://cdn.example/draft.png",
		schema.ArticleStatusDraft, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 22, 100, "https://cdn.example/first.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 23, 100, "https://cdn.example/second.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 24, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 25, 100, "https://cdn.example/hidden.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 26, 100, "https://cdn.example/explicit.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 27, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 28, 100, "", schema.ArticleStatusDraft, schema.VisibilityVisible, at)
	// 29 第一个公开文章节点、没有封面；30 后面有封面——卡片不能借用 30 的封面。
	seedProjectArticle(t, db, 29, 100, "", schema.ArticleStatusPublished, schema.VisibilityVisible, at)
	seedProjectArticle(t, db, 30, 100, "https://cdn.example/later.png",
		schema.ArticleStatusPublished, schema.VisibilityVisible, at)

	// 24 被软删：树节点保留，读者侧靠文章快照过滤。
	if err := db.Table("article_models").Where("id = ?", 24).Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 项目 3001：展示中、无封面（应回退到 DFS 首篇公开文章 22 的封面）。
	// 树：「第一章」下 21（草稿）/22，根层 23、24（已删）与空文件夹「待补充」。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3001, Title: "重构实战", Description: "从零到上线",
		Displayed: true, SortOrder: 1, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8100, OwnerID: 3001, Kind: int(outline.NodeKindFolder),
		Title: "第一章", SortOrder: 1, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8101, OwnerID: 3001, ParentID: 8100, Kind: int(outline.NodeKindArticle),
		ArticleID: 21, SortOrder: 1, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8102, OwnerID: 3001, ParentID: 8100, Kind: int(outline.NodeKindArticle),
		ArticleID: 22, SortOrder: 2, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8103, OwnerID: 3001, Kind: int(outline.NodeKindArticle),
		ArticleID: 23, SortOrder: 2, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8104, OwnerID: 3001, Kind: int(outline.NodeKindArticle),
		ArticleID: 24, SortOrder: 3, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8105, OwnerID: 3001, Kind: int(outline.NodeKindFolder),
		Title: "待补充", SortOrder: 4, At: at,
	})

	// 项目 3002：未展示——读者侧必须按不存在处理。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3002, Title: "还没公布的项目", Cover: "https://cdn.example/hidden.png",
		Displayed: false, SortOrder: 2, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8200, OwnerID: 3002, Kind: int(outline.NodeKindArticle),
		ArticleID: 25, SortOrder: 1, At: at,
	})

	// 项目 3003：展示中、有显式封面——不许被回退值覆盖。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3003, Title: "显式封面", Cover: "https://cdn.example/explicit.png",
		Displayed: true, SortOrder: 3, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8300, OwnerID: 3003, Kind: int(outline.NodeKindArticle),
		ArticleID: 26, SortOrder: 1, At: at,
	})

	// 项目 3004：已软删——读者侧同样不存在。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3004, Title: "被删的项目", Displayed: true, SortOrder: 4, CreatorID: 100,
		CreatedAt: at, UpdatedAt: at, SoftDeleted: true,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8400, OwnerID: 3004, Kind: int(outline.NodeKindArticle),
		ArticleID: 27, SortOrder: 1, At: at,
	})

	// 项目 3005：展示中、无封面、一篇公开文章都没有——树为空、篇数 0。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3005, Title: "空项目", Displayed: true, SortOrder: 5, CreatorID: 100,
		CreatedAt: at, UpdatedAt: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8500, OwnerID: 3005, Kind: int(outline.NodeKindArticle),
		ArticleID: 28, SortOrder: 1, At: at,
	})

	// 项目 3006：展示中、无封面，树里第一个公开文章节点（29）没有封面，
	// 后面 30 有封面也不能借用。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3006, Title: "首篇无封面", Displayed: true, SortOrder: 6, CreatorID: 100,
		CreatedAt: at, UpdatedAt: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8600, OwnerID: 3006, Kind: int(outline.NodeKindArticle),
		ArticleID: 29, SortOrder: 1, At: at,
	})
	seedProjectOutlineNode(t, db, schema.OutlineNodeSpec{
		ID: 8601, OwnerID: 3006, Kind: int(outline.NodeKindArticle),
		ArticleID: 30, SortOrder: 2, At: at,
	})

	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)

	listRaw := rawRequest(t, reader, "GET", "/api/projects", nil)
	listEnv := decode(t, listRaw)
	if listEnv.Code != 0 {
		t.Fatalf("项目列表应成功，实际 code=%d msg=%s", listEnv.Code, listEnv.Msg)
	}
	var list struct {
		List []struct {
			ID           ctype.ID `json:"id"`
			Title        string   `json:"title"`
			Cover        string   `json:"cover"`
			ArticleCount int      `json:"article_count"`
		} `json:"list"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listEnv.Data, &list); err != nil {
		t.Fatalf("解析项目列表失败: %v", err)
	}

	// 只出展示中且未软删的项目：3001 / 3003 / 3005 / 3006，按 sort_order 升序。
	// 卡片篇数与封面回退都读目录树（旧扁平中间表为空）。
	if list.Count != 4 || len(list.List) != 4 {
		t.Fatalf("读者列表应有 4 个项目，实际 count=%d len=%d（响应 %s）",
			list.Count, len(list.List), listRaw)
	}
	wantIDs := []uint64{3001, 3003, 3005, 3006}
	for i, want := range wantIDs {
		if list.List[i].ID.Uint64() != want {
			t.Fatalf("第 %d 张卡片应是项目 %d，实际 %s（响应 %s）",
				i+1, want, list.List[i].ID, listRaw)
		}
	}
	if list.List[0].Title != "重构实战" {
		t.Fatalf("项目卡片字段不对: %+v", list.List[0])
	}
	// 项目 3001：树里 21 草稿、24 已删都不算；公开的是 22、23，篇数 2。
	// 封面回退取 DFS 第一个公开文章节点 22 的封面。
	if list.List[0].ArticleCount != 2 || list.List[0].Cover != "https://cdn.example/first.png" {
		t.Fatalf("项目 3001 卡片不对: %+v", list.List[0])
	}
	// 项目 3003：显式封面优先，篇数 1。
	if list.List[1].ArticleCount != 1 || list.List[1].Cover != "https://cdn.example/explicit.png" {
		t.Fatalf("项目 3003 卡片不对: %+v", list.List[1])
	}
	// 项目 3005：没有公开文章 → 篇数 0、封面空。
	if list.List[2].ArticleCount != 0 || list.List[2].Cover != "" {
		t.Fatalf("项目 3005 卡片不对: %+v", list.List[2])
	}
	// 项目 3006：首篇公开文章没有封面 → 卡片封面为空，不能借用后一篇。
	if list.List[3].ArticleCount != 2 || list.List[3].Cover != "" {
		t.Fatalf("项目 3006 卡片不应借用后一篇封面: %+v", list.List[3])
	}

	// 详情 3001：嵌套树只含公开文章节点（21 草稿与 24 已删都被过滤），
	// 空文件夹「待补充」保留；篇数只算公开文章、封面回退到首篇公开文章的封面。
	detail := readProjectDetail(t, reader, ctype.NewID(3001))
	if detail.ID.Uint64() != 3001 || detail.Title != "重构实战" {
		t.Fatalf("详情项目字段不对: %+v", detail)
	}
	if detail.ArticleCount != 2 {
		t.Fatalf("项目 3001 的篇数应只算 2 篇公开文章，实际 %d", detail.ArticleCount)
	}
	if detail.Cover != "https://cdn.example/first.png" {
		t.Fatalf("项目 3001 封面应回退到 DFS 首篇公开文章的封面，实际 %q", detail.Cover)
	}
	if got := projectOutlineArticleIDs(detail.Outline.Nodes); len(got) != 2 || got[0] != 22 || got[1] != 23 {
		t.Fatalf("详情树里的文章应只剩 [22, 23]，实际 %v（nodes=%+v）", got, detail.Outline.Nodes)
	}
	if len(detail.Outline.Nodes) != 3 {
		t.Fatalf("根层应是【第一章, 23, 待补充】，实际 %+v", detail.Outline.Nodes)
	}
	if detail.Outline.Nodes[0].Kind != int(outline.NodeKindFolder) ||
		detail.Outline.Nodes[0].Title != "第一章" ||
		len(detail.Outline.Nodes[0].Children) != 1 ||
		detail.Outline.Nodes[0].Children[0].ArticleID.Uint64() != 22 {
		t.Fatalf("「第一章」下应只剩公开的 22，实际 %+v", detail.Outline.Nodes[0])
	}
	if last := detail.Outline.Nodes[2]; last.Kind != int(outline.NodeKindFolder) ||
		last.Title != "待补充" || len(last.Children) != 0 {
		t.Fatalf("空文件夹「待补充」应保留，实际 %+v", last)
	}

	// 显式封面不被回退覆盖。
	explicit := readProjectDetail(t, reader, ctype.NewID(3003))
	if explicit.Cover != "https://cdn.example/explicit.png" || explicit.ArticleCount != 1 {
		t.Fatalf("项目 3003 详情不对: %+v", explicit)
	}

	// 没有公开文章：树为空、篇数 0、封面留空（前端把空封面交给兜底底纹）。
	empty := readProjectDetail(t, reader, ctype.NewID(3005))
	if empty.Cover != "" || empty.ArticleCount != 0 || len(empty.Outline.Nodes) != 0 {
		t.Fatalf("项目 3005 详情不对: %+v", empty)
	}

	// 首个公开文章节点没有封面：详情封面也为空，不借用后一篇。
	firstNoCover := readProjectDetail(t, reader, ctype.NewID(3006))
	if firstNoCover.Cover != "" || firstNoCover.ArticleCount != 2 {
		t.Fatalf("项目 3006 详情不对: %+v", firstNoCover)
	}

	// 未展示 / 已软删的项目对读者按**不存在**处理：不返回「未发布 / 未展示」
	// 这类内部信息。
	for _, id := range []ctype.ID{ctype.NewID(3002), ctype.NewID(3004)} {
		hiddenRaw := rawRequest(t, reader, "GET", "/api/projects/"+string(id), nil)
		hiddenEnv := decode(t, hiddenRaw)
		if hiddenEnv.Code == 0 {
			t.Fatalf("项目 %s 对读者应按不存在处理，实际成功（响应 %s）", id, hiddenRaw)
		}
		if hiddenEnv.Msg != "项目不存在" {
			t.Fatalf("项目 %s 的错误文案应只说「项目不存在」，实际 %q", id, hiddenEnv.Msg)
		}
		if containsAny(hiddenEnv.Msg, []string{"未发布", "未展示", "隐藏", "草稿"}) {
			t.Fatalf("错误文案泄漏了内部状态: %q", hiddenEnv.Msg)
		}
	}
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && len(haystack) >= len(needle) {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
		}
	}
	return false
}
