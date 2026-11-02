//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/kernel/enum"
	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// migrateSidebarData 建右栏三种内置卡读投影所需的表。
//
// 顺序不能乱：AutoMigrate 会按关联建外键，被引用的表必须先存在。
// article_hot_top_models 在 UserReading 组里，评论表在 Comment 组里。
func migrateSidebarData(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.UserReading, schema.Comment)
	return db
}

// seedSidebarAuthors 插入文章作者（右栏数据本身不按角色过滤，这里只满足外键）。
func seedSidebarAuthors(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{ID: 100, Username: "sidebar_admin", Nickname: "管理员作者", Role: schema.RoleAdmin})
	schema.InsertUser(t, db, schema.UserSpec{ID: 200, Username: "sidebar_user", Nickname: "普通作者", Role: schema.RoleUser})
}

// insertSidebarComment 写一条评论。
//
// 走原生 SQL 而不是结构体：根评论的 reply_id 必须落 NULL，写成 0 会撞
// comment_models 的自引用外键（库里没有 id=0 的行）。既有评论用例也是这个口径。
func insertSidebarComment(t *testing.T, db *gorm.DB, id, articleID uint64, content string, status enum.CommentStatus, at time.Time) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO comment_models (id, created_at, updated_at, content, user_id, user_nickname, article_id, reply_id, status)
		 VALUES (?, ?, ?, ?, NULL, ?, ?, NULL, ?)`,
		id, at, at, content, "读者甲", articleID, int(status),
	).Error
	if err != nil {
		t.Fatalf("插入评论 %d 失败: %v", id, err)
	}
}

// TestSidebarDataHotArticlesOrdersByDiggAndExcludesPinned 锁定热门文章卡的两条口径：
// 按点赞数降序取公开文章；已经出现在主栏「热门置顶」的文章不再重复露出。
func TestSidebarDataHotArticlesOrdersByDiggAndExcludesPinned(t *testing.T) {
	t.Parallel()

	db := migrateSidebarData(t)
	seedSidebarAuthors(t, db)

	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3001, Title: "点赞最高但已置顶", AuthorID: 100, DiggCount: 90,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3002, Title: "点赞第二", AuthorID: 100, DiggCount: 80,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3003, Title: "点赞第三", AuthorID: 200, DiggCount: 70,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	// 草稿与用户隐藏的文章不是公开文章，不能进热门文章卡。
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3004, Title: "草稿", AuthorID: 100, DiggCount: 60,
		PublishStatus: schema.ArticleStatusDraft, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3005, Title: "用户隐藏", AuthorID: 100, DiggCount: 50,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityUserHidden,
		CreatedAt: at, UpdatedAt: at,
	})

	// 3001 是主栏运营位，右栏不能再出现；9002 是已取消（软删）的置顶行，不能继续排除文章。
	schema.InsertArticleHotTop(t, db, 9001, 3001, 1, at)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3006, Title: "曾置顶后又取消", AuthorID: 100, DiggCount: 40,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticleHotTop(t, db, 9002, 3006, 2, at)
	if err := db.Exec("UPDATE article_hot_top_models SET deleted_at = ? WHERE id = ?", at, 9002).Error; err != nil {
		t.Fatalf("软删置顶行失败: %v", err)
	}

	data := sitemysql.NewSidebarData(db)
	got, err := data.HotArticles(context.Background(), 5)
	if err != nil {
		t.Fatalf("读取热门文章失败: %v", err)
	}

	want := []uint64{3002, 3003, 3006}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 条热门文章（置顶、草稿、隐藏、已取消置顶各自处理），实际 %d 条: %+v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ID.Uint64() != id {
			t.Fatalf("第 %d 条应为 %d（点赞降序且剔除仍在架的置顶文章），实际 %d", i, id, got[i].ID.Uint64())
		}
	}
	if got[0].DiggCount != 80 || got[0].Title != "点赞第二" {
		t.Fatalf("热门文章应带点赞数与标题: %+v", got[0])
	}
}

// TestSidebarDataLatestCommentsOrdersByTimeDesc 锁定最新评论卡：按评论时间倒序，
// 只取公开文章下已发布的评论。
func TestSidebarDataLatestCommentsOrdersByTimeDesc(t *testing.T) {
	t.Parallel()

	db := migrateSidebarData(t)
	seedSidebarAuthors(t, db)

	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3101, Title: "公开文章", AuthorID: 100,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 3102, Title: "草稿文章", AuthorID: 100,
		PublishStatus: schema.ArticleStatusDraft, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})

	insertSidebarComment(t, db, 4001, 3101, "最早", enum.CommentStatusPublished, at.Add(1*time.Hour))
	insertSidebarComment(t, db, 4002, 3101, "最新", enum.CommentStatusPublished, at.Add(3*time.Hour))
	insertSidebarComment(t, db, 4003, 3101, "中间", enum.CommentStatusPublished, at.Add(2*time.Hour))
	insertSidebarComment(t, db, 4004, 3102, "草稿下的评论", enum.CommentStatusPublished, at.Add(4*time.Hour))
	insertSidebarComment(t, db, 4005, 3101, "已删除的评论", enum.CommentStatusDeleted, at.Add(5*time.Hour))

	data := sitemysql.NewSidebarData(db)
	got, err := data.LatestComments(context.Background(), 5)
	if err != nil {
		t.Fatalf("读取最新评论失败: %v", err)
	}

	want := []uint64{4002, 4003, 4001}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 条最新评论，实际 %d 条: %+v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ID.Uint64() != id {
			t.Fatalf("第 %d 条应为 %d（时间倒序），实际 %d", i, id, got[i].ID.Uint64())
		}
	}
	if got[0].ArticleTitle != "公开文章" || got[0].UserNickname != "读者甲" {
		t.Fatalf("最新评论应带文章标题与作者昵称: %+v", got[0])
	}
}

// TestSidebarDataTagCloudOrdersByWeight 锁定标签云：按文章数（字号权重）降序，
// 停用的标签不出现。
func TestSidebarDataTagCloudOrdersByWeight(t *testing.T) {
	t.Parallel()

	db := migrateSidebarData(t)
	seedSidebarAuthors(t, db)

	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	schema.InsertTag(t, db, schema.TagSpec{ID: 5001, Title: "中等", ArticleCount: 20, Enabled: true, CreatedBy: 100, At: at})
	schema.InsertTag(t, db, schema.TagSpec{ID: 5002, Title: "最多", ArticleCount: 30, Enabled: true, CreatedBy: 100, At: at})
	schema.InsertTag(t, db, schema.TagSpec{ID: 5003, Title: "最少", ArticleCount: 10, Enabled: true, CreatedBy: 100, At: at})
	schema.InsertTag(t, db, schema.TagSpec{ID: 5004, Title: "停用标签", ArticleCount: 99, Enabled: false, CreatedBy: 100, At: at})

	data := sitemysql.NewSidebarData(db)
	got, err := data.TagCloud(context.Background(), 5)
	if err != nil {
		t.Fatalf("读取标签云失败: %v", err)
	}

	want := []uint64{5002, 5001, 5003}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 个标签，实际 %d 个: %+v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ID.Uint64() != id {
			t.Fatalf("第 %d 个标签应为 %d（文章数降序），实际 %d", i, id, got[i].ID.Uint64())
		}
	}
	if got[0].ArticleCount != 30 || got[0].Title != "最多" {
		t.Fatalf("标签应带文章数与标题: %+v", got[0])
	}
}
