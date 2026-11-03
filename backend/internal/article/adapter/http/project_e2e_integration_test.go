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

// 本文件是「项目」读者侧公开读的端到端契约（#193）：列表 / 详情 / 未展示 /
// 封面回退 / 篇数只算仍公开的文章。
//
// 走真实 HTTP + 真 MySQL，断言的是**外部行为**（响应里的业务码与字段），
// 不是仓储方法被调了几次。管理端写接口属于 #197，这里直接往表里播种。

// migrateProjectsHTTP 建出项目闭环需要的表。
func migrateProjectsHTTP(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Project)
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

// TestProjectPublicRead 读者侧只看到展示中的项目：篇数只算公开文章、
// 封面空时回退首篇公开文章封面、未展示与已软删都按不存在处理。
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

	// 24 被软删：中间行保留，读者侧靠联表过滤。
	if err := db.Table("article_models").Where("id = ?", 24).Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 项目 3001：展示中、无封面（应回退到首篇公开文章 22 的封面）。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3001, Title: "重构实战", Description: "从零到上线",
		Displayed: true, SortOrder: 1, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	for i, articleID := range []uint64{21, 22, 23, 24} {
		schema.InsertProjectArticle(t, db, uint64(4000+i), 3001, articleID, i+1, at)
	}

	// 项目 3002：未展示——读者侧必须按不存在处理。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3002, Title: "还没公布的项目", Cover: "https://cdn.example/hidden.png",
		Displayed: false, SortOrder: 2, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertProjectArticle(t, db, 4004, 3002, 25, 1, at)

	// 项目 3003：展示中、有显式封面——不许被回退值覆盖。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3003, Title: "显式封面", Cover: "https://cdn.example/explicit.png",
		Displayed: true, SortOrder: 3, CreatorID: 100, CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertProjectArticle(t, db, 4005, 3003, 26, 1, at)

	// 项目 3004：已软删——读者侧同样不存在。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3004, Title: "被删的项目", Displayed: true, SortOrder: 4, CreatorID: 100,
		CreatedAt: at, UpdatedAt: at, SoftDeleted: true,
	})
	schema.InsertProjectArticle(t, db, 4006, 3004, 27, 1, at)

	// 项目 3005：展示中、无封面、一篇公开文章都没有——封面留空，篇数 0。
	schema.InsertProject(t, db, schema.ProjectSpec{
		ID: 3005, Title: "空项目", Displayed: true, SortOrder: 5, CreatorID: 100,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertProjectArticle(t, db, 4007, 3005, 28, 1, at)

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
			Description  string   `json:"description"`
			Cover        string   `json:"cover"`
			ArticleCount int      `json:"article_count"`
		} `json:"list"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listEnv.Data, &list); err != nil {
		t.Fatalf("解析项目列表失败: %v", err)
	}

	// 只出展示中且未软删的项目：3001 / 3003 / 3005，按 sort_order 升序。
	if list.Count != 3 || len(list.List) != 3 {
		t.Fatalf("读者列表应有 3 个项目，实际 count=%d len=%d（响应 %s）",
			list.Count, len(list.List), listRaw)
	}
	wantIDs := []uint64{3001, 3003, 3005}
	for i, want := range wantIDs {
		if list.List[i].ID.Uint64() != want {
			t.Fatalf("第 %d 张卡片应是项目 %d，实际 %s（响应 %s）",
				i+1, want, list.List[i].ID, listRaw)
		}
	}
	if list.List[0].Title != "重构实战" || list.List[0].Description != "从零到上线" {
		t.Fatalf("项目卡片字段不对: %+v", list.List[0])
	}

	// 篇数只算仍公开的文章：21 草稿与 24 已软删都不计入。
	if list.List[0].ArticleCount != 2 {
		t.Fatalf("项目 3001 的篇数应只算 2 篇公开文章，实际 %d", list.List[0].ArticleCount)
	}
	// 封面为空 → 回退首篇**公开**文章的封面（按收录顺序跳过草稿 21）。
	if list.List[0].Cover != "https://cdn.example/first.png" {
		t.Fatalf("项目 3001 封面应回退到首篇公开文章的封面，实际 %q", list.List[0].Cover)
	}
	// 显式封面不被回退覆盖。
	if list.List[1].Cover != "https://cdn.example/explicit.png" ||
		list.List[1].ArticleCount != 1 {
		t.Fatalf("项目 3003 卡片不对: %+v", list.List[1])
	}
	// 没有公开文章：封面留空、篇数 0（前端把空封面交给兜底底纹）。
	if list.List[2].Cover != "" || list.List[2].ArticleCount != 0 {
		t.Fatalf("项目 3005 卡片不对: %+v", list.List[2])
	}

	// 详情：按管理员设定的顺序，只出仍公开的文章。
	detailRaw := rawRequest(t, reader, "GET", "/api/projects/3001", nil)
	detailEnv := decode(t, detailRaw)
	if detailEnv.Code != 0 {
		t.Fatalf("项目详情应成功，实际 code=%d msg=%s", detailEnv.Code, detailEnv.Msg)
	}
	var detail struct {
		ID    ctype.ID `json:"id"`
		Title string   `json:"title"`
		Cover string   `json:"cover"`
		Articles []struct {
			ArticleID ctype.ID `json:"article_id"`
			SortOrder int      `json:"sort_order"`
			Title     string   `json:"title"`
			Cover     string   `json:"cover"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(detailEnv.Data, &detail); err != nil {
		t.Fatalf("解析项目详情失败: %v", err)
	}
	if detail.ID.Uint64() != 3001 || detail.Title != "重构实战" {
		t.Fatalf("详情项目字段不对: %+v（响应 %s）", detail, detailRaw)
	}
	if len(detail.Articles) != 2 {
		t.Fatalf("详情应只剩 2 篇公开文章，实际 %d（响应 %s）", len(detail.Articles), detailRaw)
	}
	// 顺序保留原序号（21 被过滤，所以是 2、3）。
	for i, want := range []struct {
		id    uint64
		order int
	}{{22, 2}, {23, 3}} {
		if detail.Articles[i].ArticleID.Uint64() != want.id || detail.Articles[i].SortOrder != want.order {
			t.Fatalf("第 %d 篇不对: %+v（期望 id=%d order=%d）", i+1, detail.Articles[i], want.id, want.order)
		}
	}

	// 未展示的项目对读者按**不存在**处理：不返回「未发布 / 未展示」这类内部信息。
	for _, id := range []string{"3002", "3004"} {
		hiddenRaw := rawRequest(t, reader, "GET", "/api/projects/"+id, nil)
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
