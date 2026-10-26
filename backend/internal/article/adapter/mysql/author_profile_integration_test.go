//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// TestAuthorProfileIncludesStats 验证「作者卡三项统计」真的从 user_stat 读出来。
//
// 为什么必须有这条测试：`/articles/author_info` 之前只返回 id/username/nickname/avatar，
// 而文章页在渲染「文章数 / 访问量 / 粉丝数」，于是那三项恒为 0。只断言"字段存在"
// 是抓不到的 —— 夹具里没有 user_stat 行时，读到的同样是 0。
// 所以这里显式插入一行非零统计，并断言读回来的值与之相等。
func TestAuthorProfileIncludesStats(t *testing.T) {
	db := migrate(t)
	seedUser(t, db, 100)
	schema.InsertUserStat(t, db, schema.UserStatSpec{
		UserID:              100,
		ViewCount:           900,
		FansCount:           12,
		FollowCount:         3,
		ArticleVisitedCount: 4567,
		ArticleCount:        8,
	})

	view, err := mysql.NewUserPort(db).Profile(context.Background(), domain.UserID(ctype.NewID(100)))
	if err != nil {
		t.Fatalf("读取作者信息失败: %v", err)
	}
	if view.Nickname != "用户100" {
		t.Fatalf("昵称应为 用户100，实际 %q", view.Nickname)
	}
	if view.ArticleCount != 8 {
		t.Fatalf("文章数应为 8，实际 %d", view.ArticleCount)
	}
	if view.ArticleVisitedCount != 4567 {
		t.Fatalf("访问量应为 4567，实际 %d", view.ArticleVisitedCount)
	}
	if view.FansCount != 12 {
		t.Fatalf("粉丝数应为 12，实际 %d", view.FansCount)
	}
}

// TestAuthorProfileWithoutStatRow 验证没有统计行时不报错、保持零值。
//
// 新注册用户可能还没有 user_stat 行；这种"缺数据"不该让作者信息整体失败。
func TestAuthorProfileWithoutStatRow(t *testing.T) {
	db := migrate(t)
	seedUser(t, db, 100)

	view, err := mysql.NewUserPort(db).Profile(context.Background(), domain.UserID(ctype.NewID(100)))
	if err != nil {
		t.Fatalf("缺统计行时不应报错: %v", err)
	}
	if view.ArticleCount != 0 || view.FansCount != 0 {
		t.Fatalf("缺统计行时应为零值，实际 article=%d fans=%d", view.ArticleCount, view.FansCount)
	}
}

// TestTopRepoListCards 验证置顶卡片的一次性投影查询。
//
// 这条 SQL 是 `@Router /articles/top` 的数据来源。它必须做到三件事：
//  1. join 出作者昵称与分类标题（否则首页那行元信息是空的）
//  2. 排除已软删的文章（否则会链接到 404）
//  3. 按置顶时间倒序
func TestTopRepoListCards(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	schema.InsertCategory(t, db, 9, 100, "前端", at)

	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 1001, Title: "置顶甲", Abstract: "摘要甲", AuthorID: 100,
		CategoryID: ptrID(ctype.NewID(9)), ViewCount: 11, PublishStatus: schema.ArticleStatusPublished,
		Visibility: schema.VisibilityVisible, CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 1002, Title: "置顶乙", Abstract: "摘要乙", AuthorID: 200,
		ViewCount: 22, PublishStatus: schema.ArticleStatusPublished,
		Visibility: schema.VisibilityVisible, CreatedAt: at, UpdatedAt: at,
	})
	// 1003 会被软删，不应出现在结果里。
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 1003, Title: "已删除", Abstract: "摘要丙", AuthorID: 100,
		ViewCount: 33, PublishStatus: schema.ArticleStatusPublished,
		Visibility: schema.VisibilityVisible, CreatedAt: at, UpdatedAt: at,
	})
	if err := db.Exec("UPDATE article_models SET deleted_at = ? WHERE id = ?", at, 1003).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	repo := mysql.NewTopRepo(db)
	// 置顶顺序：先 1001（较早），再 1003，再 1002（最新）。倒序后 1002 应排第一。
	for _, id := range []uint64{1001, 1003, 1002} {
		if err := repo.Save(context.Background(), mustEntry(t, 100, id)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", id, err)
		}
		// Save 的 created_at 精度是毫秒，显式拉开以保证排序稳定。
		at = at.Add(time.Minute)
		if err := db.Exec("UPDATE user_top_article_models SET created_at = ? WHERE article_id = ?", at, id).Error; err != nil {
			t.Fatalf("调整置顶时间失败: %v", err)
		}
	}

	cards, err := repo.ListCards(context.Background(), domain.UserID(ctype.NewID(100)))
	if err != nil {
		t.Fatalf("查询置顶卡片失败: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("应返回 2 条（软删的 1003 被排除），实际 %d 条", len(cards))
	}
	if cards[0].ArticleID != domain.ArticleID(ctype.NewID(1002)) {
		t.Fatalf("应按置顶时间倒序，首条应为 1002，实际 %s", cards[0].ArticleID)
	}
	last := cards[1]
	if last.Title != "置顶甲" || last.Abstract != "摘要甲" {
		t.Fatalf("join 出的字段不对: title=%q abstract=%q", last.Title, last.Abstract)
	}
	if last.AuthorNickname != "用户100" {
		t.Fatalf("作者昵称应为 用户100，实际 %q", last.AuthorNickname)
	}
	if last.CategoryTitle != "前端" {
		t.Fatalf("分类标题应为 前端，实际 %q", last.CategoryTitle)
	}
	if last.ViewCount != 11 {
		t.Fatalf("浏览数应为 11，实际 %d", last.ViewCount)
	}
}

// ptrID 返回 ID 的指针（ArticleSpec.CategoryID 是 *ctype.ID）。
func ptrID(id ctype.ID) *ctype.ID { return &id }

// mustEntry 构造置顶关系，失败即终止。
func mustEntry(t *testing.T, userID, articleID uint64) *top.Entry {
	t.Helper()
	return top.NewEntry(domain.UserID(ctype.NewID(userID)), domain.ArticleID(ctype.NewID(articleID)))
}
