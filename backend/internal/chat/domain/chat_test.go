package domain_test

import (
	"testing"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

func TestBuildSessionID(t *testing.T) {
	if got := domain.BuildSessionID(ctype.NewID(2), ctype.NewID(10)); got != "chat:2:10" {
		t.Fatalf("got %s", got)
	}
	if domain.BuildSessionID(ctype.NewID(10), ctype.NewID(2)) != domain.BuildSessionID(ctype.NewID(2), ctype.NewID(10)) {
		t.Fatal("会话 ID 必须与双方顺序无关")
	}
}

// TestMessageIDFromClientKey 幂等键到消息主键的映射必须：
//   - 同一（发送人，键）两次得到同一个 ID（重复提交才可能撞主键而不是产生第二条）；
//   - 不同键 / 不同发送人得到不同 ID；
//   - 落在保留区间（不与雪花 ID 相交），否则幂等消息可能撞上普通消息的主键。
func TestMessageIDFromClientKey(t *testing.T) {
	sender := ctype.NewID(100)
	first := domain.MessageIDFromClientKey(sender, "client-key-1")
	again := domain.MessageIDFromClientKey(sender, "client-key-1")
	if first != again {
		t.Fatalf("同一幂等键应得到同一消息 ID，got %s / %s", first, again)
	}
	if first.IsZero() {
		t.Fatal("幂等消息 ID 不能为零")
	}
	if got := domain.MessageIDFromClientKey(ctype.NewID(200), "client-key-1"); got == first {
		t.Fatal("不同发送人使用同一幂等键不应共享消息 ID")
	}
	if got := domain.MessageIDFromClientKey(sender, "client-key-2"); got == first {
		t.Fatal("不同幂等键应得到不同消息 ID")
	}
	if first.Uint64() < domain.ClientKeyIDBase {
		t.Fatalf("幂等消息 ID 必须落在保留区间 [%d, 2^63)，实际 %s", domain.ClientKeyIDBase, first)
	}
}

func TestSummary(t *testing.T) {
	if got := domain.Summary(domain.TypeImage, "x"); got != "[图片]" {
		t.Fatalf("图片摘要错误: %s", got)
	}
	long := "这是一个非常非常非常非常非常长的消息内容需要被截断处理"
	got := domain.Summary(domain.TypeText, long)
	if len([]rune(got)) > 23 {
		t.Fatalf("摘要未截断: %s", got)
	}
	_ = ctype.NewID(0)
}
