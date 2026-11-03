package app

import (
	"testing"

	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/platform/http/ws"
)

// 本文件锁死"私信通知跳过"的判据：进程是否投递私信通知必须是 relayOptions 的
// **显式输入**，不能再从装配顺序或包级连接池反推。
//
// 回归形态：有人把 SkipEventNames 重新绑回 "WireChat 有没有跑过"，于是 api
// 进程（先 WireHTTP 后 WireOutboxRelay 之外的任何顺序）静默跳过
// chat.message.sent，事件被标 done、通知永久丢。这里对两个角色分别断言，
// 顺序无论怎么变都必须过。

func hasSkippedEvent(opts []string, want string) bool {
	for _, name := range opts {
		if name == want {
			return true
		}
	}
	return false
}

func TestRelayOptionsSkipChatEventsOnlyWithoutPushHub(t *testing.T) {
	t.Parallel()

	// API 角色：持有 WS 连接池，绝不能跳过私信事件。
	api := relayOptions(&Infra{}, OutboxWiringOptions{DeliverChatPush: true})
	for _, event := range []string{chatdomain.EventMessageSent, chatdomain.EventMessagesRead} {
		if hasSkippedEvent(api.SkipEventNames, event) {
			t.Fatalf("api 角色不能跳过 %s，实际 SkipEventNames=%v", event, api.SkipEventNames)
		}
	}

	// river 角色：没有连接池，必须跳过两条私信事件。
	river := relayOptions(&Infra{}, OutboxWiringOptions{DeliverChatPush: false})
	for _, event := range []string{chatdomain.EventMessageSent, chatdomain.EventMessagesRead} {
		if !hasSkippedEvent(river.SkipEventNames, event) {
			t.Fatalf("river 角色必须跳过 %s，实际 SkipEventNames=%v", event, river.SkipEventNames)
		}
	}
	if len(river.SkipEventNames) != 2 {
		t.Fatalf("跳过清单应恰好是两条私信事件，实际 %v", river.SkipEventNames)
	}
}

// TestRelayOptionsDoesNotDependOnChatHubState 证明判据与 Infra.ChatHub 无关。
//
// 装配顺序问题之所以能发生，就是因为旧实现读的是"连接池有没有被 WireChat 挂上"。
// 这里显式把 ChatHub 设成非 nil 再要求 river 角色跳过：若实现又改回读连接池，
// 这条会立刻红。
func TestRelayOptionsDoesNotDependOnChatHubState(t *testing.T) {
	t.Parallel()

	infra := &Infra{ChatHub: ws.NewHub()}
	opts := relayOptions(infra, OutboxWiringOptions{DeliverChatPush: false})
	if !hasSkippedEvent(opts.SkipEventNames, chatdomain.EventMessageSent) {
		t.Fatalf("即使 ChatHub 非 nil，river 角色也必须跳过私信事件，实际 %v", opts.SkipEventNames)
	}
}
