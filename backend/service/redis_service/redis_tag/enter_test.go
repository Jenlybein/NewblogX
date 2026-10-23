package redis_tag_test

import (
	"testing"

	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_tag"
	"myblogx/test/testutil"
)

func TestTagArticleCountCache(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}

	if err := redis_tag.SetCacheArticleCount(deps, 1, 3); err != nil {
		t.Fatalf("SetCacheArticleCount 失败: %v", err)
	}
	if err := redis_tag.SetCacheArticleCount(deps, 1, -1); err != nil {
		t.Fatalf("SetCacheArticleCount 累加失败: %v", err)
	}
	if err := redis_tag.SetCacheArticleCount(deps, 2, 5); err != nil {
		t.Fatalf("SetCacheArticleCount 第二个标签失败: %v", err)
	}

	if redis_tag.GetCacheArticleCount(deps, 1) != 2 {
		t.Fatalf("tag 1 计数错误: %d", redis_tag.GetCacheArticleCount(deps, 1))
	}
	if redis_tag.GetCacheArticleCount(deps, 2) != 5 {
		t.Fatalf("tag 2 计数错误: %d", redis_tag.GetCacheArticleCount(deps, 2))
	}

	batch := redis_tag.GetBatchCacheArticleCount(deps, []ctype.ID{1, 2, 3})
	if batch[1] != 2 || batch[2] != 5 || batch[3] != 0 {
		t.Fatalf("批量读取异常: %+v", batch)
	}

	all := redis_tag.GetAllCacheArticleCount(deps)
	if len(all) != 2 {
		t.Fatalf("全量读取异常: %+v", all)
	}

	if err := redis_tag.ClearAllCacheTag(deps); err != nil {
		t.Fatalf("ClearAllCacheTag 失败: %v", err)
	}
	if redis_tag.GetCacheArticleCount(deps, 1) != 0 || redis_tag.GetCacheArticleCount(deps, 2) != 0 {
		t.Fatal("清理后计数应为 0")
	}
}
