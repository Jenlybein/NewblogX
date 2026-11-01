//go:build integration

package cli

import (
	"context"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	articlemysql "myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件验证点赞关系的一次性对账（#96 AC 第 3 条）。
//
// 为什么必须真库：对账的核心是"软删残行看不见但占着唯一键"——
// 这正是内存 fake 表达不了、sqlite 又与生产 DDL 不同源的东西。

// TestReconcileDiggCountsLeftoverRows 验证只读统计能看见残行。
func TestReconcileDiggCountsLeftoverRows(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore)
	seedReconcileFixture(t, db)

	// 2 条活 + 2 条软删残行。
	res, err := reconcileDigg(context.Background(), db, false)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if res.Live != 2 {
		t.Fatalf("活关系行应为 2，实际 %d", res.Live)
	}
	if res.Soft != 2 {
		t.Fatalf("软删残行应为 2，实际 %d", res.Soft)
	}
	if res.Applied || res.Deleted != 0 {
		t.Fatalf("只读模式不应删除任何行: applied=%v deleted=%d", res.Applied, res.Deleted)
	}

	// 只读跑完后数据必须原样。
	if got := countAll(t, db); got != 4 {
		t.Fatalf("只读对账不应改动数据，实际共 %d 行", got)
	}
}

// TestReconcileDiggApplyRemovesLeftoverRows 验证 -apply 真的清掉残行且不动活行。
func TestReconcileDiggApplyRemovesLeftoverRows(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore)
	seedReconcileFixture(t, db)

	res, err := reconcileDigg(context.Background(), db, true)
	if err != nil {
		t.Fatalf("对账(apply)失败: %v", err)
	}
	if res.Deleted != 2 {
		t.Fatalf("应物理删掉 2 条残行，实际 %d", res.Deleted)
	}
	if !res.Applied {
		t.Fatal("apply 模式应标记 Applied")
	}

	// 活行一条都不能少，残行一条都不留。
	if got := countLive(t, db); got != 2 {
		t.Fatalf("清理后应保留 2 条活关系行，实际 %d", got)
	}
	if got := countAll(t, db); got != 2 {
		t.Fatalf("清理后不应残留软删行，实际共 %d 行", got)
	}

	// 幂等：再跑一次没有可删的。
	again, err := reconcileDigg(context.Background(), db, true)
	if err != nil {
		t.Fatalf("二次对账失败: %v", err)
	}
	if again.Soft != 0 || again.Deleted != 0 {
		t.Fatalf("二次对账应无可清理项，实际 soft=%d deleted=%d", again.Soft, again.Deleted)
	}
}

// TestReconcileDiggThenRelikeWorks 验证清理之后再次点赞仍然正常。
//
// 这是"清理"与"修复"两件事的接缝：清掉残行之后，用户再点赞走的是
// **新建**分支（而不是恢复），这条分支必须同样成功——
// 否则"清完残行反而点不了赞"就成了新的故障。
func TestReconcileDiggThenRelikeWorks(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore)
	seedReconcileFixture(t, db)

	if _, err := reconcileDigg(context.Background(), db, true); err != nil {
		t.Fatalf("清理失败: %v", err)
	}

	diggs := articlemysql.NewDiggPort(db)
	ctx := context.Background()
	articleID := domain.ArticleID(ctype.NewID(1001))

	// 残行被清掉的两个用户（300/301）重新点赞：走新建分支。
	for _, uid := range []uint64{300, 301} {
		userID := domain.UserID(ctype.NewID(uid))
		added, err := diggs.Add(ctx, articleID, userID)
		if err != nil {
			t.Fatalf("清理后重新点赞失败（uid=%d）: %v", uid, err)
		}
		if !added {
			t.Fatalf("清理后重新点赞应报真新增（uid=%d）", uid)
		}
		exists, err := diggs.Exists(ctx, articleID, userID)
		if err != nil || !exists {
			t.Fatalf("清理后重新点赞应处于已点赞（uid=%d）: exists=%v err=%v", uid, exists, err)
		}
	}

	// 原有的两条活关系不受影响，总数变成 4 条活行、0 条残行。
	if got := countLive(t, db); got != 4 {
		t.Fatalf("清理并重新点赞后应有 4 条活关系行，实际 %d", got)
	}
	if got := countAll(t, db); got != 4 {
		t.Fatalf("不应残留软删行，实际共 %d 行", got)
	}

	// 取消 → 再点赞这条链路在清理之后依然成立。
	userID := domain.UserID(ctype.NewID(300))
	if removed, err := diggs.Remove(ctx, articleID, userID); err != nil || !removed {
		t.Fatalf("清理后取消点赞失败: removed=%v err=%v", removed, err)
	}
	if added, err := diggs.Add(ctx, articleID, userID); err != nil || !added {
		t.Fatalf("清理后取消再点赞失败: added=%v err=%v", added, err)
	}
}

// ── 辅助 ──

// seedReconcileFixture 造 2 条活关系 + 2 条软删残行。
func seedReconcileFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	schema.InsertUser(t, db, schema.UserSpec{
		ID: 100, Username: "rc_author", Nickname: "作者", CreatedAt: at, UpdatedAt: at,
	})
	for _, uid := range []uint64{200, 201, 300, 301} {
		schema.InsertUser(t, db, schema.UserSpec{
			ID: uid, Username: "u" + strconv.FormatUint(uid, 10), Nickname: "用户",
			CreatedAt: at, UpdatedAt: at,
		})
	}
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: 1001, Title: "对账文章", AuthorID: 100, CommentsToggle: true,
		PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CreatedAt: at, UpdatedAt: at,
	})

	// 200/201 是活关系；300/301 是取消点赞留下的软删残行。
	for i, uid := range []uint64{200, 201} {
		schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{
			ID: uint64(5000 + i), ArticleID: 1001, UserID: uid, At: at,
		})
	}
	for i, uid := range []uint64{300, 301} {
		schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{
			ID: uint64(6000 + i), ArticleID: 1001, UserID: uid, SoftDeleted: true, At: at,
		})
	}
}

// countLive / countAll 统计 article_digg_models 的行数。
func countLive(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int64
	if err := db.Model(&diggRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计活行失败: %v", err)
	}
	return int(n)
}

func countAll(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(&diggRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计全部行失败: %v", err)
	}
	return int(n)
}
