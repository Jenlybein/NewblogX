package app_test

import (
	"context"
	"strconv"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"myblogx/app"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/platform/rediscounter"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"
)

// 本文件守 ADR-0025 的三条并账约束，用的是**真实的任务体**
// （app.FlushArticleCounters）+ sqlite + miniredis：
//
//  1. digg / favor 按关系行 COUNT 重算覆盖，历史漂移本轮自愈；
//  2. 并账后增量照旧被清掉（sync key 被删除，业务侧不再看到那批增量）；
//  3. 读路径仍是「列 + 增量」，在没有漂移的正常批次里并账前后数字不变。
//
// 为什么不用 MySQL 容器：这三件事都在"哪个列怎么算"与"增量何时清"上，
// sqlite 与 MySQL 行为一致；MySQL 才有的差异（affected rows 口径）由
// adapter 的 integration 用例守。

// 文章计数并账的固定测试数据。
const (
	flushArticleID uint64 = 1001
)

// TestFlushArticleCountersRecomputesFromRelations 并账用关系行覆盖两个社列。
//
// 库内列故意写成 9（模拟"历史丢过增量"），Redis 里再放一批增量：
// 并账后 digg / favor 必须等于关系行条数，而不是 9 加增量。
// 同时锁住"增量照旧清空"：清不干净会让下一轮再加一遍（那是真的会累积）。
func TestFlushArticleCountersRecomputesFromRelations(t *testing.T) {
	db := newCounterTestDB(t)
	rdb := newCounterTestRedis(t)
	ctx := context.Background()

	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            flushArticleID,
		Title:         "并账文章",
		AuthorID:      100,
		ViewCount:     0,
		DiggCount:     9, // 漂移
		FavorCount:    9, // 漂移
		CommentCount:  0,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
	})
	// 真值：2 条活点赞关系（另有 1 条软删残行，不算）、1 条收藏关系。
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6001, ArticleID: flushArticleID, UserID: 301})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6002, ArticleID: flushArticleID, UserID: 302})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6003, ArticleID: flushArticleID, UserID: 303, SoftDeleted: true})
	schema.InsertUserArticleFavor(t, db, schema.UserArticleFavorSpec{
		ID: 7001, ArticleID: flushArticleID, UserID: 301, FavorID: 1, ArticleTitle: "并账文章",
	})

	addDelta(t, rdb, redis_article.ArticleCacheDigg, 3)
	addDelta(t, rdb, redis_article.ArticleCacheFavorite, 1)
	addDelta(t, rdb, redis_article.ArticleCacheView, 7)
	addDelta(t, rdb, redis_article.ArticleCacheComment, 4)

	affected, err := app.FlushArticleCounters(ctx, db, rediscounter.New(rdb, nil), nil)
	if err != nil {
		t.Fatalf("并账失败: %v", err)
	}
	if affected != 4 {
		t.Fatalf("四个指标各应处理 1 条，实际 %d", affected)
	}

	got := readCounterRow(t, db)
	if got.DiggCount != 2 {
		t.Fatalf("点赞数应按关系行重算为 2（不是 9+3），实际 %d", got.DiggCount)
	}
	if got.FavorCount != 1 {
		t.Fatalf("收藏数应按关系行重算为 1（不是 9+1），实际 %d", got.FavorCount)
	}
	// 没有关系行来源的两列保持累加口径：并账前后不变的是"口径"，不是"值不变"。
	if got.ViewCount != 7 || got.CommentCount != 4 {
		t.Fatalf("浏览 / 评论仍应按增量落库: view=%d comment=%d", got.ViewCount, got.CommentCount)
	}

	// 增量必须清空：active key 里不该再有这批增量，sync key 也必须删掉。
	for _, key := range []redis_article.ArticleCacheType{
		redis_article.ArticleCacheDigg,
		redis_article.ArticleCacheFavorite,
		redis_article.ArticleCacheView,
		redis_article.ArticleCacheComment,
	} {
		if left, err := rdb.HGetAll(ctx, string(key)).Result(); err != nil {
			t.Fatalf("读取增量失败 key=%s: %v", key, err)
		} else if len(left) != 0 {
			t.Fatalf("并账后增量应清空 key=%s，实际还有 %v", key, left)
		}
		if n, err := rdb.Exists(ctx, string(key)+":syncing").Result(); err != nil {
			t.Fatalf("检查 sync key 失败 key=%s: %v", key, err)
		} else if n != 0 {
			t.Fatalf("并账后 sync key 应被删除 key=%s:syncing", key)
		}
	}
}

// TestFlushArticleCountersKeepsReadValueStable 正常批次里并账不改变读到的数字。
//
// 场景：库内列 2、关系行 2（一致），用户又点了一次赞（关系行 3 + 增量 +1）。
// 读路径是「列 + 增量」：并账前 2+1=3，并账后 3+0=3。这就是 ADR-0025 说的
// 「用户看到的数字永不等并账」——并账只是把同一个数从增量搬到列上。
func TestFlushArticleCountersKeepsReadValueStable(t *testing.T) {
	db := newCounterTestDB(t)
	rdb := newCounterTestRedis(t)
	ctx := context.Background()

	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            flushArticleID,
		Title:         "并账文章",
		AuthorID:      100,
		DiggCount:     2,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
	})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6001, ArticleID: flushArticleID, UserID: 301})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6002, ArticleID: flushArticleID, UserID: 302})
	schema.InsertArticleDigg(t, db, schema.ArticleDiggSpec{ID: 6003, ArticleID: flushArticleID, UserID: 303})
	addDelta(t, rdb, redis_article.ArticleCacheDigg, 1)

	before := readCounterRow(t, db).DiggCount + deltaOf(t, rdb, redis_article.ArticleCacheDigg)

	if _, err := app.FlushArticleCounters(ctx, db, rediscounter.New(rdb, nil), nil); err != nil {
		t.Fatalf("并账失败: %v", err)
	}

	after := readCounterRow(t, db).DiggCount + deltaOf(t, rdb, redis_article.ArticleCacheDigg)
	if before != 3 || after != 3 {
		t.Fatalf("并账前后「列 + 增量」应守恒为 3: before=%d after=%d", before, after)
	}
}

// TestFlushArticleCountersDropsDeltaForDeletedArticle 文章已删除时丢弃增量而不是无限重试。
func TestFlushArticleCountersDropsDeltaForDeletedArticle(t *testing.T) {
	db := newCounterTestDB(t)
	rdb := newCounterTestRedis(t)
	ctx := context.Background()

	// 只有 Redis 增量，没有文章行：并账必须成功返回（drop 掉这条增量）。
	addDelta(t, rdb, redis_article.ArticleCacheDigg, 1)

	affected, err := app.FlushArticleCounters(ctx, db, rediscounter.New(rdb, nil), nil)
	if err != nil {
		t.Fatalf("文章不存在不应让并账失败（否则会回补增量、无限重试）: %v", err)
	}
	if affected != 1 {
		t.Fatalf("这条增量应被处理（丢弃）一次，实际 %d", affected)
	}
	if n, err := rdb.HLen(ctx, string(redis_article.ArticleCacheDigg)).Result(); err != nil || n != 0 {
		t.Fatalf("增量应被丢弃而不是回补: len=%d err=%v", n, err)
	}
}

// ── 辅助 ──

// newCounterTestDB 建并账用到的三张表。
func newCounterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testkit.NewSQLite(t, &schema.ArticleRow{}, &schema.ArticleDiggRow{}, &schema.UserArticleFavorRow{})
}

// newCounterTestRedis 起一个 miniredis 并返回连上它的客户端。
func newCounterTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// addDelta 往某个指标的 hash 里累加一篇文章的增量。
func addDelta(t *testing.T, rdb *redis.Client, key redis_article.ArticleCacheType, delta int) {
	t.Helper()
	field := strconv.FormatUint(flushArticleID, 10)
	if err := rdb.HIncrBy(context.Background(), string(key), field, int64(delta)).Err(); err != nil {
		t.Fatalf("写入增量失败 key=%s: %v", key, err)
	}
}

// deltaOf 读取某指标当前的增量（读路径的第二个加数）。
func deltaOf(t *testing.T, rdb *redis.Client, key redis_article.ArticleCacheType) int {
	t.Helper()
	got, err := rdb.HGet(context.Background(), string(key), strconv.FormatUint(flushArticleID, 10)).Int()
	if err != nil && err != redis.Nil {
		t.Fatalf("读取增量失败 key=%s: %v", key, err)
	}
	return got
}

// readCounterRow 回读文章行的四个计数列（读路径的库内快照）。
func readCounterRow(t *testing.T, db *gorm.DB) schema.ArticleRow {
	t.Helper()
	var row schema.ArticleRow
	if err := db.First(&row, "id = ?", flushArticleID).Error; err != nil {
		t.Fatalf("回查文章失败: %v", err)
	}
	return row
}
