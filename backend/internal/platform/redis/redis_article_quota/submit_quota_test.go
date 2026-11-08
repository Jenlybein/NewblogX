package redis_article_quota_test

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article_quota"
	"myblogx/test/testutil"
)

// 投稿配额（票 304 / ADR-0031）的平台层验证：按自然日分桶的计数键 + 预占 / 释放。
// 只测额度守恒，不断言 Redis 键长什么样（spec 的 Testing Decisions）。

func testRedisOptions() redis_service.Options {
	return redis_service.Options{Client: testutil.Redis(), Logger: testutil.Logger()}
}

// 当天基准：取 00:30，测试内部不跨日，避免"自然日 key"在用例中途换桶。
func dayStart() time.Time { return time.Date(2026, 11, 8, 0, 30, 0, 0, time.Local) }

func TestReserveSubmitQuotaStopsAtDailyMax(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	for i := 0; i < 15; i++ {
		res, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15)
		if err != nil || res == nil || !allowed {
			t.Fatalf("第 %d 次提交应放行: allowed=%v reservation=%v err=%v", i+1, allowed, res, err)
		}
	}

	// 第 16 次超额：既不能给出预占，也不该报错（"额度用完"是正常结论，不是故障）。
	res, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15)
	if err != nil {
		t.Fatalf("超额判定不应报错: %v", err)
	}
	if res != nil || allowed {
		t.Fatalf("第 16 次应被拒: allowed=%v reservation=%v", allowed, res)
	}
}

func TestReserveSubmitQuotaConservesQuotaAcrossRelease(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	var reserved []*redis_article_quota.Reservation
	for i := 0; i < 15; i++ {
		res, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15)
		if err != nil || !allowed || res == nil {
			t.Fatalf("第 %d 次提交应放行: allowed=%v err=%v", i+1, allowed, err)
		}
		reserved = append(reserved, res)
	}
	if _, allowed, _ := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15); allowed {
		t.Fatal("用满之后不应还能预占")
	}

	// 一次失败提交归还额度后，应当又能预占一次（额度守恒）。
	if err := reserved[0].Release(deps); err != nil {
		t.Fatalf("释放失败: %v", err)
	}
	res, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15)
	if err != nil || !allowed || res == nil {
		t.Fatalf("归还一次后应可再预占: allowed=%v err=%v", allowed, err)
	}

	// 再还一次也不能把额度"还多"：仍然只有一次可用。
	if err := res.Release(deps); err != nil {
		t.Fatalf("第二次释放失败: %v", err)
	}
	if err := res.Release(deps); err != nil {
		t.Fatalf("重复释放同一预占不应报错: %v", err)
	}
	if _, allowed, _ := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15); !allowed {
		t.Fatal("重复释放后应仍可预占一次")
	}
	if _, allowed, _ := redis_article_quota.ReserveSubmitQuota(deps, user, now, 15); allowed {
		t.Fatal("额度不应被重复释放放大")
	}
}

func TestReserveSubmitQuotaIsPerUserAndPerNaturalDay(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	day := dayStart()
	a, b := ctype.NewID(7), ctype.NewID(8)

	if _, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, a, day, 1); err != nil || !allowed {
		t.Fatalf("A 首次应放行: allowed=%v err=%v", allowed, err)
	}
	if _, allowed, _ := redis_article_quota.ReserveSubmitQuota(deps, a, day, 1); allowed {
		t.Fatal("A 用满后不应再放行")
	}
	// 额度按用户分桶：A 用满不影响 B。
	if _, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, b, day, 1); err != nil || !allowed {
		t.Fatalf("B 不应受 A 的额度影响: allowed=%v err=%v", allowed, err)
	}
	// 额度按自然日分桶：第二天重新计数。
	if _, allowed, err := redis_article_quota.ReserveSubmitQuota(deps, a, day.Add(24*time.Hour), 1); err != nil || !allowed {
		t.Fatalf("第二天应重新获得额度: allowed=%v err=%v", allowed, err)
	}
}

func TestReserveSubmitQuotaFailsClosedWithoutRedis(t *testing.T) {
	res, allowed, err := redis_article_quota.ReserveSubmitQuota(redis_service.Options{}, ctype.NewID(7), dayStart(), 15)
	if err == nil || res != nil || allowed {
		t.Fatalf("Redis 未初始化时应失败关闭: allowed=%v res=%v err=%v", allowed, res, err)
	}
}
