package redis_comment_test

import (
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/test/testutil"
	"testing"
)

func testRedisDeps() redis_service.Deps {
	return redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
}

func TestReplyCacheCounters(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()

	if err := redis_comment.SetCacheReply(deps, 1, 3); err != nil {
		t.Fatalf("SetCacheReply 失败: %v", err)
	}
	if err := redis_comment.SetCacheReply(deps, 1, -1); err != nil {
		t.Fatalf("SetCacheReply 累加失败: %v", err)
	}
	if err := redis_comment.SetCacheReply(deps, 2, 5); err != nil {
		t.Fatalf("SetCacheReply 写入第二个评论失败: %v", err)
	}

	if redis_comment.GetCacheReply(deps, 1) != 2 {
		t.Fatalf("reply 计数错误: %d", redis_comment.GetCacheReply(deps, 1))
	}

	batch := redis_comment.GetBatchCacheReply(deps, []ctype.ID{1, 2, 3})
	if batch[1] != 2 || batch[2] != 5 {
		t.Fatalf("批量读取结果异常: %+v", batch)
	}

	all := redis_comment.GetAllCacheReply(deps)
	if len(all) != 2 {
		t.Fatalf("GetAllCacheReply 长度异常: %+v", all)
	}

	if err := redis_comment.ClearAllCacheReply(deps); err != nil {
		t.Fatalf("ClearAllCacheReply 失败: %v", err)
	}
	if redis_comment.GetCacheReply(deps, 1) != 0 || redis_comment.GetCacheReply(deps, 2) != 0 {
		t.Fatal("清理后计数应为0")
	}
}

func TestDiggCacheCounters(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()

	if err := redis_comment.SetCacheDigg(deps, 1, 2); err != nil {
		t.Fatalf("SetCacheDigg 失败: %v", err)
	}
	if err := redis_comment.SetCacheDigg(deps, 1, -1); err != nil {
		t.Fatalf("SetCacheDigg 累加失败: %v", err)
	}
	if err := redis_comment.SetCacheDigg(deps, 2, 4); err != nil {
		t.Fatalf("SetCacheDigg 写入第二个评论失败: %v", err)
	}

	if redis_comment.GetCacheDigg(deps, 1) != 1 {
		t.Fatalf("digg 计数错误: %d", redis_comment.GetCacheDigg(deps, 1))
	}

	batch := redis_comment.GetBatchCacheDigg(deps, []ctype.ID{1, 2, 3})
	if batch[1] != 1 || batch[2] != 4 {
		t.Fatalf("digg 批量读取异常: %+v", batch)
	}

	all := redis_comment.GetAllCacheDigg(deps)
	if len(all) != 2 {
		t.Fatalf("GetAllCacheDigg 长度异常: %+v", all)
	}

	if err := redis_comment.DelCacheDigg(deps, 2); err != nil {
		t.Fatalf("DelCacheDigg 失败: %v", err)
	}
	if redis_comment.GetCacheDigg(deps, 2) != 0 {
		t.Fatalf("DelCacheDigg 后应为0: %d", redis_comment.GetCacheDigg(deps, 2))
	}

	if err := redis_comment.ClearAllCacheDigg(deps); err != nil {
		t.Fatalf("ClearAllCacheDigg 失败: %v", err)
	}
	if redis_comment.GetCacheDigg(deps, 1) != 0 {
		t.Fatalf("清理后 digg 计数应为0: %d", redis_comment.GetCacheDigg(deps, 1))
	}
}
