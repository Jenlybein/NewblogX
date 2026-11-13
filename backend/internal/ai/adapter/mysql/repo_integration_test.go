//go:build integration

package mysql

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是评分仓储**只在 MySQL 上才成立**的那一半语义。
//
// SQLite 版（repo_test.go）跑的是同一份 Upsert 代码，但 lockArticle 在非 MySQL
// 方言上直接返回，所以"锁文章行"这条路径在 SQLite 上根本不执行 —— 这正是
// **「闸门开启时新文章发不出去」**漏过所有单测的原因。
//
// 这里补上两件事：
//  1. 文章行还不存在时（质量闸门的正常路径）评分照样落库；
//  2. 文章行存在时并发写仍然只有一行（锁没有被顺手删掉）。

// 测试用的固定作者 ID：article_models.author_id 有外键指向 user_models，
// 所以要先播一个用户行，否则建文章行会撞 FK。
const testAuthorID uint64 = 7

// newMySQLScoreRepo 建出评分用例需要的两张表：article_models（锁的目标）
// 与 article_ai_score_record_models（评分行），并播一个作者用户行。
func newMySQLScoreRepo(t *testing.T) *Repo {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore)
	// 评分表没有独立的 schema 组（只在 All 里出现过），单独建它。
	if err := db.AutoMigrate(&schema.ArticleAIScoreRecordRow{}); err != nil {
		t.Fatalf("建评分表失败: %v", err)
	}
	seedAuthor(t, db)
	return New(db)
}

// seedAuthor 播一个最小可用的用户行（外键目标）。
func seedAuthor(t *testing.T, db *gorm.DB) {
	t.Helper()
	user := schema.UserRow{
		BaseRow:  schema.BaseRow{ID: ctype.NewID(testAuthorID)},
		Username: "score-author",
		Nickname: "评分作者",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("播种用户行失败: %v", err)
	}
}

// createArticle 播一篇文章行（已入库那一侧的场景）。
func createArticle(t *testing.T, db *gorm.DB, articleID uint64, title string) {
	t.Helper()
	if err := db.Create(&schema.ArticleRow{
		BaseRow:       schema.BaseRow{ID: ctype.NewID(articleID)},
		Title:         title,
		AuthorID:      ctype.NewID(testAuthorID),
		PublishStatus: 3,
	}).Error; err != nil {
		t.Fatalf("播种文章行失败: %v", err)
	}
}

// TestScoreRepo_UpsertAcceptsArticleNotYetCreated 钉住闸门路径：
// 新建文章的评分记录先于文章行落库（ADR-0023 决策 5 要求评分在提交事务之外）。
//
// 回归的是"闸门开着就发不出新文章"：lockArticle 查不到文章行时返回
// ai.article_missing，整次提交被打成"拿不到可用分数"，于是任何新建即提交
// 都失败关闭。
func TestScoreRepo_UpsertAcceptsArticleNotYetCreated(t *testing.T) {
	repo := newMySQLScoreRepo(t)
	ctx := context.Background()

	const articleID uint64 = 9100
	record := scoreRecord(articleID, 78)
	if err := repo.Upsert(ctx, record); err != nil {
		t.Fatalf("文章行还没入库时评分应能落库，实际: %v", err)
	}
	if record.ID.IsZero() {
		t.Fatal("写入后应回填主键")
	}
	if rows := countScoreRows(t, repo, articleID); rows != 1 {
		t.Fatalf("评分行数 = %d，期望 1", rows)
	}

	// 文章随后落库（真实顺序是本次请求的写事务），评分行读得回来。
	createArticle(t, repo.db, articleID, "闸门先评分后入库")
	latest, err := repo.Latest(ctx, articleID)
	if err != nil {
		t.Fatalf("读最新评分失败: %v", err)
	}
	if latest == nil || latest.TotalScore != 78 {
		t.Fatalf("评分行应归到这篇文章，实际: %+v", latest)
	}
}

// TestScoreRepo_UpsertKeepsSingleRowOnMySQL 钉住"文章行存在时的那把锁"：
// 并发评分仍然只有一行。SQLite 版用单连接、没有行锁，只能证明逻辑顺序，
// 证明不了 MySQL 上的排他语义。
func TestScoreRepo_UpsertKeepsSingleRowOnMySQL(t *testing.T) {
	repo := newMySQLScoreRepo(t)
	ctx := context.Background()

	const articleID uint64 = 9200
	createArticle(t, repo.db, articleID, "并发评分")

	const writers = 6
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(score int) {
			defer wg.Done()
			if err := repo.Upsert(ctx, scoreRecord(articleID, score)); err != nil {
				errCh <- err
			}
		}(60 + i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发评分写入失败: %v", err)
	}

	if rows := countScoreRows(t, repo, articleID); rows != 1 {
		t.Fatalf("并发评分后评分行数 = %d，期望 1", rows)
	}
	latest, err := repo.Latest(ctx, articleID)
	if err != nil {
		t.Fatalf("读最新评分失败: %v", err)
	}
	if latest == nil {
		t.Fatal("并发写完后应能读到评分记录")
	}
	if latest.TotalScore < 60 || latest.TotalScore >= 60+writers {
		t.Fatalf("读到的分数 %d 不是任何一次写入的结果", latest.TotalScore)
	}
	if latest.CreatedAt.IsZero() || time.Since(latest.CreatedAt) > time.Minute {
		t.Fatalf("覆盖写应把 created_at 推到本次评分时间，实际: %v", latest.CreatedAt)
	}
}
