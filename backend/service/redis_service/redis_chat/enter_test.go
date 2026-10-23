package redis_chat_test

import (
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_chat"
	"myblogx/test/testutil"
	"testing"
	"time"
)

func testRedisDeps() redis_service.Deps {
	return redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
}

// TestReserveChatMinuteRateSessionAndUserLimit 验证分钟级滑动窗口限流会同时约束会话级和用户级发送频率。
func TestReserveChatMinuteRateSessionAndUserLimit(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	now := time.Now()

	for i := 0; i < 30; i++ {
		reservation, limitedBy, err := redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:2", now.Add(time.Duration(i)*time.Millisecond))
		if err != nil {
			t.Fatalf("第 %d 次会话预占失败: %v", i+1, err)
		}
		if reservation == nil || limitedBy != "" {
			t.Fatalf("第 %d 次会话预占应成功, reservation=%v limitedBy=%s", i+1, reservation, limitedBy)
		}
	}

	reservation, limitedBy, err := redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:2", now.Add(31*time.Millisecond))
	if err != nil {
		t.Fatalf("第 31 次同会话预占失败: %v", err)
	}
	if reservation != nil || limitedBy != "session" {
		t.Fatalf("第 31 次同会话应被 session 限流, reservation=%v limitedBy=%s", reservation, limitedBy)
	}

	for i := 0; i < 30; i++ {
		reservation, limitedBy, err = redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:3", now.Add(time.Duration(100+i)*time.Millisecond))
		if err != nil {
			t.Fatalf("第 %d 次跨会话预占失败: %v", i+1, err)
		}
		if reservation == nil || limitedBy != "" {
			t.Fatalf("第 %d 次跨会话预占应成功, reservation=%v limitedBy=%s", i+1, reservation, limitedBy)
		}
	}

	reservation, limitedBy, err = redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:4", now.Add(200*time.Millisecond))
	if err != nil {
		t.Fatalf("用户级限流检测失败: %v", err)
	}
	if reservation != nil || limitedBy != "user" {
		t.Fatalf("用户级第 61 次应被 user 限流, reservation=%v limitedBy=%s", reservation, limitedBy)
	}
}

// TestReserveChatMinuteRateSlidingWindow 验证滑动 60 秒窗口会随时间推进自动释放旧额度。
func TestReserveChatMinuteRateSlidingWindow(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	now := time.Now()

	for i := 0; i < 30; i++ {
		if _, limitedBy, err := redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:2", now.Add(time.Duration(i)*time.Millisecond)); err != nil || limitedBy != "" {
			t.Fatalf("预热分钟限流失败 limitedBy=%s err=%v", limitedBy, err)
		}
	}

	if _, limitedBy, err := redis_chat.ReserveChatMinuteRate(deps, 1, "chat:1:2", now.Add(61*time.Second)); err != nil || limitedBy != "" {
		t.Fatalf("滑动窗口后应重新允许发送 limitedBy=%s err=%v", limitedBy, err)
	}
}

// TestReserveChatWeekQuotaAndReset 验证自然周配额上限以及被回复后的重置行为。
func TestReserveChatWeekQuotaAndReset(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	now := time.Now().In(time.Local)

	for i := 0; i < 3; i++ {
		reservation, allowed, err := redis_chat.ReserveChatWeekQuota(deps, 1, 2, 3, now)
		if err != nil {
			t.Fatalf("第 %d 次自然周配额预占失败: %v", i+1, err)
		}
		if reservation == nil || !allowed {
			t.Fatalf("第 %d 次自然周配额应成功, reservation=%v allowed=%v", i+1, reservation, allowed)
		}
	}

	reservation, allowed, err := redis_chat.ReserveChatWeekQuota(deps, 1, 2, 3, now)
	if err != nil {
		t.Fatalf("第 4 次自然周配额检测失败: %v", err)
	}
	if reservation != nil || allowed {
		t.Fatalf("第 4 次自然周配额应受限, reservation=%v allowed=%v", reservation, allowed)
	}

	if err := redis_chat.ResetChatWeekQuota(deps, 1, 2, now); err != nil {
		t.Fatalf("重置自然周配额失败: %v", err)
	}

	reservation, allowed, err = redis_chat.ReserveChatWeekQuota(deps, 1, 2, 3, now)
	if err != nil {
		t.Fatalf("重置后再次预占失败: %v", err)
	}
	if reservation == nil || !allowed {
		t.Fatalf("重置后应重新允许发送, reservation=%v allowed=%v", reservation, allowed)
	}
}

// TestReserveChatWeekQuotaWithStrangerLimit 验证陌生人自然周额度为 1 条。
func TestReserveChatWeekQuotaWithStrangerLimit(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	deps := testRedisDeps()
	now := time.Now().In(time.Local)

	reservation, allowed, err := redis_chat.ReserveChatWeekQuota(deps, 1, 2, 1, now)
	if err != nil {
		t.Fatalf("陌生人首条自然周配额预占失败: %v", err)
	}
	if reservation == nil || !allowed {
		t.Fatalf("陌生人首条消息应允许, reservation=%v allowed=%v", reservation, allowed)
	}

	reservation, allowed, err = redis_chat.ReserveChatWeekQuota(deps, 1, 2, 1, now)
	if err != nil {
		t.Fatalf("陌生人第二条自然周配额检测失败: %v", err)
	}
	if reservation != nil || allowed {
		t.Fatalf("陌生人每周第二条应受限, reservation=%v allowed=%v", reservation, allowed)
	}
}
