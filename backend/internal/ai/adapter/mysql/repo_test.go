package mysql

import (
	"context"
	"sync"
	"testing"

	"myblogx/internal/ai/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// newScoreRepo 建一张只有评分表的库，返回待测仓储。
func newScoreRepo(t *testing.T) *Repo {
	t.Helper()
	return New(testutil.SetupSQLite(t, &schema.ArticleAIScoreRecordRow{}))
}

// scoreRecord 造一条待写入的评分记录。
func scoreRecord(articleID uint64, total int) *domain.ScoreRecord {
	return &domain.ScoreRecord{
		ArticleID:      ctype.NewID(articleID),
		UserID:         ctype.NewID(7),
		TitleSnapshot:  "标题",
		ContentHash:    "hash",
		ContentLength:  120,
		AITotalScore:   total,
		TotalScore:     total,
		ScoreLevel:     "B",
		ArticleType:    "教程",
		DimensionsJSON: "[]",
		MainIssuesJSON: "[]",
		OverallComment: "评语",
		Provider:       "llm",
		ModelName:      "test-model",
		PromptVersion:  "article-scoring-v1",
	}
}

// countScoreRows 数库里该文章的评分行（含软删，用来钉住"永远只有一行"）。
func countScoreRows(t *testing.T, repo *Repo, articleID uint64) int64 {
	t.Helper()
	var count int64
	if err := repo.db.Unscoped().Model(&scoreRow{}).
		Where("article_id = ?", articleID).
		Count(&count).Error; err != nil {
		t.Fatalf("统计评分行失败: %v", err)
	}
	return count
}

// TestScoreRepo_UpsertOverwritesStoredScore 钉住 #305 的核心行为：
// 同一篇文章再次评分时覆盖既有行，而不是"已有记录就跳过"。
func TestScoreRepo_UpsertOverwritesStoredScore(t *testing.T) {
	repo := newScoreRepo(t)
	ctx := context.Background()

	first := scoreRecord(100, 41)
	if err := repo.Upsert(ctx, first); err != nil {
		t.Fatalf("首次评分落库失败: %v", err)
	}
	if first.ID.IsZero() {
		t.Fatal("首次写入后应回填主键")
	}

	second := scoreRecord(100, 88)
	second.OverallComment = "改稿后明显更好"
	if err := repo.Upsert(ctx, second); err != nil {
		t.Fatalf("再次评分落库失败: %v", err)
	}

	latest, err := repo.Latest(ctx, 100)
	if err != nil {
		t.Fatalf("读最新评分失败: %v", err)
	}
	if latest == nil {
		t.Fatal("覆盖写后应能读到评分记录")
	}
	if latest.TotalScore != 88 || latest.AITotalScore != 88 {
		t.Fatalf("读到的分数 = %+v，期望覆盖成 88", latest)
	}
	if latest.OverallComment != "改稿后明显更好" {
		t.Fatalf("读到的评语 = %q，期望是最近一次的结果", latest.OverallComment)
	}
	if latest.ID != first.ID {
		t.Fatalf("覆盖写应保留原行主键 %v，实际 %v", first.ID, latest.ID)
	}
	if rows := countScoreRows(t, repo, 100); rows != 1 {
		t.Fatalf("同一篇文章评分行数 = %d，期望 1", rows)
	}
}

// TestScoreRepo_UpsertKeepsSingleRowUnderConcurrency 钉住"并发写也只有一行"。
//
// 表上没有 article_id 的唯一索引（零 DDL 加不了），这条不变式只能靠仓储自己守：
// 写之前先锁住文章行，判断与写入在同一个事务里，谁也插不进来。
// 测试库是 SQLite，写入由它自己串行化；这里断言的是仓储在交错调用下不产生第二行，
// 也不留下"改了一半"的记录。
func TestScoreRepo_UpsertKeepsSingleRowUnderConcurrency(t *testing.T) {
	repo := newScoreRepo(t)
	const writers = 8

	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(score int) {
			defer wg.Done()
			if err := repo.Upsert(context.Background(), scoreRecord(200, score)); err != nil {
				errCh <- err
			}
		}(40 + i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发评分写入失败: %v", err)
	}

	if rows := countScoreRows(t, repo, 200); rows != 1 {
		t.Fatalf("并发评分后评分行数 = %d，期望 1", rows)
	}
	latest, err := repo.Latest(context.Background(), 200)
	if err != nil {
		t.Fatalf("读最新评分失败: %v", err)
	}
	if latest == nil {
		t.Fatal("并发写完后应能读到评分记录")
	}
	if latest.TotalScore < 40 || latest.TotalScore >= 40+writers {
		t.Fatalf("读到的分数 %d 不是任何一次写入的结果", latest.TotalScore)
	}
}
