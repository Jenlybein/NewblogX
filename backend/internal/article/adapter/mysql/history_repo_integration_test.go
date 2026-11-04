//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是浏览历史的**查询契约**用例（#241）：
//   - 游标分页（before_updated_at + before_id）替代 offset —— 删除/置顶后不位移；
//   - keyword 按文章标题过滤（历史表只存关联，标题来自 article_models）；
//   - 90 天清理（固定时钟）与"清空后重新浏览要复活同一行"（唯一键不带 deleted_at）。
//
// 这些都必须真 MySQL：分页是 ORDER BY + 比较、关键词是 LIKE、去重是唯一键。

const historyReader uint64 = 300

// seedHistoryReader 插入浏览历史的主人（外键需要它存在）。
func seedHistoryReader(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{
		ID:       historyReader,
		Username: "hist_reader",
		Nickname: "历史读者",
		Role:     schema.RoleUser,
	})
}

// seedHistoryArticles 插入若干已发布可见文章，标题可指定。
func seedHistoryArticles(t *testing.T, db *gorm.DB, specs []struct {
	id    uint64
	title string
}) {
	t.Helper()
	seedVisibilityUsers(t, db)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, spec := range specs {
		schema.InsertArticle(t, db, schema.ArticleSpec{
			ID:             spec.id,
			Title:          spec.title,
			AuthorID:       visNormalAuthor,
			CommentsToggle: true,
			PublishStatus:  schema.ArticleStatusPublished,
			Visibility:     schema.VisibilityVisible,
			CreatedAt:      at,
			UpdatedAt:      at,
		})
	}
}

// listHistory 是仓储读一页的简写。
func listHistory(t *testing.T, db *gorm.DB, q viewhistory.ListQuery) []*viewhistory.Record {
	t.Helper()
	repo := mysql.NewViewHistoryRepo(db)
	records, err := repo.List(context.Background(), q)
	if err != nil {
		t.Fatalf("列浏览历史失败: %v", err)
	}
	return records
}

// articleIDs 把记录压成 ID 序列，便于比对顺序。
func articleIDs(records []*viewhistory.Record) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r.ArticleID().String())
	}
	return out
}

// TestViewHistoryCursorPaginationKeepsDayGroupsContiguous 验证游标分页的核心承诺。
//
// 场景：5 条浏览记录按时间倒序（t5 最新）。每页 2 条逐页读，期间**删掉已读过的
// 一条**再继续 —— offset 分页在删除后会让下一页整体前移（把一条挤掉，按天分组
// 就此断页）；游标分页必须逐条不漏不重。
func TestViewHistoryCursorPaginationKeepsDayGroupsContiguous(t *testing.T) {
	db := migrate(t)
	seedHistoryReader(t, db)
	seedHistoryArticles(t, db, []struct {
		id    uint64
		title string
	}{{1001, "第一篇"}, {1002, "第二篇"}, {1003, "第三篇"}, {1004, "第四篇"}, {1005, "第五篇"}})

	// t5 > t4 > t3 > t2 > t1，同一时刻内靠 article_id 倒序兜底。
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		schema.InsertViewHistory(t, db, uint64(5000+i), historyReader, uint64(1000+i), base.Add(time.Duration(i)*time.Hour))
	}
	// 文章 1002 与 1003 的浏览时间相同，钉住第二排序键（article_id desc）。
	if err := db.Exec("UPDATE user_article_view_history_models SET updated_at = ? WHERE article_id = ?",
		base.Add(3*time.Hour), 1002).Error; err != nil {
		t.Fatalf("对齐浏览时间失败: %v", err)
	}

	q := viewhistory.ListQuery{UserID: domain.UserID(ctype.NewID(historyReader)), Limit: 2}
	page1 := listHistory(t, db, q)
	got := articleIDs(page1)
	// 同刻的两条按 article_id 倒序：1003 在 1002 之前。
	if len(got) != 2 || got[0] != "1005" || got[1] != "1004" {
		t.Fatalf("第 1 页应为 [1005 1004]，实际 %v", got)
	}

	// 删掉第 1 页一条（用户移出历史），再用游标续页。
	repo := mysql.NewViewHistoryRepo(db)
	if err := repo.Delete(context.Background(), domain.UserID(ctype.NewID(historyReader)),
		[]domain.ArticleID{domain.ArticleID(ctype.NewID(1005))}); err != nil {
		t.Fatalf("删除历史失败: %v", err)
	}

	last := page1[len(page1)-1]
	cursor := &viewhistory.Cursor{ViewedAt: last.ViewedAt(), ArticleID: last.ArticleID()}
	page2 := listHistory(t, db, viewhistory.ListQuery{
		UserID: domain.UserID(ctype.NewID(historyReader)),
		Before: cursor,
		Limit:  2,
	})
	got = articleIDs(page2)
	if len(got) != 2 || got[0] != "1003" || got[1] != "1002" {
		t.Fatalf("删除已读条目后第 2 页应为 [1003 1002]（不因删除而位移），实际 %v", got)
	}

	last = page2[len(page2)-1]
	page3 := listHistory(t, db, viewhistory.ListQuery{
		UserID: domain.UserID(ctype.NewID(historyReader)),
		Before: &viewhistory.Cursor{ViewedAt: last.ViewedAt(), ArticleID: last.ArticleID()},
		Limit:  2,
	})
	if got = articleIDs(page3); len(got) != 1 || got[0] != "1001" {
		t.Fatalf("第 3 页应只剩 [1001]，实际 %v", got)
	}

	// 三页并起来不重不漏：第 1 页取到后 1005 才被删，所以它仍算一次；
	// 关键是删除**不影响后续页**——1004 之后不会跳条也不会重复。
	seen := map[string]int{}
	for _, page := range [][]*viewhistory.Record{page1, page2, page3} {
		for _, id := range articleIDs(page) {
			seen[id]++
		}
	}
	if len(seen) != 5 {
		t.Fatalf("三页应覆盖 5 篇（含删除前已读的 1005），实际 %v", seen)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("文章 %s 在游标分页中出现了 %d 次（应为 1 次）: %v", id, count, seen)
		}
	}
}

// TestViewHistoryKeywordMatchesArticleTitle 验证关键词按文章标题过滤。
//
// 历史表里没有标题快照，所以这是一次 join 条件；写错了表现为"搜了标题却
// 搜不到自己看过的文章"，而别的读面都正常。
func TestViewHistoryKeywordMatchesArticleTitle(t *testing.T) {
	db := migrate(t)
	seedHistoryReader(t, db)
	seedHistoryArticles(t, db, []struct {
		id    uint64
		title string
	}{{1001, "Go 并发模型"}, {1002, "Rust 所有权"}, {1003, "Go 泛型"}})

	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for k, articleID := range []uint64{1001, 1002, 1003} {
		schema.InsertViewHistory(t, db, uint64(7000+k), historyReader, articleID, at.Add(time.Duration(k)*time.Minute))
	}

	userID := domain.UserID(ctype.NewID(historyReader))
	all := listHistory(t, db, viewhistory.ListQuery{UserID: userID, Limit: 10})
	if len(all) != 3 {
		t.Fatalf("不带关键词应返回 3 条，实际 %d", len(all))
	}

	// 命中两篇 Go，倒序（1003 更晚）。
	got := articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Keyword: "Go", Limit: 10}))
	if len(got) != 2 || got[0] != "1003" || got[1] != "1001" {
		t.Fatalf("关键词 Go 应命中 [1003 1001]，实际 %v", got)
	}

	// 排序规则不区分大小写（utf8mb4_0900_ai_ci）：小写同样命中。
	got = articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Keyword: "go", Limit: 10}))
	if len(got) != 2 {
		t.Fatalf("小写 go 应同样命中 2 条，实际 %v", got)
	}

	// 关键词与游标叠加：先按 Go，再用最后一条续页，只剩另一篇。
	first := listHistory(t, db, viewhistory.ListQuery{UserID: userID, Keyword: "Go", Limit: 1})
	if len(first) != 1 || first[0].ArticleID().String() != "1003" {
		t.Fatalf("关键词+limit=1 应返回 1003，实际 %v", articleIDs(first))
	}
	next := listHistory(t, db, viewhistory.ListQuery{
		UserID:  userID,
		Keyword: "Go",
		Before:  &viewhistory.Cursor{ViewedAt: first[0].ViewedAt(), ArticleID: first[0].ArticleID()},
		Limit:   1,
	})
	if len(next) != 1 || next[0].ArticleID().String() != "1001" {
		t.Fatalf("关键词+游标续页应返回 1001，实际 %v", articleIDs(next))
	}

	// 未命中：空列表而不是报错。
	if got = articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Keyword: "Kotlin", Limit: 10})); len(got) != 0 {
		t.Fatalf("未命中应返回空列表，实际 %v", got)
	}
}

// TestViewHistoryUpsertRevivesSoftDeletedRow 验证"移出/清理后再浏览"不会撞唯一键。
//
// uk_user_article_history 是 (article_id, user_id)，**不含** deleted_at：
// 若 Upsert 用带软删过滤的查询去找旧行，就会走 Create 撞唯一键 ——
// 表现是用户重新打开一篇看过的文章时接口 500。所以必须 Unscoped 查并复活那一行。
func TestViewHistoryUpsertRevivesSoftDeletedRow(t *testing.T) {
	db := migrate(t)
	seedHistoryReader(t, db)
	seedHistoryArticles(t, db, []struct {
		id    uint64
		title string
	}{{1001, "看过的文章"}})

	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	schema.InsertViewHistory(t, db, 8001, historyReader, 1001, at)

	repo := mysql.NewViewHistoryRepo(db)
	ctx := context.Background()
	userID := domain.UserID(ctype.NewID(historyReader))

	if err := repo.Delete(ctx, userID, []domain.ArticleID{domain.ArticleID(ctype.NewID(1001))}); err != nil {
		t.Fatalf("移出历史失败: %v", err)
	}
	// 软删后列表为空。
	if got := articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Limit: 10})); len(got) != 0 {
		t.Fatalf("移出后列表应为空，实际 %v", got)
	}

	// 再次浏览同一篇：复活那一行。
	revisit := at.Add(2 * time.Hour)
	if err := repo.Upsert(ctx, viewhistory.NewRecord(userID, domain.ArticleID(ctype.NewID(1001)), revisit)); err != nil {
		t.Fatalf("再次浏览应复活记录，实际 %v", err)
	}

	got := articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Limit: 10}))
	if len(got) != 1 || got[0] != "1001" {
		t.Fatalf("复活后应重新出现且只有 1 条，实际 %v", got)
	}

	// 库里仍然只有 1 行（含软删行）——Create 会撞唯一键，这里同时反证没有新增行。
	var rows int64
	if err := db.Unscoped().Model(&schema.UserArticleViewHistoryRow{}).
		Where("article_id = ? AND user_id = ?", 1001, historyReader).Count(&rows).Error; err != nil {
		t.Fatalf("统计历史行失败: %v", err)
	}
	if rows != 1 {
		t.Fatalf("同一 (article, user) 应只有 1 行（含软删），实际 %d", rows)
	}

	// 浏览时间被刷新（复活不是"回到原来的位置"）。
	rec := listHistory(t, db, viewhistory.ListQuery{UserID: userID, Limit: 10})[0]
	if !rec.ViewedAt().Equal(revisit) {
		t.Fatalf("复活后浏览时间应刷新为 %s，实际 %s", revisit, rec.ViewedAt())
	}
}

// TestPurgeViewHistoryBeforeWithFixedClock 是 90 天清理的固定时钟用例。
//
// 边界：now-91d 被清理、now-90d（恰好 90 天前）保留、now-89d 保留；
// 软删（库里行还在、deleted_at 有值）；重复执行幂等（第二次 0 行）。
func TestPurgeViewHistoryBeforeWithFixedClock(t *testing.T) {
	db := migrate(t)
	seedHistoryReader(t, db)
	seedHistoryArticles(t, db, []struct {
		id    uint64
		title string
	}{{1001, "91 天前"}, {1002, "恰好 90 天"}, {1003, "89 天前"}})

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	schema.InsertViewHistory(t, db, 9001, historyReader, 1001, now.Add(-91*24*time.Hour))
	schema.InsertViewHistory(t, db, 9002, historyReader, 1002, now.Add(-90*24*time.Hour))
	schema.InsertViewHistory(t, db, 9003, historyReader, 1003, now.Add(-89*24*time.Hour))

	ctx := context.Background()
	cutoff := viewhistory.PurgeCutoff(now)
	// cutoff 是固定值：钉住"清理口径从调用方传进来"，不在 SQL 里取 now()。
	if !cutoff.Equal(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("固定时钟的清理界限应为 2026-08-06T12:00:00Z，实际 %s", cutoff)
	}

	affected, err := mysql.PurgeViewHistoryBefore(ctx, db, cutoff)
	if err != nil {
		t.Fatalf("清理历史失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("应只清理 1 条（now-91d），实际 %d", affected)
	}

	userID := domain.UserID(ctype.NewID(historyReader))
	left := articleIDs(listHistory(t, db, viewhistory.ListQuery{UserID: userID, Limit: 10}))
	if len(left) != 2 || left[0] != "1003" || left[1] != "1002" {
		t.Fatalf("剩余应为 [1003 1002]，实际 %v", left)
	}

	// 软删：行还在（deleted_at 有值），只是不再出现在列表里。
	var rows int64
	if err := db.Unscoped().Model(&schema.UserArticleViewHistoryRow{}).
		Where("deleted_at IS NOT NULL").Count(&rows).Error; err != nil {
		t.Fatalf("统计软删行失败: %v", err)
	}
	if rows != 1 {
		t.Fatalf("应有 1 行软删记录，实际 %d", rows)
	}

	// 幂等：同一时钟再跑一次不再命中任何行。
	affected, err = mysql.PurgeViewHistoryBefore(ctx, db, cutoff)
	if err != nil {
		t.Fatalf("第二次清理失败: %v", err)
	}
	if affected != 0 {
		t.Fatalf("重复清理应 0 行，实际 %d", affected)
	}
}
