package domain_test

import (
	"testing"

	"myblogx/internal/filter/domain"
)

// 处置动作与记录状态的枚举口径（票 398 + 399）。
//
// 这两张表是外部契约（HTTP 请求体、风险页筛选参数、前端按钮）与内部流转的
// 唯一对照：接错一个字符串，管理员点"禁言"会变成"删内容"或者静默无效。

func TestParseAction(t *testing.T) {
	cases := []struct {
		raw       string
		want      domain.Action
		wantOK    bool
		wantAccnt bool
	}{
		{"delete", domain.ActionDelete, true, false},
		{"ignore", domain.ActionIgnore, true, false},
		{"ban", domain.ActionBan, true, true},
		{"mute", domain.ActionMute, true, true},
		{"banish", "", false, false},
		{"", "", false, false},
	}
	for _, c := range cases {
		got, ok := domain.ParseAction(c.raw)
		if ok != c.wantOK || got != c.want {
			t.Fatalf("ParseAction(%q)=(%q,%v)，期望 (%q,%v)", c.raw, got, ok, c.want, c.wantOK)
		}
		if ok && got.IsAccountAction() != c.wantAccnt {
			t.Fatalf("%q 的 IsAccountAction 应为 %v", c.raw, c.wantAccnt)
		}
	}
}

// TestStatusForAction 钉住"动作把记录带到哪个状态"。
//
// 封禁 / 禁言不删内容，所以记录状态是新增的「账号已处置」而不是"已删除"：
// 前者表示"内容还在、账号已被处理"，后者会让管理员以为内容没了。
func TestStatusForAction(t *testing.T) {
	cases := []struct {
		action domain.Action
		want   domain.Status
	}{
		{domain.ActionDelete, domain.StatusDeleted},
		{domain.ActionIgnore, domain.StatusIgnored},
		{domain.ActionBan, domain.StatusAccount},
		{domain.ActionMute, domain.StatusAccount},
	}
	for _, c := range cases {
		if got := c.action.StatusFor(); got != c.want {
			t.Fatalf("动作 %s 的终态应为 %s，实际 %s", c.action, c.want, got)
		}
	}
}

func TestParseStatusIncludesAccount(t *testing.T) {
	for _, raw := range []string{"pending", "ignored", "deleted", "account"} {
		got, ok := domain.ParseStatus(raw)
		if !ok || string(got) != raw {
			t.Fatalf("ParseStatus(%q)=(%q,%v)，期望原样通过", raw, got, ok)
		}
		if !got.Valid() {
			t.Fatalf("%q 应是合法状态", raw)
		}
	}
	if _, ok := domain.ParseStatus("banished"); ok {
		t.Fatal("不认识的状态不该通过")
	}
}
