package chat

import (
	"context"
	"errors"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	sitecfg "myblogx/internal/platform/config"
)

// 本文件钉住 #243 的**两级**语义：
//
//	有效判决 = 站点总闸(site.chat.stranger_msg_enabled)
//	           AND 接收方个人开关(user_conf_models.stranger_chat_enabled，默认开)
//
// 两级取值放在模块装配内（strangerPolicy），因为站点配置是运行时单例、
// 接收方偏好属于 user 模块的读端口；chat 用例层只消费合成后的结论。

// fakeStrangerUserSettings 是 user 模块读端口的替身，记录被问的是哪个接收方。
type fakeStrangerUserSettings struct {
	enabled bool
	err     error
	seen    []ctype.ID
}

func (f *fakeStrangerUserSettings) StrangerChatEnabled(_ context.Context, id ctype.ID) (bool, error) {
	f.seen = append(f.seen, id)
	return f.enabled, f.err
}

// newRuntimeSite 构造只带站点段默认值的生效值模块（缓存为空时读默认值）。
func newRuntimeSite(strangerMsgEnabled bool) *sitecfg.EffectiveConfig {
	return sitecfg.NewEffectiveConfig(sitecfg.NewRuntimeConfigService(conf.RuntimeDefaults{
		Site: conf.Site{Chat: siteconf.Chat{StrangerMsgEnabled: strangerMsgEnabled}},
	}, nil, nil))
}

func TestStrangerPolicy_两级开关取交集(t *testing.T) {
	cases := []struct {
		name   string
		siteOn bool
		userOn bool
		want   chatdomain.StrangerVerdict
	}{
		{name: "站点开×接收方开", siteOn: true, userOn: true, want: chatdomain.StrangerAllowed},
		{name: "站点开×接收方关", siteOn: true, userOn: false, want: chatdomain.StrangerBlockedByRecipient},
		{name: "站点关×接收方开", siteOn: false, userOn: true, want: chatdomain.StrangerBlockedBySite},
		{name: "站点关×接收方关", siteOn: false, userOn: false, want: chatdomain.StrangerBlockedBySite},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			users := &fakeStrangerUserSettings{enabled: c.userOn}
			bridge := &strangerPolicy{inner: newRuntimeSite(c.siteOn), users: users}

			got, err := bridge.CheckStranger(context.Background(), ctype.NewID(200))
			if err != nil {
				t.Fatalf("判定失败: %v", err)
			}
			if got != c.want {
				t.Fatalf("有效判决 got=%v want=%v", got, c.want)
			}
			// 站点总闸开着时，判定必须问到**接收方**（不是发送方、不是笼统的站点）。
			if c.siteOn && (len(users.seen) != 1 || users.seen[0] != ctype.NewID(200)) {
				t.Fatalf("应查询接收方 200 的个人开关，实际 %v", users.seen)
			}
		})
	}
}

// TestStrangerPolicy_站点关闭时不再读接收方 站点总闸关闭已经足以拒绝，
// 不必为每个陌生人消息再查一次用户偏好。
func TestStrangerPolicy_站点关闭时不再读接收方(t *testing.T) {
	users := &fakeStrangerUserSettings{enabled: true, err: errors.New("不该被读到")}
	bridge := &strangerPolicy{inner: newRuntimeSite(false), users: users}

	got, err := bridge.CheckStranger(context.Background(), ctype.NewID(200))
	if err != nil {
		t.Fatalf("站点关闭时应直接拒绝而不是报错: %v", err)
	}
	if got != chatdomain.StrangerBlockedBySite {
		t.Fatalf("站点关闭时判决应指向站点级，实际 %v", got)
	}
	if len(users.seen) != 0 {
		t.Fatalf("站点关闭时不应读接收方偏好，实际 %v", users.seen)
	}
}

// TestStrangerPolicy_接收方偏好读取错误上抛 不能把"读不到"吞成"允许"。
func TestStrangerPolicy_接收方偏好读取错误上抛(t *testing.T) {
	boom := errors.New("读接收方偏好失败")
	bridge := &strangerPolicy{
		inner: newRuntimeSite(true),
		users: &fakeStrangerUserSettings{enabled: true, err: boom},
	}

	_, err := bridge.CheckStranger(context.Background(), ctype.NewID(200))
	if !errors.Is(err, boom) {
		t.Fatalf("应原样上抛接收方偏好读取错误，实际 %v", err)
	}
}

// TestStrangerPolicy_依赖缺失时失败关闭 站点配置或用户读端口没装配时，
// 宁可拒绝也不能静默放行（否则等于个人开关被绕过）。
func TestStrangerPolicy_依赖缺失时失败关闭(t *testing.T) {
	users := &fakeStrangerUserSettings{enabled: true}

	cases := []struct {
		name   string
		bridge *strangerPolicy
	}{
		{name: "站点配置未装配", bridge: &strangerPolicy{users: users}},
		{name: "用户读端口未装配", bridge: &strangerPolicy{inner: newRuntimeSite(true)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.bridge.CheckStranger(context.Background(), ctype.NewID(200))
			if err != nil {
				t.Fatalf("依赖缺失不该以错误暴露给发送方: %v", err)
			}
			// 失败关闭的文案归属站点级：这是部署/配置问题，
			// 不该把用户引向"对方关了开关"这条查不到的结论。
			if got != chatdomain.StrangerBlockedBySite {
				t.Fatalf("依赖缺失时应失败关闭到站点级，实际 %v", got)
			}
		})
	}
}
