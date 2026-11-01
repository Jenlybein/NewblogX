//go:build integration

package mysql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是「点赞关系行口径」的契约测试（#96）。
//
// 为什么必须真 MySQL：缺陷的根因是 `uk_article_digg (article_id, user_id)`
// **不含 deleted_at**，而「取消点赞」走 gorm 的默认 Delete（软删），行留在表里。
// 于是 `Exists`（带 `deleted_at IS NULL`）与唯一索引的口径不一致：
// 软删后 Exists 返回 false → Add 走 INSERT → 撞唯一索引。
//
// 这条只有真库能抓住：内存 fake 没有唯一索引，sqlite 上 gorm 建出的索引
// 与生产 DDL 也不是同一份。口径不一致时本文件必然变红。
//
// 定案口径（见 docs/notes/2026-11-02-grill-session.md ⑳-B）：
// **取消点赞后不留残行的等价物**——Add 时若发现"同一 (article_id, user_id)
// 的软删行"就恢复它，而不是插入新行。软删行对业务而言仍是一条"已存在"的关系。

const (
	diggAuthor uint64 = 100
	diggReader uint64 = 200
)

// TestDiggToggleThreeTimesKeepsConsistent 验证点赞→取消→再点赞三次连击。
//
// 这是用户视角的复现路径（AC 第 1、2 条）：三次都要成功，且每次的
// "是否有变化"必须与关系行的真实变化一致 —— 只有真新增才 +1，只有真移除才 −1。
func TestDiggToggleThreeTimesKeepsConsistent(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	diggs := mysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1001))
	userID := domain.UserID(ctype.NewID(diggReader))
	seedDiggArticle(t, db, 1001)

	// 第一次点赞：真新增。
	added, err := diggs.Add(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("第一次点赞失败（线上表现为 duplicated key）: %v", err)
	}
	if !added {
		t.Fatal("第一次点赞应报真新增")
	}
	if !existsDigg(t, diggs, articleID, userID) {
		t.Fatal("第一次点赞后应处于已点赞")
	}
	if n := countDiggRows(t, db, 1001, false); n != 1 {
		t.Fatalf("第一次点赞后应有 1 条活关系行，实际 %d", n)
	}

	// 取消点赞：真移除。
	removed, err := diggs.Remove(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("取消点赞失败: %v", err)
	}
	if !removed {
		t.Fatal("取消点赞应报真移除")
	}
	if existsDigg(t, diggs, articleID, userID) {
		t.Fatal("取消点赞后不应再处于已点赞")
	}

	// 第二次点赞：这就是线上报错的那一次。
	//
	// 修好之后它必须成功；修好之前这里返回 duplicated key 错误。
	added, err = diggs.Add(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("取消后再点赞失败（撞唯一索引）: %v", err)
	}
	if !added {
		t.Fatal("取消后再点赞应报真新增（此前是软删行，恢复它是一次真实变化）")
	}
	if !existsDigg(t, diggs, articleID, userID) {
		t.Fatal("取消后再点赞后应处于已点赞")
	}

	// 关键：整张表里始终只有 1 行，且是活行。
	//
	// 口径若不统一，这里会看到"1 条软删行 + 插入失败"或"2 行"。
	if n := countDiggRows(t, db, 1001, false); n != 1 {
		t.Fatalf("再点赞后应有且仅有 1 条活关系行，实际 %d", n)
	}
	if n := countDiggRows(t, db, 1001, true); n != 1 {
		t.Fatalf("再点赞后不应留下软删残行，实际含软删共 %d 行", n)
	}
}

// TestDiggAddIsIdempotentWhenAlreadyLiked 验证重复点赞不是错误。
//
// 与上面那条互补：那里覆盖"软删行要能恢复"，这里覆盖"活行不能重复计数"。
// Add 返回 false 表示"没有发生真实变化"，调用方据此不加计数、不发通知。
func TestDiggAddIsIdempotentWhenAlreadyLiked(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	diggs := mysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1002))
	userID := domain.UserID(ctype.NewID(diggReader))
	seedDiggArticle(t, db, 1002)

	if added, err := diggs.Add(ctx, articleID, userID); err != nil || !added {
		t.Fatalf("首次点赞失败: added=%v err=%v", added, err)
	}

	// 再点一次：活行已存在，不应报错、不应新增。
	added, err := diggs.Add(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("重复点赞不应报错: %v", err)
	}
	if added {
		t.Fatal("重复点赞应返回无变化（false），否则计数会多加一次")
	}
	if n := countDiggRows(t, db, 1002, true); n != 1 {
		t.Fatalf("重复点赞后仍应只有 1 行，实际 %d", n)
	}
}

// TestDiggRemoveWhenNeverLiked 验证取消未点的赞不是错误。
//
// 计数一致性要求它返回 false（没有真实移除），调用方据此不减计数。
func TestDiggRemoveWhenNeverLiked(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	diggs := mysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1003))
	userID := domain.UserID(ctype.NewID(diggReader))
	seedDiggArticle(t, db, 1003)

	removed, err := diggs.Remove(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("取消未点的赞不应报错: %v", err)
	}
	if removed {
		t.Fatal("取消未点的赞应返回无变化（false），否则计数会多减一次")
	}
}

// TestDiggDeleteByArticleRemovesRows 验证删文章的级联清理。
//
// 级联与"取消点赞"是两条不同的写入路径：级联要求行真的不再占位，
// 否则删文章后重新建一篇同 ID 的文章会撞上老关系行。
func TestDiggDeleteByArticleRemovesRows(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	diggs := mysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1004))
	seedDiggArticle(t, db, 1004)

	for _, uid := range []uint64{diggReader, 300} {
		// 200 由 seedUsers 建好；300 是本文件额外的读者。
		if uid != diggReader {
			schema.InsertUser(t, db, schema.UserSpec{
				ID: uid, Username: "u" + strconv.FormatUint(uid, 10), Nickname: "用户",
			})
		}
		if _, err := diggs.Add(ctx, articleID, domain.UserID(ctype.NewID(uid))); err != nil {
			t.Fatalf("点赞失败: %v", err)
		}
	}

	n, err := diggs.DeleteByArticle(ctx, articleID)
	if err != nil {
		t.Fatalf("级联删除失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("应清理 2 条点赞，实际 %d", n)
	}
	// 含软删一起数：级联不应留下残行。
	if got := countDiggRows(t, db, 1004, true); got != 0 {
		t.Fatalf("级联删除后不应留下任何行（含软删），实际 %d", got)
	}
}

// TestDiggLegacySoftDeletedRowDoesNotBlockRelike 复现线上残行场景（AC 第 3 条）。
//
// 线上在修复前已经积累了软删残行：用户取消点赞后行留在表里、占着唯一键。
// 本用例直接插一行"已经软删"的老数据（不经过 Remove，模拟线上存量），
// 再走正常点赞路径，断言它不再报错——这正是"老用户仍会踩到"的那个场景。
//
// 结论：定案口径下残行只是历史痕迹，不再阻断再次点赞；
// 需要清理时按 docs 里的一次性对账步骤做，但**不清理也不会再报错**。
func TestDiggLegacySoftDeletedRowDoesNotBlockRelike(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	diggs := mysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1005))
	userID := domain.UserID(ctype.NewID(diggReader))
	seedDiggArticle(t, db, 1005)

	// 直接落一行软删残行：这就是修复前"取消点赞"留在库里的形态。
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{
		ID: 9001, ArticleID: 1005, UserID: diggReader, SoftDeleted: true,
	})

	// 存量残行不该被当成"已点赞"。
	if existsDigg(t, diggs, articleID, userID) {
		t.Fatal("软删残行不应被视作已点赞")
	}
	// 也不该算进活关系行数（否则级联清理会多报）。
	if n := countDiggRows(t, db, 1005, false); n != 0 {
		t.Fatalf("软删残行不应计入活关系行，实际 %d", n)
	}

	// 关键：老用户再次点赞必须成功，并且是"恢复"而不是"再插一行"。
	added, err := diggs.Add(ctx, articleID, userID)
	if err != nil {
		t.Fatalf("存在历史软删残行时再点赞失败: %v", err)
	}
	if !added {
		t.Fatal("恢复历史残行应算作一次真实变化（计数 +1）")
	}
	if !existsDigg(t, diggs, articleID, userID) {
		t.Fatal("再点赞后应处于已点赞")
	}
	// 表里仍然只有那一行，且已经复活。
	if n := countDiggRows(t, db, 1005, false); n != 1 {
		t.Fatalf("再点赞后应有且仅有 1 条活关系行，实际 %d", n)
	}
	if n := countDiggRows(t, db, 1005, true); n != 1 {
		t.Fatalf("再点赞后不应另有残行，实际含软删共 %d 行", n)
	}

	// 恢复之后整条取消→再点赞链路继续可用。
	removed, err := diggs.Remove(ctx, articleID, userID)
	if err != nil || !removed {
		t.Fatalf("恢复后取消失败: removed=%v err=%v", removed, err)
	}
	if added, err := diggs.Add(ctx, articleID, userID); err != nil || !added {
		t.Fatalf("恢复后再点赞失败: added=%v err=%v", added, err)
	}
	if n := countDiggRows(t, db, 1005, true); n != 1 {
		t.Fatalf("多轮切换后仍应只有 1 行，实际 %d 行", n)
	}
}

// ── 辅助 ──

// seedDiggArticle 插入一篇已发布可见的文章（点赞表有指向文章表的外键）。
func seedDiggArticle(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          "点赞契约文章",
		AuthorID:       diggAuthor,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
		CreatedAt:      at,
		UpdatedAt:      at,
	})
}

// existsDigg 通过端口读取"是否已点赞"。
func existsDigg(t *testing.T, diggs interface {
	Exists(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
}, articleID domain.ArticleID, userID domain.UserID) bool {
	t.Helper()
	got, err := diggs.Exists(context.Background(), articleID, userID)
	if err != nil {
		t.Fatalf("读取点赞状态失败: %v", err)
	}
	return got
}

// countDiggRows 直接数 article_digg_models 的行数。
//
// withDeleted 为真时含软删行（Unscoped）。这是本文件的关键判据：
// 只有它能区分"行真的没了"和"行被软删但还在占位"。
func countDiggRows(t *testing.T, db *gorm.DB, articleID uint64, withDeleted bool) int {
	t.Helper()
	q := db.Model(&schema.ArticleDiggRow{})
	if withDeleted {
		q = q.Unscoped()
	}
	var n int64
	if err := q.Where("article_id = ?", articleID).Count(&n).Error; err != nil {
		t.Fatalf("统计点赞行失败: %v", err)
	}
	return int(n)
}
