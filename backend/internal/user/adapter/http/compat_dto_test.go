package http

import "testing"

// TestUpdateInfoRequest_陌生人私信开关映射 钉住 #243 的写入口：
// `PUT /users/info` 是唯一的"资料 + 偏好"更新接口，账号设置页要把
// stranger_msg_enabled 原样带进 SettingsPatch，而不是只读不写（#208 的死配置）。
// 全指针语义：没传 → 不改；传 false → 关掉（false 是合法值）。
func TestUpdateInfoRequest_陌生人私信开关映射(t *testing.T) {
	off, on := false, true

	cases := []struct {
		name string
		req  UpdateInfoRequest
		want *bool
	}{
		{name: "没传就不改", req: UpdateInfoRequest{}, want: nil},
		{name: "传 false 能关掉", req: UpdateInfoRequest{StrangerChatEnabled: &off}, want: &off},
		{name: "传 true 能打开", req: UpdateInfoRequest{StrangerChatEnabled: &on}, want: &on},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.req.toApp()
			got := in.Settings.StrangerChatEnabled
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("没传时不应带补丁，实际 %v", *got)
			case c.want != nil && got == nil:
				t.Fatalf("传了 %v 却没进补丁", *c.want)
			case c.want != nil && got != nil && *got != *c.want:
				t.Fatalf("补丁值 got=%v want=%v", *got, *c.want)
			}
		})
	}
}
