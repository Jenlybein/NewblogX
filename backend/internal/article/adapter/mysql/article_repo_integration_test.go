//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"myblogx/internal/testkit/schema"
	"testing"
	"time"

	"strconv"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/testkit/mysqlcontainer"
)

// migrate 按业务依赖顺序建表（表结构来源见 internal/testkit/schema）。
//
// 顺序不能随意：MySQL 下 AutoMigrate 会建外键，被引用的表必须先存在。
func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.UserSession, schema.ArticleCore,
		schema.Favorite, schema.UserReading, schema.ArticleReview,
		schema.Comment, schema.Outbox,
	)
	return db
}

// seedUsers 插入本组测试用到的作者行（100 与 200）。
func seedUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedUser(t, db, 100)
	seedUser(t, db, 200)
}

// seedUser 插入作者行。
//
// AutoMigrate 会建出 article_models.author_id -> user_models.id 的外键，
// 不先插用户就没有作者可引用。真库里也一样。
func seedUser(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{
		ID:       id,
		Username: "user" + strconv.FormatUint(id, 10),
		Nickname: "用户" + strconv.FormatUint(id, 10),
	})
}

func newArticle(t *testing.T, id domain.ArticleID, author domain.UserID, status domain.Status) *domain.Article {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := domain.NewArticle(domain.NewArticleParams{
		ID:         id,
		AuthorID:   author,
		Title:      "契约测试文章",
		Abstract:   "摘要",
		Content:    "正文",
		Status:     status,
		Visibility: domain.VisibilityVisible,
		Now:        now,
	})
	if err != nil {
		t.Fatalf("构造文章失败: %v", err)
	}
	return a
}

func TestArticleRepoCRUD(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewArticleRepo(db)
	ctx := context.Background()

	a := newArticle(t, ctype.NewID(1001), ctype.NewID(100), domain.StatusDraft)
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, err := repo.Find(ctx, ctype.NewID(1001))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Title() != "契约测试文章" || got.Status() != domain.StatusDraft {
		t.Fatalf("字段不符: title=%q status=%s", got.Title(), got.Status())
	}
	if got.AuthorID().Uint64() != 100 {
		t.Fatalf("作者应为 100，实际 %s", got.AuthorID())
	}

	// 改状态与标题
	title := "改过的标题"
	if _, err := got.ApplyEdit(domain.EditPatch{Title: &title}, time.Now()); err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if err := got.Submit(time.Now(), true); err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	reloaded, err := repo.Find(ctx, ctype.NewID(1001))
	if err != nil {
		t.Fatalf("再读失败: %v", err)
	}
	if reloaded.Title() != "改过的标题" {
		t.Fatalf("标题未更新，实际 %q", reloaded.Title())
	}
	if reloaded.Status() != domain.StatusPublished {
		t.Fatalf("状态应为已发布，实际 %s", reloaded.Status())
	}

	// 软删
	if err := repo.SoftDelete(ctx, ctype.NewID(1001)); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	if _, err := repo.Find(ctx, ctype.NewID(1001)); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("软删后应查不到，实际 %v", err)
	}

	// 硬查确认行还在（软删不是物理删除）
	var n int64
	if err := db.Model(&schema.ArticleRow{}).Unscoped().Where("id = ?", 1001).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("软删应保留行，实际行数 %d", n)
	}
}

// TestArticleRepoUniqueKey 验证真库的唯一键行为（sqlite 上测不准）。
func TestArticleRepoUniqueKey(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewArticleRepo(db)
	ctx := context.Background()

	a := newArticle(t, ctype.NewID(2001), ctype.NewID(100), domain.StatusDraft)
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	if err := repo.Create(ctx, newArticle(t, ctype.NewID(2001), ctype.NewID(100), domain.StatusDraft)); err == nil {
		t.Fatal("重复主键应报错")
	}
}

func TestArticleRepoListOnlyPublic(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewArticleRepo(db)
	ctx := context.Background()

	for _, c := range []struct {
		id     domain.ArticleID
		status domain.Status
	}{
		{ctype.NewID(3001), domain.StatusPublished},
		{ctype.NewID(3002), domain.StatusDraft},
		{ctype.NewID(3003), domain.StatusExamining},
	} {
		if err := repo.Create(ctx, newArticle(t, c.id, ctype.NewID(100), c.status)); err != nil {
			t.Fatalf("创建 %s 失败: %v", c.id, err)
		}
	}

	got, total, err := repo.List(ctx, domain.Filter{OnlyPublic: true}, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID().Uint64() != 3001 {
		t.Fatalf("OnlyPublic 应只返回已发布，实际 total=%d", total)
	}

	// 不带 OnlyPublic 时应看到全部 3 篇
	_, total, err = repo.List(ctx, domain.Filter{}, domain.Page{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("应返回 3 篇，实际 %d", total)
	}
}

func TestTagRepoReplaceAndLoad(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	tags := mysql.NewTagRepo(db)
	articles := mysql.NewArticleRepo(db)
	ctx := context.Background()

	if err := articles.Create(ctx, newArticle(t, ctype.NewID(4001), ctype.NewID(100), domain.StatusDraft)); err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	// 循环变量用数值，ID 在循环体内构造：ctype.ID 是 string，不能直接 ++。
	for i := uint64(1); i <= 2; i++ {
		id := ctype.NewID(i)
		tag := taxonomy.RestoreTag(taxonomy.RestoreTagParams{ID: id, Title: "标签" + strconv.FormatUint(i, 10), Enabled: true})
		if err := tags.Save(ctx, tag); err != nil {
			t.Fatalf("保存标签失败: %v", err)
		}
	}

	if err := tags.ReplaceArticleTags(ctx, ctype.NewID(4001), []domain.TagID{ctype.NewID(1), ctype.NewID(2)}); err != nil {
		t.Fatalf("替换标签失败: %v", err)
	}
	got, err := tags.ArticleTagIDs(ctx, ctype.NewID(4001))
	if err != nil {
		t.Fatalf("读取标签失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应有 2 个标签，实际 %v", got)
	}

	// 换成空集合应清空
	if err := tags.ReplaceArticleTags(ctx, ctype.NewID(4001), nil); err != nil {
		t.Fatalf("清空标签失败: %v", err)
	}
	got, _ = tags.ArticleTagIDs(ctx, ctype.NewID(4001))
	if len(got) != 0 {
		t.Fatalf("标签应被清空，实际 %v", got)
	}
}

func TestFavoriteDefaultFolderCreatedOnce(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewFavoriteRepo(db)
	ctx := context.Background()

	f1, err := repo.DefaultFolder(ctx, ctype.NewID(100))
	if err != nil {
		t.Fatalf("创建默认夹失败: %v", err)
	}
	f2, err := repo.DefaultFolder(ctx, ctype.NewID(100))
	if err != nil {
		t.Fatalf("二次取默认夹失败: %v", err)
	}
	if f1.ID() != f2.ID() {
		t.Fatalf("应复用同一默认夹，实际 %s vs %s", f1.ID(), f2.ID())
	}
	if !f1.IsDefault() {
		t.Fatal("应为默认收藏夹")
	}
}

func TestFavoriteItemUnique(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewFavoriteRepo(db)
	ctx := context.Background()

	folder, err := repo.DefaultFolder(ctx, ctype.NewID(100))
	if err != nil {
		t.Fatalf("创建默认夹失败: %v", err)
	}

	// 先落文章行：user_article_favor_models.article_id 有外键指向 article_models。
	target := newArticle(t, ctype.NewID(5001), ctype.NewID(200), domain.StatusPublished)
	if err := mysql.NewArticleRepo(db).Create(ctx, target); err != nil {
		t.Fatalf("创建被收藏文章失败: %v", err)
	}

	item := favorite.NewItemFromArticle(folder.ID(), ctype.NewID(100), target)
	if err := repo.SaveItem(ctx, item); err != nil {
		t.Fatalf("收藏失败: %v", err)
	}
	if err := repo.SaveItem(ctx, item); err == nil {
		t.Fatal("同一文章重复收藏应触发唯一键冲突")
	}

	n, err := repo.CountByArticle(ctx, ctype.NewID(5001))
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应有 1 条收藏，实际 %d", n)
	}
}

func TestReviewRepoLifecycle(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	repo := mysql.NewReviewRepo(db)
	ctx := context.Background()

	task := review.NewTask(review.NewTaskParams{
		ID:            ctype.NewID(9001),
		ArticleID:     ctype.NewID(6001),
		AuthorID:      ctype.NewID(100),
		ArticleTitle:  "待审",
		AuthorName:    "作者",
		PublishStatus: domain.StatusExamining,
		Source:        review.SourceCreate,
		Now:           time.Now(),
	})
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("保存任务失败: %v", err)
	}

	pending, err := repo.ListPendingByArticle(ctx, ctype.NewID(6001))
	if err != nil {
		t.Fatalf("查待审失败: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("应有 1 条待审，实际 %d", len(pending))
	}

	if err := task.Approve(ctype.NewID(900), time.Now()); err != nil {
		t.Fatalf("审核通过失败: %v", err)
	}
	if err := repo.Update(ctx, task); err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}

	got, err := repo.Find(ctx, 9001)
	if err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if got.Status() != review.StatusApproved {
		t.Fatalf("状态应为 approved，实际 %s", got.Status())
	}
	if got.ReviewedBy() == nil || (*got.ReviewedBy()).Uint64() != 900 {
		t.Fatal("应记录审核人")
	}

	pending, _ = repo.ListPendingByArticle(ctx, ctype.NewID(6001))
	if len(pending) != 0 {
		t.Fatalf("处理后不应再有待审，实际 %d", len(pending))
	}
}

// TestUoWCommitAndRollback 用真库验证事务语义：
// 回调返回 nil 必须提交，返回错误必须整体回滚。
func TestUoWCommitAndRollback(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUsers(t, db)
	uow := mysql.NewTxManager(db, redis_service.Options{}, nil /* 作者统计不在本测试断言范围 */)
	repo := mysql.NewArticleRepo(db)
	ctx := context.Background()

	// 提交
	if err := uow.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
		return r.Articles().Create(ctx, newArticle(t, ctype.NewID(7001), ctype.NewID(100), domain.StatusDraft))
	}); err != nil {
		t.Fatalf("提交路径应成功: %v", err)
	}
	if _, err := repo.Find(ctx, ctype.NewID(7001)); err != nil {
		t.Fatalf("提交后应能读到: %v", err)
	}

	// 回滚
	boom := errors.New("故意失败")
	err := uow.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
		if err := r.Articles().Create(ctx, newArticle(t, ctype.NewID(7002), ctype.NewID(100), domain.StatusDraft)); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回原始错误，实际 %v", err)
	}
	if _, err := repo.Find(ctx, ctype.NewID(7002)); !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("回滚后 7002 不应存在，实际 %v", err)
	}
}
