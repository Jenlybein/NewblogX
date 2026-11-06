//go:build integration

package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// 本文件是 #274 expand 期两个具名端点的**真 MySQL 契约用例**。
//
// 只经 HTTP 接口观察行为：整树提交、失效文章过滤、封面回退、上一章 / 下一章、
// `?from=` 来路、两张新表互不串数据、旧共享端点已下线。不读源码文本、不越过
// 接口查库断言实现细节（库表结构一致性另有 schema 包契约用例钉住）。

// namedOutlineCase 是同一份行为用例在两个实体上的参数。
type namedOutlineCase struct {
	name       string
	createPath string
	treePath   func(id ctype.ID) string
	wantKind   int
}

// 两个具名端点路径：每个实体一条 `/<entity>s/{id}/outline-tree`。
// GET 是读者读面（可按 `?from=` 求上一章 / 下一章），PUT 是后台整树提交。
func namedOutlineCases() []namedOutlineCase {
	return []namedOutlineCase{
		{
			name:       "collection",
			createPath: "/api/collections",
			treePath: func(id ctype.ID) string {
				return fmt.Sprintf("/api/collections/%s/outline-tree", id)
			},
			wantKind: int(outline.OwnerCollection),
		},
		{
			name:       "project",
			createPath: "/api/projects",
			treePath: func(id ctype.ID) string {
				return fmt.Sprintf("/api/projects/%s/outline-tree", id)
			},
			wantKind: int(outline.OwnerProject),
		},
	}
}

// namedOutlineRef 是响应里「当前 / 上一篇 / 下一篇」的一行。
type namedOutlineRef struct {
	ID           ctype.ID `json:"id"`
	ArticleID    ctype.ID `json:"article_id"`
	ArticleTitle string   `json:"article_title"`
	ArticleCover string   `json:"article_cover"`
}

// namedOutlineTreeResponse 是具名端点响应的外部形状（字段名来自契约产物）。
type namedOutlineTreeResponse struct {
	HasTree      bool                    `json:"has_tree"`
	OwnerKind    int                     `json:"owner_kind"`
	OwnerID      ctype.ID                `json:"owner_id"`
	Nodes        []collectionOutlineNode `json:"nodes"`
	ArticleCount int                     `json:"article_count"`
	Cover        string                  `json:"cover"`
	Current      *namedOutlineRef        `json:"current"`
	Previous     *namedOutlineRef        `json:"previous"`
	Next         *namedOutlineRef        `json:"next"`
}

func requestNamedTree(t *testing.T, r *gin.Engine, method, path string, body []byte) namedOutlineTreeResponse {
	t.Helper()
	raw := rawRequest(t, r, method, path, body)
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("%s %s 应成功，实际 code=%d msg=%s（响应 %s）", method, path, env.Code, env.Msg, raw)
	}
	var out namedOutlineTreeResponse
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("解析具名端点响应失败: %v（响应 %s）", err, raw)
	}
	return out
}

// legacyStatus 返回旧共享端点的 HTTP 状态码（不解析响应体）。
//
// gin 对未注册路由直接回纯文本 404，没有业务 Envelope；验「端点已下线」
// 只需要状态码这一层，越过它去解 JSON 会把 404 误报成解析失败。
func legacyStatus(t *testing.T, r *gin.Engine, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func migrateNamedOutline(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	// schema.Outline 组只含两张按实体命名的树表；e2e 夹具点名这一组。
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Collection, schema.Project, schema.Outline,
	)
	return db
}

// seedNamedOutlineArticles 造 4 篇已发布且可见的文章；首篇无封面、第二篇有封面，
// 用来验「封面回退取 DFS 第一个仍公开的文章节点」与「失效节点被过滤」。
func seedNamedOutlineArticles(t *testing.T, db *gorm.DB, at time.Time) {
	t.Helper()
	seedAuthor(t, db, 100)
	covers := map[uint64]string{
		21: "",
		22: "https://cdn.example.com/first-visible.png",
		23: "https://cdn.example.com/second-visible.png",
		24: "https://cdn.example.com/third-visible.png",
	}
	for id, cover := range covers {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             id,
			Title:          fmt.Sprintf("章节文章 %d", id),
			Abstract:       "摘要",
			Content:        "正文",
			AuthorID:       100,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      at,
			UpdatedAt:      at,
		})
		if err := db.Table("article_models").Where("id = ?", id).
			Update("cover", cover).Error; err != nil {
			t.Fatalf("设置文章 %d 封面失败: %v", id, err)
		}
	}
}

// createNamedOwner 通过后台接口建一个展示中的归属实体，返回它的 ID。
func createNamedOwner(t *testing.T, r *gin.Engine, path string) ctype.ID {
	t.Helper()
	raw := rawRequest(t, r, "POST", path, []byte(`{
		"title": "具名树归属",
		"description": "",
		"cover": "",
		"displayed": true
	}`))
	env := decode(t, raw)
	if env.Code != 0 {
		t.Fatalf("建归属实体 %s 应成功，实际 code=%d msg=%s（响应 %s）", path, env.Code, env.Msg, raw)
	}
	var created struct {
		ID ctype.ID `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("解析归属实体 ID 失败: %v（响应 %s）", err, raw)
	}
	return created.ID
}

const namedOutlineSaveBody = `{"nodes":[
	{"kind":1,"title":"第一章","children":[
		{"kind":2,"article_id":"21"},
		{"kind":2,"article_id":"22"},
		{"kind":2,"article_id":"23"}
	]},
	{"kind":2,"article_id":"24"}
]}`

// TestNamedOutlineEndpointsOverRealMySQL 逐实体验整树提交、失效过滤、封面回退、
// 上一章 / 下一章与 `?from=` 来路；两张新表各自独立。
func TestNamedOutlineEndpointsOverRealMySQL(t *testing.T) {
	for _, tc := range namedOutlineCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := migrateNamedOutline(t)
			at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			seedNamedOutlineArticles(t, db, at)

			admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
			reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
			ownerID := createNamedOwner(t, admin, tc.createPath)
			treePath := tc.treePath(ownerID)

			// ① 整树提交：文件夹 + 3 篇文章 + 根层 1 篇；响应是后台读面（含全部节点）。
			saved := requestNamedTree(t, admin, "PUT", treePath, []byte(namedOutlineSaveBody))
			if saved.OwnerKind != tc.wantKind || saved.OwnerID != ownerID {
				t.Fatalf("具名形状应带归属（kind=%d id=%s），实际 %+v", tc.wantKind, ownerID, saved)
			}
			flat := flattenCollectionOutline(saved.Nodes)
			if len(flat) != 5 {
				t.Fatalf("整树提交应落 5 个节点（第一章 + 21/22/23 + 24），实际 %d: %+v", len(flat), flat)
			}
			if flat[0].Kind != int(outline.NodeKindFolder) || flat[0].Title != "第一章" {
				t.Fatalf("第一个节点应是文件夹「第一章」，实际 %+v", flat[0])
			}

			// ② 幂等：重复提交同一棵树不产生新行。
			again := requestNamedTree(t, admin, "PUT", treePath, []byte(namedOutlineSaveBody))
			if len(flattenCollectionOutline(again.Nodes)) != 5 {
				t.Fatalf("重复提交应幂等，实际 %+v", again.Nodes)
			}

			// ③ 文章 21 退回草稿：读者读面过滤它，后台读面保留并标失效。
			if err := db.Table("article_models").Where("id = ?", 21).
				Update("publish_status", int8(schema.ArticleStatusDraft)).Error; err != nil {
				t.Fatalf("退回草稿失败: %v", err)
			}

			// ④ 阅读页把 `?from=<kind>:<ownerId>` 原样透传，外加当前文章 ID；
			// 验来路、失效过滤、封面回退、上一章 / 下一章。
			from := fmt.Sprintf("%s:%s", tc.name, ownerID)
			got := requestNamedTree(t, reader, "GET",
				treePath+"?from="+from+"&article_id=23", nil)
			if !got.HasTree || got.OwnerKind != tc.wantKind || got.OwnerID != ownerID {
				t.Fatalf("具名响应形状不对: %+v", got)
			}
			visible := flattenCollectionOutline(got.Nodes)
			if len(visible) != 4 {
				t.Fatalf("读者读面应过滤掉失效的 21，剩 4 个节点，实际 %d: %+v", len(visible), visible)
			}
			for _, row := range visible {
				if row.ArticleID.Uint64() == 21 {
					t.Fatalf("失效文章 21 不应出现在读者读面: %+v", visible)
				}
			}
			if got.ArticleCount != 3 {
				t.Fatalf("篇数应只数仍公开的文章（3），实际 %d", got.ArticleCount)
			}
			// DFS 第一个仍公开的文章是 22；它没有封面时不能跳过借 23 的封面。
			// 这里 22 有封面，回退值必须是它（不借用后一篇）。
			if got.Cover != "https://cdn.example.com/first-visible.png" {
				t.Fatalf("封面应回退到 DFS 第一个公开文章节点，实际 %q", got.Cover)
			}
			if got.Current == nil || got.Current.ArticleID.Uint64() != 23 {
				t.Fatalf("current 应是来路文章 23，实际 %+v", got.Current)
			}
			if got.Previous == nil || got.Previous.ArticleID.Uint64() != 22 {
				t.Fatalf("previous 应是 22，实际 %+v", got.Previous)
			}
			if got.Next == nil || got.Next.ArticleID.Uint64() != 24 {
				t.Fatalf("next 应跨文件夹取到根层的 24，实际 %+v", got.Next)
			}

			// ⑤ 不带 `from`：不回答相邻文章，但树、篇数、封面照旧。
			plain := requestNamedTree(t, reader, "GET", treePath, nil)
			if plain.Current != nil || plain.Previous != nil || plain.Next != nil {
				t.Fatalf("没有来路文章时相邻引用都应为 null，实际 %+v", plain)
			}
			if plain.ArticleCount != 3 || plain.Cover != got.Cover {
				t.Fatalf("不带 from 时读者读面不应变化: %+v", plain)
			}

			// ⑥ 后台读面保留失效行并标原因：再整树提交一遍（带 ID 的幂等保存），
			// 响应是后台读面，失效的 21 仍在树上且带 stale_reason。
			backstage := requestNamedTree(t, admin, "PUT", treePath, []byte(namedOutlineSaveBody))
			backstageFlat := flattenCollectionOutline(backstage.Nodes)
			if len(backstageFlat) != 5 {
				t.Fatalf("后台读面应保留 5 个节点，实际 %d: %+v", len(backstageFlat), backstageFlat)
			}
			if stale := backstageFlat[1]; stale.ArticleID.Uint64() != 21 ||
				!stale.Stale || stale.StaleReason != "not_published" {
				t.Fatalf("草稿文章的节点应标 not_published，实际 %+v", stale)
			}

			// ⑦ 旧共享端点已随 #277 收缩下线：同一个后台身份打过去应 404，
			// 具名端点仍是唯一入口（不是"旧表还能读"的兼容层）。
			// gin 的 404 是纯文本 "404 page not found"，不是 Envelope JSON：
			// 这里只看 HTTP 状态码，别拿 decode() 去解一层不存在的外壳。
			legacyPath := fmt.Sprintf("/api/%ss/%s/outline/admin", tc.name, ownerID)
			if got := legacyStatus(t, admin, "GET", legacyPath); got != http.StatusNotFound {
				t.Fatalf("旧共享端点 %s 应返回 404，实际 %d", legacyPath, got)
			}
		})
	}
}

// TestNamedOutlineTablesAreIndependent 验两张具名表各自独立：同一篇文章可以
// 同时进两棵树，但一棵树的节点不会串到另一棵。
func TestNamedOutlineTablesAreIndependent(t *testing.T) {
	t.Parallel()

	db := migrateNamedOutline(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedNamedOutlineArticles(t, db, at)

	admin := newRouterWithIDs(t, db, domain.AdminUser(ctype.NewID(900)), 100)
	reader := newRouterWithIDs(t, db, domain.Anonymous(), 100)
	collectionID := createNamedOwner(t, admin, "/api/collections")
	projectID := createNamedOwner(t, admin, "/api/projects")

	collectionTree := fmt.Sprintf("/api/collections/%s/outline-tree", collectionID)
	projectTree := fmt.Sprintf("/api/projects/%s/outline-tree", projectID)

	requestNamedTree(t, admin, "PUT", collectionTree,
		[]byte(`{"nodes":[{"kind":2,"article_id":"21"},{"kind":2,"article_id":"22"}]}`))
	requestNamedTree(t, admin, "PUT", projectTree,
		[]byte(`{"nodes":[{"kind":2,"article_id":"22"},{"kind":2,"article_id":"24"}]}`))

	c := requestNamedTree(t, reader, "GET", collectionTree, nil)
	p := requestNamedTree(t, reader, "GET", projectTree, nil)
	if len(c.Nodes) != 2 || len(p.Nodes) != 2 {
		t.Fatalf("两棵树各自应有 2 个节点，实际合集 %+v 项目 %+v", c.Nodes, p.Nodes)
	}
	if c.Nodes[0].ArticleID.Uint64() != 21 || c.Nodes[1].ArticleID.Uint64() != 22 {
		t.Fatalf("合集树只应有 21/22，实际 %+v", c.Nodes)
	}
	if p.Nodes[0].ArticleID.Uint64() != 22 || p.Nodes[1].ArticleID.Uint64() != 24 {
		t.Fatalf("项目树只应有 22/24，实际 %+v", p.Nodes)
	}
	// 22 同时出现在两棵树里（跨树重复允许），没有因唯一键互相顶掉。
	if c.ArticleCount != 2 || p.ArticleCount != 2 {
		t.Fatalf("跨树重复不应影响篇数，实际合集 %d 项目 %d", c.ArticleCount, p.ArticleCount)
	}
}
