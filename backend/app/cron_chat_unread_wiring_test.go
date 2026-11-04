package app_test

import (
	"os"
	"strings"
	"testing"
)

// 两个并账必须挂在**同一个 30 分钟 tick** 上（spec 269 / D-2，票 266）。
//
// 私信没有 Redis 增量源，所以它的覆盖面是全部活跃会话；如果哪天有人把它
// 拆成第二个 sch.Every，ADR-0025「不新增定时任务」的约束就破了，而页面
// 上看起来毫无差别 —— 这类回归只能靠源码门禁抓。
func TestChatUnreadReconcileSharesTheCounterTick(t *testing.T) {
	source, err := os.ReadFile("cron_wiring.go")
	if err != nil {
		t.Fatalf("读 cron_wiring.go 失败: %v", err)
	}
	text := string(source)

	if !strings.Contains(text, "FlushArticleCounters(ctx, db, flusher, infra.Logger)") {
		t.Fatal("文章计数并账必须仍在同一个任务体里")
	}
	if !strings.Contains(text, "ReconcileChatUnread(ctx, db)") {
		t.Fatal("私信未读重算必须挂在同一个任务体里")
	}

	// sch.Every 的次数没有增加：仍是那一处 counterSyncInterval 的任务。
	if got := strings.Count(text, "sch.Every(counterSyncInterval"); got < 1 {
		t.Fatalf("应该保留 30 分钟并账任务，实际 %d", got)
	}
}
