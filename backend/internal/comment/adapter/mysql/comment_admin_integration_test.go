//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"myblogx/internal/comment/adapter/mysql"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 管理端评论列表的真 MySQL 契约。
//
// 为什么必须真库：这一层要证明的是"删除只写 deleted_at、不改 status"这个
// 库里事实，以及管理端列表据此怎么区分已发布 / 已删除。SQLite 上没有 gorm 的
// 软删列语义差异，测不出"已删除筛选会不会永远是空"。

// seedAdminComments 预置三条评论：一条活着、一条软删、一条存量的 status=已删除
// （存量「审核中」退场时只改状态、没写 deleted_at，见 RetireExamining）。
func seedAdminComments(t *testing.T, repo *mysql.Repo) (live, softDeleted, legacy domain.Comment) {
	t.Helper()

	live = domain.Comment{
		Content:      "还在的评论",
		UserID:       ctype.NewID(200),
		UserNickname: "读者",
		ArticleID:    ctype.NewID(1001),
		Status:       domain.StatusPublished,
	}
	if err := repo.Create(nil, &live); err != nil {
		t.Fatalf("预置正常评论失败: %v", err)
	}

	softDeleted = domain.Comment{
		Content:      "软删的评论",
		UserID:       ctype.NewID(200),
		UserNickname: "读者",
		ArticleID:    ctype.NewID(1001),
		Status:       domain.StatusPublished,
	}
	if err := repo.Create(nil, &softDeleted); err != nil {
		t.Fatalf("预置待删评论失败: %v", err)
	}

	legacy = domain.Comment{
		Content:      "存量已删除的评论",
		UserID:       ctype.NewID(200),
		UserNickname: "读者",
		ArticleID:    ctype.NewID(1001),
		Status:       domain.StatusDeleted,
	}
	if err := repo.Create(nil, &legacy); err != nil {
		t.Fatalf("预置存量已删除评论失败: %v", err)
	}

	// 正常删除路径：只写软删标记。
	if err := repo.SoftDeleteMany(context.Background(), []domain.CommentID{softDeleted.ID}); err != nil {
		t.Fatalf("软删评论失败: %v", err)
	}
	return live, softDeleted, legacy
}

// TestSoftDeleteKeepsStatusColumn 钉住"删除只写 deleted_at"这条存储事实。
//
// 这条用例本身不是管理端功能，而是管理端列表口径的前提：如果哪天删除动作
// 改成同时写 status=3，这里会红，而管理端的状态筛选口径要跟着重新核对。
func TestSoftDeleteKeepsStatusColumn(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	_, softDeleted, _ := seedAdminComments(t, repo)

	var row struct {
		Status    int8
		DeletedAt sql.NullTime
	}
	if err := db.Raw("SELECT status, deleted_at FROM comment_models WHERE id = ?",
		softDeleted.ID.Uint64()).Scan(&row).Error; err != nil {
		t.Fatalf("回查软删行失败: %v", err)
	}
	if row.Status != int8(domain.StatusPublished) {
		t.Fatalf("删除路径不该改 status（管理端列表口径依赖这一点），实际 status=%d", row.Status)
	}
	if !row.DeletedAt.Valid {
		t.Fatal("软删必须写 deleted_at，否则这条评论会重新出现在读者侧")
	}
}

// TestListAdminIncludesSoftDeleted 管理端列表要能看到软删的行，并按状态正确分类。
func TestListAdminIncludesSoftDeleted(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	live, softDeleted, legacy := seedAdminComments(t, repo)

	ctx := context.Background()

	// 全部：三条都在（管理端默认视图区别于读者侧）。
	all, hasMore, err := repo.ListAdmin(ctx, domain.AdminFilter{Status: domain.AdminStatusAll}, domain.AdminPage{Limit: 10})
	if err != nil {
		t.Fatalf("管理端列表失败: %v", err)
	}
	if hasMore {
		t.Fatalf("只有 3 条却报 has_more，分页判据不对")
	}
	if len(all) != 3 {
		t.Fatalf("全部视图应看到 3 条（含软删行），实际 %d 条", len(all))
	}
	byID := map[uint64]domain.Comment{}
	for _, row := range all {
		byID[row.ID.Uint64()] = row
	}
	if got, ok := byID[softDeleted.ID.Uint64()]; !ok {
		t.Fatal("软删的评论没有出现在管理端列表里（Unscoped 漏了？）")
	} else if !got.IsDeleted() {
		t.Fatalf("软删行应被识别为已删除: %+v", got)
	}
	if !byID[legacy.ID.Uint64()].IsDeleted() {
		t.Fatal("存量 status=已删除的行应被识别为已删除")
	}
	if byID[live.ID.Uint64()].IsDeleted() {
		t.Fatal("正常评论不该被识别为已删除")
	}

	// 已发布：只应剩正常那条（软删行虽然 status 仍是 2，也不能出现）。
	published, _, err := repo.ListAdmin(ctx, domain.AdminFilter{Status: domain.AdminStatusPublished}, domain.AdminPage{Limit: 10})
	if err != nil {
		t.Fatalf("已发布筛选失败: %v", err)
	}
	if len(published) != 1 || published[0].ID != live.ID {
		t.Fatalf("已发布视图应只剩正常评论 %s，实际 %+v", live.ID, published)
	}

	// 已删除：软删行 + 存量已删除行。
	deleted, _, err := repo.ListAdmin(ctx, domain.AdminFilter{Status: domain.AdminStatusDeleted}, domain.AdminPage{Limit: 10})
	if err != nil {
		t.Fatalf("已删除筛选失败: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("已删除视图应有软删行与存量已删除行共 2 条，实际 %d 条", len(deleted))
	}
	for _, row := range deleted {
		if !row.IsDeleted() {
			t.Fatalf("已删除视图里混进了活评论: %+v", row)
		}
	}

	// 内容关键词：只命中正常那条。
	hits, _, err := repo.ListAdmin(ctx, domain.AdminFilter{Keyword: "还在"}, domain.AdminPage{Limit: 10})
	if err != nil {
		t.Fatalf("关键词筛选失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != live.ID {
		t.Fatalf("关键词筛选结果不对: %+v", hits)
	}

	// 分页 has_more：limit=1 时后面还有。
	page, more, err := repo.ListAdmin(ctx, domain.AdminFilter{Status: domain.AdminStatusAll}, domain.AdminPage{Limit: 1})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if len(page) != 1 || !more {
		t.Fatalf("limit=1 应只回 1 条且 has_more=true，实际 %d 条 has_more=%v", len(page), more)
	}

	// 其余筛选维度（用户 / 昵称 / 文章 / 时间）在真库上也要真的生效。
	byUser, _, err := repo.ListAdmin(ctx, domain.AdminFilter{UserID: ctype.NewID(200)}, domain.AdminPage{Limit: 10})
	if err != nil || len(byUser) != 3 {
		t.Fatalf("按用户筛选应命中 3 条，实际 %d 条 err=%v", len(byUser), err)
	}
	byUserKeyword, _, err := repo.ListAdmin(ctx, domain.AdminFilter{UserKeyword: "读"}, domain.AdminPage{Limit: 10})
	if err != nil || len(byUserKeyword) != 3 {
		t.Fatalf("按昵称快照筛选应命中 3 条，实际 %d 条 err=%v", len(byUserKeyword), err)
	}
	byArticle, _, err := repo.ListAdmin(ctx, domain.AdminFilter{ArticleID: ctype.NewID(1001)}, domain.AdminPage{Limit: 10})
	if err != nil || len(byArticle) != 3 {
		t.Fatalf("按文章筛选应命中 3 条，实际 %d 条 err=%v", len(byArticle), err)
	}

	// 时间范围：右边界是"不含"，所以把下界放到 1 小时后应当一条都查不到。
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	withinWindow, _, err := repo.ListAdmin(ctx, domain.AdminFilter{FromAt: &past, ToAt: &future}, domain.AdminPage{Limit: 10})
	if err != nil || len(withinWindow) != 3 {
		t.Fatalf("时间窗内应命中 3 条，实际 %d 条 err=%v", len(withinWindow), err)
	}
	afterWindow, _, err := repo.ListAdmin(ctx, domain.AdminFilter{FromAt: &future}, domain.AdminPage{Limit: 10})
	if err != nil || len(afterWindow) != 0 {
		t.Fatalf("起点在未来应一条都查不到，实际 %d 条 err=%v", len(afterWindow), err)
	}
}

// TestListAdminSortsNewestFirst 管理端列表最新在前（运营找最近的问题评论）。
func TestListAdminSortsNewestFirst(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	first := domain.Comment{
		Content:   "先发的",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, &first); err != nil {
		t.Fatalf("预置第一条失败: %v", err)
	}
	second := domain.Comment{
		Content:   "后发的",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, &second); err != nil {
		t.Fatalf("预置第二条失败: %v", err)
	}

	rows, _, err := repo.ListAdmin(context.Background(), domain.AdminFilter{Status: domain.AdminStatusAll}, domain.AdminPage{Limit: 10})
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有 2 条，实际 %d 条", len(rows))
	}
	if rows[0].ID != second.ID || rows[1].ID != first.ID {
		t.Fatalf("应按 created_at 倒序（后发的在前），实际 %s / %s", rows[0].ID, rows[1].ID)
	}
}
