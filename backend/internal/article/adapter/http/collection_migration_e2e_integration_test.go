//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是 #275「合集迁移」的**真 MySQL 契约用例**：只经具名 HTTP 端点观察
// 外部行为，不读源码、不越过接口查库断言实现细节。
//
// 覆盖票内四条端到端路径：
//   - 后台编辑器对合集具名树整树编排（GET 读回 / PUT 提交 / 树内不重复）；
//   - 读者详情页与阅读页左栏按 `?from=collection:<id>` 找回同一棵树；
//   - 一篇文章可以进合集具名树、且同一棵树内不重复；
//   - 管理员能打开未展示的合集（后台读面保留失效行），读者仍按不存在处理。
//
// 旧共享端点仍可用是 #274 expand 期的事实，归 #277 收缩，本票不删。

// collectionMigrationEnv 是一次用例的真实装配：真库 + 固定身份的两套路由。
type collectionMigrationEnv struct {
	db     *gorm.DB
	admin  *gin.Engine
	reader *gin.Engine
}

// newCollectionMigrationEnv 建出合集具名树闭环需要的表。
// schema.Outline 组含旧共用表与两张按实体命名的新表（#274 后）。
func newCollectionMigrationEnv(t *testing.T) collectionMigrationEnv {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection, schema.Outline,
	)
	return collectionMigrationEnv{
		db:     db,
		admin:  newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100),
		reader: newRouterWithIDs(t, db, domain.Anonymous(), 100),
	}
}

// seedMigrationArticles 造 3 篇已发布且可见的文章供合集树编排。
func seedMigrationArticles(t *testing.T, env collectionMigrationEnv) {
	t.Helper()
	seedAuthor(t, env.db, 100)
	for id := uint64(21); id <= 23; id++ {
		seedCollectionArticle(t, env.db, id, 100)
	}
}

// createCollectionOwner 经后台接口建合集，返回 ID；displayed 由调用方给定。
func createCollectionOwner(t *testing.T, r *gin.Engine, displayed bool) ctype.ID {
	t.Helper()
	body := fmt.Sprintf(`{"title":"迁移合集","description":"","cover":"","displayed":%t}`, displayed)
	raw := rawRequest(t, r, "POST", "/api/collections", []byte(body))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("建合集应成功，实际 code=%d msg=%s（响应 %s）", env.Code, env.Msg, raw)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("解析合集 ID 失败: %v（响应 %s）", err, raw)
	}
	return created.ID
}

// collectionTreeURL 是 #274 选定的合集具名端点（契约路径不带 /api）。
func collectionTreeURL(id ctype.ID) string {
	return fmt.Sprintf("/api/collections/%s/outline-tree", id)
}

// requestCollectionTree 经具名端点读写，返回外部响应形状。
// 命名形状复用 #274 集成用例的 namedOutlineTreeResponse，不另抄一份字段名。
func requestCollectionTree(
	t *testing.T, r *gin.Engine, method, path string, body []byte,
) namedOutlineTreeResponse {
	t.Helper()
	raw := rawRequest(t, r, method, path, body)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("%s %s 应成功，实际 code=%d msg=%s（响应 %s）", method, path, env.Code, env.Msg, raw)
	}
	var out namedOutlineTreeResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("解析合集具名树响应失败: %v（响应 %s）", err, raw)
	}
	return out
}

// TestCollectionMigrationAdminArrangesNamedTree 后台编辑器经合集具名端点完成
// 整树编排：文件夹 + 文章节点、排序、树内不重复、幂等重复提交。
func TestCollectionMigrationAdminArrangesNamedTree(t *testing.T) {
	t.Parallel()
	env := newCollectionMigrationEnv(t)
	seedMigrationArticles(t, env)
	id := createCollectionOwner(t, env.admin, true)
	tree := collectionTreeURL(id)

	const body = `{"nodes":[
		{"kind":1,"title":"第一章","children":[
			{"kind":2,"article_id":"21"},
			{"kind":2,"article_id":"22"}
		]},
		{"kind":2,"article_id":"23"}
	]}`

	// ① 整树提交：后台读面保留全部 4 个节点，顺序按 DFS。
	saved := requestCollectionTree(t, env.admin, "PUT", tree, []byte(body))
	flat := flattenCollectionOutline(saved.Nodes)
	if len(flat) != 4 {
		t.Fatalf("整树提交应落 4 个节点，实际 %d: %+v", len(flat), flat)
	}
	if flat[0].Title != "第一章" || flat[1].ArticleID.Uint64() != 21 ||
		flat[2].ArticleID.Uint64() != 22 || flat[3].ArticleID.Uint64() != 23 {
		t.Fatalf("节点顺序 / 归属不对: %+v", flat)
	}

	// ② GET 具名树：管理员读回同一棵后台树（不是空树）。
	loaded := requestCollectionTree(t, env.admin, "GET", tree, nil)
	if len(flattenCollectionOutline(loaded.Nodes)) != 4 {
		t.Fatalf("后台编辑器读回的树应与提交一致，实际 %+v", loaded.Nodes)
	}

	// ③ 同一棵树内同一篇文章不重复：第二次出现应被接口拒绝，且原树不被破坏。
	dupRaw := rawRequest(t, env.admin, "PUT", tree, []byte(`{"nodes":[
		{"kind":2,"article_id":"21"},
		{"kind":2,"article_id":"21"}
	]}`))
	if dup := decode(t, dupRaw); dup.Code == 0 {
		t.Fatalf("同一棵合集树内重复文章应被拒绝，实际成功（响应 %s）", dupRaw)
	}
	afterReject := requestCollectionTree(t, env.admin, "GET", tree, nil)
	if len(flattenCollectionOutline(afterReject.Nodes)) != 4 {
		t.Fatalf("被拒的整树提交不应改动原树，实际 %+v", afterReject.Nodes)
	}

	// ④ 幂等：重复提交同一棵树不产生新行。
	again := requestCollectionTree(t, env.admin, "PUT", tree, []byte(body))
	if len(flattenCollectionOutline(again.Nodes)) != 4 {
		t.Fatalf("重复提交应幂等，实际 %+v", again.Nodes)
	}
}

// TestCollectionMigrationReaderFindsSameTreeByFrom 读者详情页与阅读页左栏
// 按 `?from=collection:<id>` 找回后台编排的同一棵树：篇数、封面回退、
// 上一章 / 下一章都不丢上下文。
func TestCollectionMigrationReaderFindsSameTreeByFrom(t *testing.T) {
	t.Parallel()
	env := newCollectionMigrationEnv(t)
	seedMigrationArticles(t, env)
	// 给 21 一张封面，验「封面取 DFS 第一个公开文章节点」而非借后一篇。
	if err := env.db.Table("article_models").Where("id = ?", 21).
		Update("cover", "https://cdn.example.com/first.png").Error; err != nil {
		t.Fatalf("设置封面失败: %v", err)
	}
	id := createCollectionOwner(t, env.admin, true)
	tree := collectionTreeURL(id)

	requestCollectionTree(t, env.admin, "PUT", tree, []byte(`{"nodes":[
		{"kind":1,"title":"第一章","children":[
			{"kind":2,"article_id":"21"},
			{"kind":2,"article_id":"22"}
		]},
		{"kind":2,"article_id":"23"}
	]}`))

	// ① 读者详情页：具名 GET 不带 article_id 时是纯读者读面。
	detail := requestCollectionTree(t, env.reader, "GET", tree, nil)
	if !detail.HasTree || detail.OwnerKind != 1 || detail.OwnerID != id {
		t.Fatalf("合集具名形状应带归属，实际 %+v", detail)
	}
	if detail.ArticleCount != 3 || detail.Cover != "https://cdn.example.com/first.png" {
		t.Fatalf("篇数 / 封面回退不对: %+v", detail)
	}
	if detail.Current != nil || detail.Previous != nil || detail.Next != nil {
		t.Fatalf("不带 article_id 不应回答相邻文章，实际 %+v", detail)
	}

	// ② 阅读页左栏：`?from=collection:<id>` + 当前文章 id，找回同一棵树。
	from := fmt.Sprintf("collection:%s", id)
	aside := requestCollectionTree(t, env.reader, "GET",
		tree+"?from="+from+"&article_id=22", nil)
	if !aside.HasTree || aside.OwnerID != id || aside.ArticleCount != 3 {
		t.Fatalf("按来路找回的树不对: %+v", aside)
	}
	if aside.Current == nil || aside.Current.ArticleID.Uint64() != 22 ||
		aside.Previous == nil || aside.Previous.ArticleID.Uint64() != 21 ||
		aside.Next == nil || aside.Next.ArticleID.Uint64() != 23 {
		t.Fatalf("上一章 / 下一篇应跨文件夹成立，实际 current=%+v previous=%+v next=%+v",
			aside.Current, aside.Previous, aside.Next)
	}

	// ③ 来路指向别的树时不串专题：这棵合集树按没有来路上下文处理。
	wrong := requestCollectionTree(t, env.reader, "GET",
		tree+"?from=project:999&article_id=22", nil)
	if wrong.HasTree {
		t.Fatalf("来路不属于这棵合集树时不应给出树，实际 %+v", wrong)
	}
}

// TestCollectionMigrationAdminOpensUndisplayedCollection 管理员能打开未展示的
// 合集：后台编辑器 GET 具名树读回完整后台读面（含失效行），读者仍按「不存在」
// 处理。这是显式管理员例外，不新增端点与契约形状。
func TestCollectionMigrationAdminOpensUndisplayedCollection(t *testing.T) {
	t.Parallel()
	env := newCollectionMigrationEnv(t)
	seedMigrationArticles(t, env)
	id := createCollectionOwner(t, env.admin, false) // 未展示
	tree := collectionTreeURL(id)

	requestCollectionTree(t, env.admin, "PUT", tree, []byte(`{"nodes":[
		{"kind":2,"article_id":"21"},
		{"kind":2,"article_id":"22"}
	]}`))
	// 21 退回草稿：后台读面保留并标失效，读者读面过滤。
	if err := env.db.Table("article_models").Where("id = ?", 21).
		Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}

	// ① 管理员 GET：未展示也能读回整棵树，失效行保留。
	adminView := requestCollectionTree(t, env.admin, "GET", tree, nil)
	adminFlat := flattenCollectionOutline(adminView.Nodes)
	if len(adminFlat) != 2 {
		t.Fatalf("管理员应读回未展示合集的完整树（2 个节点），实际 %+v", adminView.Nodes)
	}
	if stale := adminFlat[0]; !stale.Stale || stale.StaleReason != "not_published" {
		t.Fatalf("后台读面应标记草稿文章失效，实际 %+v", stale)
	}

	// ② 读者 GET 同一端点：未展示按空树处理（详情页 / 左栏不出现）。
	readerView := requestCollectionTree(t, env.reader, "GET", tree, nil)
	if readerView.HasTree || len(readerView.Nodes) != 0 || readerView.ArticleCount != 0 {
		t.Fatalf("读者读未展示合集应没有树，实际 %+v", readerView)
	}

	// ③ 读者详情旧端点仍按不存在处理（#277 才下线）。
	detailRaw := rawRequest(t, env.reader, "GET", fmt.Sprintf("/api/collections/%s", id), nil)
	if detail := decode(t, detailRaw); detail.Code == 0 {
		t.Fatalf("读者读未展示合集详情应按不存在处理，实际成功（响应 %s）", detailRaw)
	}
}

// TestCollectionMigrationArticleCanEnterCollectionTree 一篇文章可以进合集具名树，
// 且同一棵树内不重复出现（跨树重复允许，由 #276 验项目侧）。
func TestCollectionMigrationArticleCanEnterCollectionTree(t *testing.T) {
	t.Parallel()
	env := newCollectionMigrationEnv(t)
	seedMigrationArticles(t, env)
	collectionID := createCollectionOwner(t, env.admin, true)
	tree := collectionTreeURL(collectionID)

	saved := requestCollectionTree(t, env.admin, "PUT", tree, []byte(`{"nodes":[
		{"kind":2,"article_id":"21"}
	]}`))
	flat := flattenCollectionOutline(saved.Nodes)
	if len(flat) != 1 || flat[0].ArticleID.Uint64() != 21 {
		t.Fatalf("文章应能进合集树，实际 %+v", flat)
	}
	// 同一篇文章重复进入同一棵树被拒（与整树提交共用一条外部行为）。
	dup := rawRequest(t, env.admin, "PUT", tree, []byte(`{"nodes":[
		{"kind":2,"article_id":"21"},
		{"kind":1,"title":"空章节","children":[{"kind":2,"article_id":"21"}]}
	]}`))
	if env := decode(t, dup); env.Code == 0 {
		t.Fatalf("同一棵树内重复文章应被拒绝，实际成功（响应 %s）", dup)
	}
}
