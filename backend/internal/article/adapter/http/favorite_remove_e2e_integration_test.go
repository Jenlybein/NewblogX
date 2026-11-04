//go:build integration

package http_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件守「收藏移除」的两条数据语义（评审 #248 的 P0-1 / P0-2）：
//
//  1. **只删目标收藏夹**：文章同时收在 A/B 时，移出 A 必须删 A 的那条、保留 B 的，
//     而不是「取任意一条、按它的 folder 删」；
//  2. **批量移除按实际语义返回**：全部成功 → removed_count = N；部分成功 →
//     removed_count / failed_count 同时给；全部失败 → 非 0 业务码（不是成功 + 空 data）。
//
// 为什么走真 HTTP + 真 MySQL：这两条都是 handler 与仓储的接缝（AnyFolder 取值、
// 循环里 continue 吞错），fake 仓储里能"看起来对"，只有在关系行上才看得见。

func TestUnfavoriteRemovesOnlyTargetFolderEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "fav_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "fav_reader", "读者", "", "", schema.RoleUser)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             2001,
		Title:          "同时收在两个夹子里的文章",
		AuthorID:       100,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
	const articleID = "2001"

	folderA := createFavoriteFolder(t, reader, "收藏夹 A")
	folderB := createFavoriteFolder(t, reader, "收藏夹 B")
	addArticleToFolder(t, reader, articleID, folderA)
	addArticleToFolder(t, reader, articleID, folderB)
	if got := countFavoriteRows(t, db, 2001, folderA); got != 1 {
		t.Fatalf("A 里应有一条关系行，实际 %d", got)
	}
	if got := countFavoriteRows(t, db, 2001, folderB); got != 1 {
		t.Fatalf("B 里应有一条关系行，实际 %d", got)
	}

	// 移出 A：只删 A 的那条，B 的必须原样保留。
	resp := do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
		[]byte(fmt.Sprintf(`{"articles":[%s],"favorite_id":%s}`, articleID, folderA)))
	respCode(t, resp, "移出 A")

	data := dataOf(t, resp)
	if got := data["is_favor"]; got != true {
		t.Fatalf("还在 B 里，is_favor 应为 true，实际 %v（响应 %v）", got, resp)
	}
	// P0-2 / P0-3 的契约载荷：批量移除必须给出 removed_count / failed_count。
	if got, ok := data["removed_count"].(float64); !ok || int(got) != 1 {
		t.Fatalf("removed_count 应为 1，实际 %v（响应 %v）", data["removed_count"], resp)
	}
	if got, ok := data["failed_count"].(float64); !ok || int(got) != 0 {
		t.Fatalf("failed_count 应为 0，实际 %v（响应 %v）", data["failed_count"], resp)
	}

	if got := countFavoriteRows(t, db, 2001, folderA); got != 0 {
		t.Fatalf("A 的关系行应已删除，实际还剩 %d 条", got)
	}
	// 关键断言：旧实现会删掉 B 的那条，A 反而还在。
	if got := countFavoriteRows(t, db, 2001, folderB); got != 1 {
		t.Fatalf("B 的关系行不应被误删，实际剩 %d 条", got)
	}

	// 收藏夹内容列表与 DB 同口径：A 空、B 有。
	if got := len(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite/contents?favorite_id="+folderA, nil))); got != 0 {
		t.Fatalf("A 的收藏内容应为空，实际 %d 条", got)
	}
	if !containsArticleID(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite/contents?favorite_id="+folderB, nil)), articleID) {
		t.Fatal("B 的收藏内容应仍含该文章")
	}
}

func TestRemoveFavoriteItemsSuccessSemanticsEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "fav_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "fav_reader", "读者", "", "", schema.RoleUser)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []uint64{3001, 3002, 3003} {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             id,
			Title:          fmt.Sprintf("批量移除文章 %d", id),
			AuthorID:       100,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      at,
			UpdatedAt:      at,
		})
	}

	folder := createFavoriteFolder(t, reader, "批量收藏夹")

	// 3001 / 3002 收进目标夹；3003 故意不收，用来制造部分失败与全失败。
	addArticleToFolder(t, reader, "3001", folder)
	addArticleToFolder(t, reader, "3002", folder)

	t.Run("全部成功：removed_count=N、failed_count=0", func(t *testing.T) {
		resp := do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
			[]byte(fmt.Sprintf(`{"articles":[3001,3002],"favorite_id":%s}`, folder)))
		respCode(t, resp, "全部成功")
		data := dataOf(t, resp)
		if got, ok := data["removed_count"].(float64); !ok || int(got) != 2 {
			t.Fatalf("removed_count 应为 2，实际 %v", data["removed_count"])
		}
		if got, ok := data["failed_count"].(float64); !ok || int(got) != 0 {
			t.Fatalf("failed_count 应为 0，实际 %v", data["failed_count"])
		}
		if n := countFavoriteRows(t, db, 3001, folder); n != 0 {
			t.Fatalf("3001 的关系行应已删除，实际 %d", n)
		}
		if n := countFavoriteRows(t, db, 3002, folder); n != 0 {
			t.Fatalf("3002 的关系行应已删除，实际 %d", n)
		}
	})

	t.Run("部分成功：调用方必须能从 removed_count/failed_count 分辨", func(t *testing.T) {
		addArticleToFolder(t, reader, "3001", folder) // 只有 3001 在夹里，3003 不在。
		resp := do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
			[]byte(fmt.Sprintf(`{"articles":[3001,3003],"favorite_id":%s}`, folder)))
		respCode(t, resp, "部分成功")
		data := dataOf(t, resp)
		if got, ok := data["removed_count"].(float64); !ok || int(got) != 1 {
			t.Fatalf("removed_count 应为 1，实际 %v（响应 %v）", data["removed_count"], resp)
		}
		if got, ok := data["failed_count"].(float64); !ok || int(got) != 1 {
			t.Fatalf("failed_count 应为 1，实际 %v（响应 %v）", data["failed_count"], resp)
		}
	})

	t.Run("全部失败：返回非 0 业务码，而不是成功 + 空 data", func(t *testing.T) {
		// 3001/3002 都已被移出、3003 从未收藏过：三条都会失败。
		resp := do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
			[]byte(fmt.Sprintf(`{"articles":[3001,3002,3003],"favorite_id":%s}`, folder)))
		code, _ := resp["code"].(float64)
		if code == 0 {
			t.Fatalf("全部失败不能当成功返回（旧实现 OkWithMsg 吞错）：%v", resp)
		}
		if msg, _ := resp["msg"].(string); msg == "" {
			t.Fatalf("全部失败必须给出明确错误文案：%v", resp)
		}
	})
}

// TestFavoriteCanBeAddedAgainAfterRemovalEndToEnd 守「移出后再收藏同一夹」不能撞唯一键。
//
// 关系行是**软删**的，而唯一键 (article_id, user_id, favor_id) 不含 deleted_at：
// 只按软删后的可见性去查，就会走 Create 撞唯一键，用户收到 1001
// "duplicated key not allowed"。这是修 P0-1/P0-2 时顺带发现的同族缺陷
// （与浏览历史 Upsert 的注释是同一类问题）。
func TestFavoriteCanBeAddedAgainAfterRemovalEndToEnd(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "fav_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "fav_reader", "读者", "", "", schema.RoleUser)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             4001,
		Title:          "移出后再收藏的文章",
		AuthorID:       100,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})

	folder := createFavoriteFolder(t, reader, "反复收藏夹")
	addArticleToFolder(t, reader, "4001", folder)

	respCode(t, do(t, reader, http.MethodDelete, "/api/articles/favorite/contents",
		[]byte(fmt.Sprintf(`{"articles":[4001],"favorite_id":%s}`, folder))), "移出")

	// 再收藏：必须成功，且是复活同一行（含软删的行不能变成两行）。
	respCode(t, do(t, reader, http.MethodPost, "/api/articles/favorite",
		[]byte(fmt.Sprintf(`{"article_id":4001,"favor_id":%s}`, folder))), "再次收藏")

	if got := countFavoriteRows(t, db, 4001, folder); got != 1 {
		t.Fatalf("再次收藏后活关系行应为 1，实际 %d", got)
	}
	if got := countAllFavoriteRows(t, db, 4001, folder); got != 1 {
		t.Fatalf("再次收藏应复活原行而不是插入新行，含软删实际 %d 行", got)
	}
	if !containsArticleID(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite/contents?favorite_id="+folder, nil)), "4001") {
		t.Fatal("再次收藏后收藏内容应含该文章")
	}
}

// ── 辅助 ──
// createFavoriteFolder 走真实 PUT 建夹，再从列表按标题取回 ID。
func createFavoriteFolder(t *testing.T, reader *gin.Engine, title string) string {
	t.Helper()
	respCode(t, do(t, reader, http.MethodPut, "/api/articles/favorite",
		[]byte(fmt.Sprintf(`{"title":%q}`, title))), "建收藏夹 "+title)

	id := findIDByTitle(listOf(t, do(t, reader, http.MethodGet,
		"/api/articles/favorite?user_id=200", nil)), title)
	if id == "" {
		t.Fatalf("收藏夹列表缺 %q", title)
	}
	return id
}

// addArticleToFolder 把文章收藏进指定夹子（POST 用 `favor_id` 字段）。
func addArticleToFolder(t *testing.T, reader *gin.Engine, articleID, folderID string) {
	t.Helper()
	respCode(t, do(t, reader, http.MethodPost, "/api/articles/favorite",
		[]byte(fmt.Sprintf(`{"article_id":%s,"favor_id":%s}`, articleID, folderID))),
		"收藏 "+articleID)
}

// countFavoriteRows 统计某文章在某收藏夹里的活关系行数。
func countFavoriteRows(t *testing.T, db *gorm.DB, articleID uint64, folderID string) int {
	t.Helper()
	var n int64
	if err := db.Model(&schema.UserArticleFavorRow{}).
		Where("article_id = ? AND favor_id = ?", articleID, folderID).
		Count(&n).Error; err != nil {
		t.Fatalf("统计收藏关系行失败: %v", err)
	}
	return int(n)
}

// countAllFavoriteRows 统计含软删的全部关系行。
func countAllFavoriteRows(t *testing.T, db *gorm.DB, articleID uint64, folderID string) int {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(&schema.UserArticleFavorRow{}).
		Where("article_id = ? AND favor_id = ?", articleID, folderID).
		Count(&n).Error; err != nil {
		t.Fatalf("统计全部收藏关系行失败: %v", err)
	}
	return int(n)
}
