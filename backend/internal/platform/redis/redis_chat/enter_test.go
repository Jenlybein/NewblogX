package redis_chat_test

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_chat"
	"myblogx/test/testutil"
)

func testRedisOptions() redis_service.Options {
	return redis_service.Options{Client: testutil.Redis(), Logger: testutil.Logger()}
}

// 限流口径（ADR-0018 / 票 268）：同一 (sender, receiver) 10 秒内 1 条、自然日 99 条。
// 旧的 60/60s、30/60s 与周配额 1/3 条已整体替换。

func TestReserveChatPairRateBlocksBurstWithinTenSeconds(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := time.Now()
	sender, receiver := ctype.NewID(1), ctype.NewID(2)

	first, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now, 10, 99)
	if err != nil || first == nil || limitedBy != "" {
		t.Fatalf("第一条应放行: reservation=%v limitedBy=%s err=%v", first, limitedBy, err)
	}

	// 10 秒窗口内的第二条被挡，且不能污染当日额度（预占失败要能回滚）。
	second, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now.Add(3*time.Second), 10, 99)
	if err != nil {
		t.Fatalf("限流判定不应报错: %v", err)
	}
	if second != nil || limitedBy != "window" {
		t.Fatalf("10 秒窗口内第二条应被 window 限流: reservation=%v limitedBy=%s", second, limitedBy)
	}

	// 第一条失败回滚后，窗口释放，第三条可以发（额度守恒）。
	if err := first.Release(deps); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	third, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now.Add(3*time.Second), 10, 99)
	if err != nil || third == nil || limitedBy != "" {
		t.Fatalf("回滚后应可再发: reservation=%v limitedBy=%s err=%v", third, limitedBy, err)
	}
}

func TestReserveChatPairRateSlidingWindowReleasesAfterTenSeconds(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := time.Now()
	sender, receiver := ctype.NewID(1), ctype.NewID(2)

	if _, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now, 10, 99); err != nil || limitedBy != "" {
		t.Fatalf("首条预占失败 limitedBy=%s err=%v", limitedBy, err)
	}
	if _, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now.Add(11*time.Second), 10, 99); err != nil || limitedBy != "" {
		t.Fatalf("滑过 10 秒窗口后应重新允许发送 limitedBy=%s err=%v", limitedBy, err)
	}
}

func TestReserveChatPairRateDailyQuotaIsNinetyNine(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	// 固定当天 00:30 作为基准，99 次 ×11s ≈ 18 分钟，不会跨日。
	// 否则"自然日 key"会在测试中途换一天，额度被分到两个桶里。
	now := time.Date(2026, 11, 4, 0, 30, 0, 0, time.Local)
	sender, receiver := ctype.NewID(1), ctype.NewID(2)
	for i := 0; i < 99; i++ {
		res, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now.Add(time.Duration(i*11)*time.Second), 10, 99)
		if err != nil || res == nil || limitedBy != "" {
			t.Fatalf("第 %d 条应放行: limitedBy=%s err=%v", i+1, limitedBy, err)
		}
	}
	res, limitedBy, err := redis_chat.ReserveChatPairRate(deps, sender, receiver, now.Add(99*11*time.Second), 10, 99)
	if err != nil {
		t.Fatalf("第 100 条判定不应报错: %v", err)
	}
	if res != nil || limitedBy != "daily" {
		t.Fatalf("第 100 条应被 daily 限流: reservation=%v limitedBy=%s", res, limitedBy)
	}
}

func TestReserveChatPairRateIsDirectionalAndPerPair(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisOptions()
	now := time.Now()
	a, b, c := ctype.NewID(1), ctype.NewID(2), ctype.NewID(3)

	// A→B 用满窗口，不影响 B→A 与 A→C。
	if _, limitedBy, err := redis_chat.ReserveChatPairRate(deps, a, b, now, 10, 99); err != nil || limitedBy != "" {
		t.Fatalf("A→B 首条应放行 limitedBy=%s err=%v", limitedBy, err)
	}
	for _, pair := range [][2]ctype.ID{{b, a}, {a, c}} {
		res, limitedBy, err := redis_chat.ReserveChatPairRate(deps, pair[0], pair[1], now, 10, 99)
		if err != nil || res == nil || limitedBy != "" {
			t.Fatalf("方向/收件人应独立: %v→%v limitedBy=%s err=%v", pair[0], pair[1], limitedBy, err)
		}
	}
}

func TestReserveChatPairRateFailsClosedWithoutRedis(t *testing.T) {
	res, _, err := redis_chat.ReserveChatPairRate(redis_service.Options{}, ctype.NewID(1), ctype.NewID(2), time.Now(), 10, 99)
	if err == nil || res != nil {
		t.Fatalf("Redis 未初始化时应失败关闭，实际 res=%v err=%v", res, err)
	}
}
