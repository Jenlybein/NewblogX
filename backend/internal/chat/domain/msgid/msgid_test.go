package msgid_test

import (
	"testing"

	"myblogx/internal/chat/domain/msgid"
	"myblogx/internal/kernel/ctype"
)

// 派生主键必须满足两条外部性质（号段本身是实现细节，不在这里断言）：
//
//  1. 同一（发送人，幂等键）两次派生出同一个主键 —— 重复提交才会撞主键而不是
//     产生第二条消息（零 DDL 幂等的前提）；
//  2. 换发送人或换幂等键就换主键 —— 否则两个人用同一个客户端键会互相吞消息。
func TestFromClientKeyIsDeterministic(t *testing.T) {
	sender := ctype.NewID(100)

	first := msgid.FromClientKey(sender, "client-key-1")
	again := msgid.FromClientKey(sender, "client-key-1")
	if first != again {
		t.Fatalf("同一幂等键应得到同一消息主键，got %s / %s", first, again)
	}
	if first.IsZero() {
		t.Fatal("派生主键不能为零值")
	}

	if got := msgid.FromClientKey(ctype.NewID(200), "client-key-1"); got == first {
		t.Fatal("不同发送人使用同一幂等键不应共享主键")
	}
	if got := msgid.FromClientKey(sender, "client-key-2"); got == first {
		t.Fatal("不同幂等键应得到不同主键")
	}
}
