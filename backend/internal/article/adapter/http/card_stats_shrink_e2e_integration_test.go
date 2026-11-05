//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 本文件是 #277「卡片统计统一收口」的真 MySQL 行为用例：只经 HTTP 接口观察
// 外部行为——PUT 具名树之后，合集与项目两处列表卡片的篇数 / 封面同时同步。
//
// 收缩前合集卡片读旧共用表、项目卡片读项目具名表，是「一半失真」的中间态；
// 这篇用例钉住的是统一后的「归属树读模型」：调用方只给出哪个 owner，读面
// 内部按归属走各自新表。不读源码、不越过接口查库断言实现细节。

type shrinkCard struct {
	ID           ctype.ID `json:"id"`
	ArticleCount int      `json:"article_count"`
	Cover        string   `json:"cover"`
}

// TestNamedOutlineTreesDriveBothListCards PUT 具名树 → 两处列表卡片同步。
func TestNamedOutlineTreesDriveBothListCards(t *testing.T) {
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

	// 两个归属各自整树提交：合集挂 22 / 24，项目挂 24 / 23。
	// 22 的封面是 first-visible、24 是 third-visible、23 是 second-visible，
	// 因此两处卡片的回退封面必须分别取各自 DFS 第一篇公开文章，不能串表。
	requestNamedTree(t, admin, "PUT", collectionTree,
		[]byte(`{"nodes":[{"kind":2,"article_id":"22"},{"kind":2,"article_id":"24"}]}`))
	requestNamedTree(t, admin, "PUT", projectTree,
		[]byte(`{"nodes":[{"kind":2,"article_id":"24"},{"kind":2,"article_id":"23"}]}`))

	readCards := func(path string) []shrinkCard {
		t.Helper()
		raw := rawRequest(t, reader, "GET", path, nil)
		env := decode(t, raw)
		if env.Code != 0 {
			t.Fatalf("读 %s 应成功，实际 code=%d msg=%s（响应 %s）", path, env.Code, env.Msg, raw)
		}
		var data struct {
			List []shrinkCard `json:"list"`
		}
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatalf("解析 %s 失败: %v（响应 %s）", path, err, raw)
		}
		return data.List
	}

	readAdminCards := func(path string) []shrinkCard {
		t.Helper()
		raw := rawRequest(t, admin, "GET", path, nil)
		env := decode(t, raw)
		if env.Code != 0 {
			t.Fatalf("读 %s 应成功，实际 code=%d msg=%s（响应 %s）", path, env.Code, env.Msg, raw)
		}
		var data struct {
			List []shrinkCard `json:"list"`
		}
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatalf("解析 %s 失败: %v（响应 %s）", path, err, raw)
		}
		return data.List
	}

	collections := readCards("/api/collections")
	projects := readCards("/api/projects")
	if len(collections) != 1 || collections[0].ArticleCount != 2 ||
		collections[0].Cover != "https://cdn.example.com/first-visible.png" {
		t.Fatalf("合集列表卡片应读合集具名树（2 篇 / first-visible），实际 %+v", collections)
	}
	if len(projects) != 1 || projects[0].ArticleCount != 2 ||
		projects[0].Cover != "https://cdn.example.com/third-visible.png" {
		t.Fatalf("项目列表卡片应读项目具名树（2 篇 / third-visible），实际 %+v", projects)
	}

	// 后台卡片与读者卡片同一份归属树读模型。
	adminCollections := readAdminCards("/api/collections/admin")
	adminProjects := readAdminCards("/api/projects/admin")
	if len(adminCollections) != 1 || adminCollections[0].ArticleCount != 2 ||
		adminCollections[0].Cover != "" {
		t.Fatalf("后台合集卡片篇数应为 2、cover 保持库里的空串，实际 %+v", adminCollections)
	}
	if len(adminProjects) != 1 || adminProjects[0].ArticleCount != 2 ||
		adminProjects[0].Cover != "" {
		t.Fatalf("后台项目卡片篇数应为 2、cover 保持库里的空串，实际 %+v", adminProjects)
	}
}
