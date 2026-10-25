package domain_test

import (
	"testing"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

func TestBuildSessionID(t *testing.T) {
	if got := domain.BuildSessionID(2, 10); got != "chat:2:10" {
		t.Fatalf("got %s", got)
	}
	if domain.BuildSessionID(10, 2) != domain.BuildSessionID(2, 10) {
		t.Fatal("会话 ID 必须与双方顺序无关")
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
	_ = ctype.ID(0)
}
