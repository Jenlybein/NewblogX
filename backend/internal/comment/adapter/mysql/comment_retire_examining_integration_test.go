//go:build integration

package mysql_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/comment/adapter/mysql"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// legacyExaminingStatus 是已退场的「审核中」取值（票 400）。
//
// 测试里用字面量而不是 domain 常量：domain 里已经**没有**这个状态了，
// 这正是这条迁移存在的理由 —— 数据里还可能有它。
const legacyExaminingStatus domain.Status = 1

// TestRetireExaminingMigratesLegacyRows 是真 MySQL 契约：存量「审核中」转已删除。
//
// 为什么要真库：这条迁移的价值全在"一次 UPDATE 命中哪些行"，用假仓储验证不出
// 条件写错（比如少写 status、或者把已发布的也一起改了）。
func TestRetireExaminingMigratesLegacyRows(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	// 三条：一条存量「审核中」、一条已发布（对照组）、一条「审核中」的回复。
	published := &domain.Comment{
		Content:   "正常评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, published); err != nil {
		t.Fatalf("预置已发布评论失败: %v", err)
	}
	legacy := &domain.Comment{
		Content:   "被卡在审核中的评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    legacyExaminingStatus,
	}
	if err := repo.Create(nil, legacy); err != nil {
		t.Fatalf("预置审核中评论失败: %v", err)
	}
	legacyReply := &domain.Comment{
		Content:   "被卡在审核中的回复",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		ReplyID:   published.ID,
		RootID:    published.ID,
		Status:    legacyExaminingStatus,
	}
	if err := repo.Create(nil, legacyReply); err != nil {
		t.Fatalf("预置审核中回复失败: %v", err)
	}

	retired, err := repo.RetireExamining(nil)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if retired != 2 {
		t.Fatalf("应迁移 2 条（评论 + 回复），实际 %d", retired)
	}

	if got := statusOf(t, db, legacy.ID.Uint64()); got != int8(domain.StatusDeleted) {
		t.Fatalf("存量审核中应转成已删除，实际状态 %d", got)
	}
	if got := statusOf(t, db, legacyReply.ID.Uint64()); got != int8(domain.StatusDeleted) {
		t.Fatalf("审核中的回复同样应转成已删除，实际状态 %d", got)
	}
	if got := statusOf(t, db, published.ID.Uint64()); got != int8(domain.StatusPublished) {
		t.Fatalf("已发布的评论不该被动，实际状态 %d", got)
	}

	// 迁移后读者端真的看不到它们：根评论列表只剩那条已发布评论。
	rows, _, err := repo.ListRoot(nil, 1001, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("列根评论失败: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != published.ID {
		t.Fatalf("公开列表应只剩已发布的那条，实际 %+v", rows)
	}
}

// TestRetireExaminingIsIdempotentAndDoesNotWriteWhenEmpty。
//
// 幂等是发布口径的一部分：这条迁移在**每个进程角色启动时**都会跑，重复执行必须
// 既不改数据、也不再发 UPDATE —— 后者用 updated_at 钉住（真发了 UPDATE，
// 即使 0 行受影响，对照组的 updated_at 也会被 MySQL 保留成原值，所以这里
// 再叠一条"第二次返回值必须是 0"）。
func TestRetireExaminingIsIdempotentAndDoesNotWriteWhenEmpty(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	published := &domain.Comment{
		Content:   "正常评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, published); err != nil {
		t.Fatalf("预置评论失败: %v", err)
	}
	legacy := &domain.Comment{
		Content:   "被卡在审核中的评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    legacyExaminingStatus,
	}
	if err := repo.Create(nil, legacy); err != nil {
		t.Fatalf("预置审核中评论失败: %v", err)
	}

	if retired, err := repo.RetireExamining(nil); err != nil || retired != 1 {
		t.Fatalf("第一次迁移应为 1 条，实际 %d（err=%v）", retired, err)
	}

	before := updatedAtOf(t, db, published.ID.Uint64())

	// 第二次：库里已没有「审核中」，必须 0 条且不碰任何行。
	retired, err := repo.RetireExamining(nil)
	if err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}
	if retired != 0 {
		t.Fatalf("重复迁移应命中 0 条，实际 %d", retired)
	}
	if after := updatedAtOf(t, db, published.ID.Uint64()); !after.Equal(before) {
		t.Fatalf("没有存量时不该写库，对照组的 updated_at 从 %v 变成了 %v", before, after)
	}
	if got := statusOf(t, db, published.ID.Uint64()); got != int8(domain.StatusPublished) {
		t.Fatalf("已发布的评论不该被动，实际状态 %d", got)
	}
}

// statusOf 回查一条评论的状态。
func statusOf(t *testing.T, db *gorm.DB, id uint64) int8 {
	t.Helper()
	var status int8
	if err := db.Raw("SELECT status FROM comment_models WHERE id = ?", id).Scan(&status).Error; err != nil {
		t.Fatalf("回查状态失败: %v", err)
	}
	return status
}

// updatedAtOf 回查一条评论的 updated_at（"这次迁移有没有碰这一行"的唯一外部证据）。
func updatedAtOf(t *testing.T, db *gorm.DB, id uint64) time.Time {
	t.Helper()
	var at time.Time
	if err := db.Raw("SELECT updated_at FROM comment_models WHERE id = ?", id).Scan(&at).Error; err != nil {
		t.Fatalf("回查 updated_at 失败: %v", err)
	}
	return at
}
