package redis_ai_quota_test

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_ai_quota"
	"myblogx/test/testutil"
)

// AI 每日额度的平台层验证（ADR-0038）：按 场景 + 自然日 + 用户 分桶的计数键，
// 以及预占 / 释放。
// 只测额度守恒与跨日换桶，不断言 Redis 键长什么样（与投稿配额的测试同一口径）。

func testRedisOptions() redis_service.Options {
	return redis_service.Options{Client: testutil.Redis(), Logger: testutil.Logger()}
}

// 当天基准：取 00:30，测试内部不跨日，避免"自然日 key"在用例中途换桶。
func dayStart() time.Time { return time.Date(2026, 11, 10, 0, 30, 0, 0, time.Local) }

func TestReserveAIDailyQuotaStopsAtLimit(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	for i := 0; i < 5; i++ {
		res, allowed, err := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 5)
		if err != nil || !allowed || res == nil {
			t.Fatalf("第 %d 次应放行: allowed=%v reservation=%v err=%v", i+1, allowed, res, err)
		}
	}

	// 第 6 次超额：既不能给出预占，也不该报错（"额度用完"是正常结论，不是故障）。
	res, allowed, err := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 5)
	if err != nil {
		t.Fatalf("超额判定不应报错: %v", err)
	}
	if res != nil || allowed {
		t.Fatalf("第 6 次应被拒: allowed=%v reservation=%v", allowed, res)
	}
}

func TestReserveAIDailyQuotaIsScopedPerScene(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	// metainfo 用完不影响 overwrite：额度按场景分设。
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 1); !allowed {
		t.Fatal("metainfo 首次应放行")
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 1); allowed {
		t.Fatal("metainfo 第 2 次应被拒")
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "overwrite", user, now, 1); !allowed {
		t.Fatal("overwrite 是另一个桶，首次应放行")
	}

	// 换成另一个用户也是独立桶。
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", ctype.NewID(8), now, 1); !allowed {
		t.Fatal("另一个用户的 metainfo 首次应放行")
	}
}

func TestReserveAIDailyQuotaRollsOverAtMidnight(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	user := ctype.NewID(7)

	today := dayStart()
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "diagnose", user, today, 1); !allowed {
		t.Fatal("今天首次应放行")
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "diagnose", user, today, 1); allowed {
		t.Fatal("今天第 2 次应被拒")
	}

	// 跨自然日换桶：昨天用满不影响今天。
	tomorrow := today.AddDate(0, 0, 1)
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "diagnose", user, tomorrow, 1); !allowed {
		t.Fatal("跨日后应重新有额度")
	}
}

func TestReserveAIDailyQuotaReleaseRestoresOne(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	res, allowed, err := redis_ai_quota.ReserveAIDailyQuota(deps, "scoring", user, now, 1)
	if err != nil || !allowed || res == nil {
		t.Fatalf("首次应放行: allowed=%v err=%v", allowed, err)
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "scoring", user, now, 1); allowed {
		t.Fatal("用满后应被拒")
	}

	// 失败归还：额度回到 1，下一次又能放行。
	if err := res.Release(deps); err != nil {
		t.Fatalf("归还失败: %v", err)
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "scoring", user, now, 1); !allowed {
		t.Fatal("归还后应能再放行一次")
	}

	// 幂等：重复归还不会把额度放大（否则等于凭空多出次数）。
	if err := res.Release(deps); err != nil {
		t.Fatalf("重复归还应当安全: %v", err)
	}
	if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "scoring", user, now, 1); allowed {
		t.Fatal("重复归还不该放大额度")
	}
}

func TestReserveAIDailyQuotaUnlimitedDoesNotWrite(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := dayStart()
	user := ctype.NewID(7)

	// limit <= 0（不限）：直接放行、不写键、也不需要归还。
	res, allowed, err := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 0)
	if err != nil || !allowed || res != nil {
		t.Fatalf("不限场景应直接放行且不预占: res=%v allowed=%v err=%v", res, allowed, err)
	}
	for i := 0; i < 10; i++ {
		if _, allowed, _ := redis_ai_quota.ReserveAIDailyQuota(deps, "metainfo", user, now, 0); !allowed {
			t.Fatal("不限场景不该被拒")
		}
	}
}
