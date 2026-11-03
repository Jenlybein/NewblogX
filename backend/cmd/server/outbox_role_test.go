package main

import "testing"

// TestOutboxWiringOptionsFollowRole 锁死角色 → 中继策略的装配映射。
//
// api / all 调用 WireHTTP（含 WireChat），持有私信 WS 连接池；river 没有。
// 这个映射必须显式在装配处给，而不是由中继回读"连接池有没有被挂上"——
// 后者会把行为绑到 WireChat 与 WireOutboxRelay 的调用顺序上。
func TestOutboxWiringOptionsFollowRole(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"api", "all"} {
		if !outboxWiringOptionsForRole(role).DeliverChatPush {
			t.Fatalf("%s 角色持有 WS 连接池，必须投递私信通知", role)
		}
	}
	if outboxWiringOptionsForRole("river").DeliverChatPush {
		t.Fatal("river 角色没有 WS 连接池，必须跳过私信通知")
	}
}
