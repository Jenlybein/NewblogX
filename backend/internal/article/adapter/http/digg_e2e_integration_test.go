//go:build integration

package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是点赞的端到端验收（#96 AC 第 1、2 条）：走真实 HTTP + 真 MySQL，
// 把「点赞 → 取消 → 再点赞」连击三次，断言每一次都是业务成功且计数正确。
//
// 为什么已经有了仓储契约测试还要这一条：契约测试守的是"关系行与唯一索引的
// 口径"，这一条守的是"整条链路上的错误码与计数" —— 撞唯一索引在线上的表现是
// HTTP 500 + `duplicated key not allowed`，那是 handler 层才看得见的东西。

// TestDiggThreeConsecutiveTogglesSucceed 验证三连击全成功。
func TestDiggThreeConsecutiveTogglesSucceed(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "digg_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "digg_reader", "读者", "", "", schema.RoleUser)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             1001,
		Title:          "点赞端到端文章",
		AuthorID:       100,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
	const articleID = "1001"

	// 三种期望状态：点赞 → 取消 → 再点赞。
	//
	// 关键在第三击：修复前它返回 1001 + "duplicated key not allowed"。
	want := []struct {
		step string
		digg bool
	}{
		{step: "第 1 次点赞", digg: true},
		{step: "第 2 次取消", digg: false},
		{step: "第 3 次再点赞", digg: true},
	}

	for _, w := range want {
		resp := do(t, reader, http.MethodPut, "/api/articles/"+articleID+"/digg", nil)
		code, _ := resp["code"].(float64)
		if code != 0 {
			t.Fatalf("%s 应返回业务码 0，实际 %v（msg=%v）", w.step, resp["code"], resp["msg"])
		}
		if got := dataOf(t, resp)["is_digg"]; got != w.digg {
			t.Fatalf("%s 后 is_digg 应为 %v，实际 %v", w.step, w.digg, got)
		}
		// 每次操作后，库里的活关系行必须与 is_digg 一致。
		if n := countLiveDiggRows(t, db, 1001); (n == 1) != w.digg {
			t.Fatalf("%s 后库里活关系行应%s，实际 %d 条", w.step, boolCN(w.digg), n)
		}
	}

	// 整张表里始终只有一行：不许出现"软删行 + 新行"并存。
	if n := countAllDiggRows(t, db, 1001); n != 1 {
		t.Fatalf("三连击后 article_digg_models 应只有 1 行（含软删），实际 %d 行", n)
	}
}

// TestDiggCountersTrackRelationshipRows 验证计数与关系行严格一致。
//
// 计数走 Redis 增量，无 Redis 时退化为 no-op（本套装配没接 Redis），
// 所以这里断言的是**关系行**与响应里的 is_digg 一致 —— 那才是"计数增减与
// 关系行真实变化一致"在端到端层面能稳定观测的部分。
func TestDiggCountersTrackRelationshipRows(t *testing.T) {
	db := migrate(t)
	seedGoldenUser(t, db, 100, "digg_author", "作者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 200, "digg_reader", "读者", "", "", schema.RoleUser)
	seedGoldenUser(t, db, 201, "digg_reader2", "读者二", "", "", schema.RoleUser)

	reader := newRouter(t, db, domain.NormalUser(ctype.NewID(200)))
	reader2 := newRouter(t, db, domain.NormalUser(ctype.NewID(201)))

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             1002,
		Title:          "计数一致性文章",
		AuthorID:       100,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
	const articleID = "1002"

	// 两个读者各点一次 → 2 条活关系行。
	for _, r := range []struct {
		name   string
		router *gin.Engine
	}{
		{name: "读者一", router: reader},
		{name: "读者二", router: reader2},
	} {
		respCode(t, do(t, r.router, http.MethodPut, "/api/articles/"+articleID+"/digg", nil), r.name+"点赞")
	}
	if n := countLiveDiggRows(t, db, 1002); n != 2 {
		t.Fatalf("两个读者点赞后应有 2 条活关系行，实际 %d", n)
	}

	// 其中一个取消 → 只剩 1 条，且取消者的状态回到未点赞。
	respCode(t, do(t, reader, http.MethodPut, "/api/articles/"+articleID+"/digg", nil), "读者一取消")
	if n := countLiveDiggRows(t, db, 1002); n != 1 {
		t.Fatalf("一个读者取消后应剩 1 条活关系行，实际 %d", n)
	}

	// 取消者重新点赞 → 恢复它自己那一行，总数回到 2，不会撞唯一键。
	respCode(t, do(t, reader, http.MethodPut, "/api/articles/"+articleID+"/digg", nil), "读者一再点赞")
	if n := countLiveDiggRows(t, db, 1002); n != 2 {
		t.Fatalf("再点赞后应回到 2 条活关系行，实际 %d", n)
	}
	if n := countAllDiggRows(t, db, 1002); n != 2 {
		t.Fatalf("再点赞后不应留下软删残行，实际含软删共 %d 行", n)
	}
}

// ── 辅助 ──

// countLiveDiggRows 统计某文章未软删的点赞行数。
func countLiveDiggRows(t *testing.T, db *gorm.DB, articleID uint64) int {
	t.Helper()
	var n int64
	if err := db.Model(&schema.ArticleDiggRow{}).
		Where("article_id = ?", articleID).Count(&n).Error; err != nil {
		t.Fatalf("统计活点赞行失败: %v", err)
	}
	return int(n)
}

// countAllDiggRows 统计含软删的全部点赞行数。
func countAllDiggRows(t *testing.T, db *gorm.DB, articleID uint64) int {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(&schema.ArticleDiggRow{}).
		Where("article_id = ?", articleID).Count(&n).Error; err != nil {
		t.Fatalf("统计全部点赞行失败: %v", err)
	}
	return int(n)
}

// boolCN 把布尔转成中文，用于失败信息。
func boolCN(v bool) string {
	if v {
		return "存在"
	}
	return "不存在"
}
