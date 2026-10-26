package redis_article_test

import (
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/test/testutil"
	"testing"
)

func testRedisOptions() redis_service.Options {
	return redis_service.Options{Client: testutil.Redis(), Logger: testutil.Logger()}
}

func TestArticleCacheCounters(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()

	if err := redis_article.SetCacheView(deps, ctype.NewID(1), 3); err != nil {
		t.Fatalf("SetCacheView 失败: %v", err)
	}
	if err := redis_article.SetCacheDigg(deps, ctype.NewID(1), 2); err != nil {
		t.Fatalf("SetCacheDigg 失败: %v", err)
	}
	if err := redis_article.SetCacheFavorite(deps, ctype.NewID(1), 1); err != nil {
		t.Fatalf("SetCacheFavorite 失败: %v", err)
	}
	if err := redis_article.SetCacheComment(deps, ctype.NewID(1), 4); err != nil {
		t.Fatalf("SetCacheComment 失败: %v", err)
	}

	if redis_article.GetCacheView(deps, ctype.NewID(1)) != 3 {
		t.Fatalf("view 计数错误: %d", redis_article.GetCacheView(deps, ctype.NewID(1)))
	}
	if redis_article.GetCacheDigg(deps, ctype.NewID(1)) != 2 {
		t.Fatalf("digg 计数错误: %d", redis_article.GetCacheDigg(deps, ctype.NewID(1)))
	}
	if redis_article.GetCacheFavorite(deps, ctype.NewID(1)) != 1 {
		t.Fatalf("favorite 计数错误: %d", redis_article.GetCacheFavorite(deps, ctype.NewID(1)))
	}
	if redis_article.GetCacheComment(deps, ctype.NewID(1)) != 4 {
		t.Fatalf("comment 计数错误: %d", redis_article.GetCacheComment(deps, ctype.NewID(1)))
	}

	if err := redis_article.ClearAllCacheArticle(deps); err != nil {
		t.Fatalf("ClearAllCacheArticle 失败: %v", err)
	}
	if redis_article.GetCacheView(deps, ctype.NewID(1)) != 0 ||
		redis_article.GetCacheDigg(deps, ctype.NewID(1)) != 0 ||
		redis_article.GetCacheFavorite(deps, ctype.NewID(1)) != 0 ||
		redis_article.GetCacheComment(deps, ctype.NewID(1)) != 0 {
		t.Fatal("清理后计数应为 0")
	}
}

func TestGetBatchCounters(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()

	if err := redis_article.SetCacheView(deps, ctype.NewID(1), 3); err != nil {
		t.Fatalf("SetCacheView 失败: %v", err)
	}
	if err := redis_article.SetCacheView(deps, ctype.NewID(2), 5); err != nil {
		t.Fatalf("SetCacheView 失败: %v", err)
	}
	if err := redis_article.SetCacheDigg(deps, ctype.NewID(1), 2); err != nil {
		t.Fatalf("SetCacheDigg 失败: %v", err)
	}
	if err := redis_article.SetCacheFavorite(deps, ctype.NewID(2), 4); err != nil {
		t.Fatalf("SetCacheFavorite 失败: %v", err)
	}
	if err := redis_article.SetCacheComment(deps, ctype.NewID(3), 6); err != nil {
		t.Fatalf("SetCacheComment 失败: %v", err)
	}

	counters := redis_article.GetBatchCounters(deps, []ctype.ID{ctype.NewID(1), ctype.NewID(2), ctype.NewID(3)})
	if counters.ViewMap[ctype.NewID(1)] != 3 || counters.ViewMap[ctype.NewID(2)] != 5 {
		t.Fatalf("view 批量计数异常: %+v", counters.ViewMap)
	}
	if counters.DiggMap[ctype.NewID(1)] != 2 {
		t.Fatalf("digg 批量计数异常: %+v", counters.DiggMap)
	}
	if counters.FavorMap[ctype.NewID(2)] != 4 {
		t.Fatalf("favorite 批量计数异常: %+v", counters.FavorMap)
	}
	if counters.CommentMap[ctype.NewID(3)] != 6 {
		t.Fatalf("comment 批量计数异常: %+v", counters.CommentMap)
	}
}

func TestArticleHistoryCache(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()

	redis_article.SetUserArticleHistoryCache(deps, 10, 20)
	if !redis_article.GetUserArticleHistoryCache(deps, 10, 20) {
		t.Fatal("用户历史记录应存在")
	}

	redis_article.SetGuestArticleHistoryCache(deps, 11, "abc")
	if !redis_article.GetGuestArticleHistoryCache(deps, 11, "abc") {
		t.Fatal("访客历史记录应存在")
	}
}
