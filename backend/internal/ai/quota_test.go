package ai

import (
	"context"
	"errors"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 每日额度的收费点（ADR-0028）。
//
// 只断言**对外可观察的结论**：该扣的时候扣了几次、该跳过的场景是否一次都不扣、
// 额度用完与额度服务故障是不是两种错误码。不断言 Redis 键名或内部字段。
//
// 为什么直接测 chargeCall 而不是 Chat：Chat 在额度放行之后会真的发模型请求，
// 单测里只能验证"被拒绝"那一侧；chargeCall 是 Chat 与 ChatStream 共用的唯一收费
// 函数，把它测透等价于覆盖两个出口的计费分支。

type fakeLease struct{ released int }

func (l *fakeLease) Release(_ context.Context) error {
	l.released++
	return nil
}

type fakeGate struct {
	calls   int
	scene   string
	limit   int
	userID  ctype.ID
	allowed bool
	err     error
	lease   *fakeLease
}

func (g *fakeGate) Reserve(_ context.Context, scene string, userID ctype.ID, limit int) (QuotaLease, bool, error) {
	g.calls++
	g.scene = scene
	g.limit = limit
	g.userID = userID
	if g.err != nil {
		return nil, false, g.err
	}
	if !g.allowed {
		return nil, false, nil
	}
	g.lease = &fakeLease{}
	return g.lease, true, nil
}

func limitsAll(n int) conf.DailyLimits {
	return conf.DailyLimits{
		Scoring: n, Metainfo: n, Overwrite: n, Diagnose: n,
		SearchRewrite: n, SearchAnswer: n, Moderation: n,
	}
}

func installGate(t *testing.T, gate *fakeGate) {
	t.Helper()
	SetQuotaGate(gate)
	t.Cleanup(func() { SetQuotaGate(nil) })
}

func metaCtx(meta CallMeta) context.Context {
	return WithCallMeta(context.Background(), meta)
}

func TestSceneDailyLimitMapsEveryScene(t *testing.T) {
	limits := conf.DailyLimits{
		Scoring: 1, Metainfo: 2, Overwrite: 3, Diagnose: 4,
		SearchRewrite: 5, SearchAnswer: 6, Moderation: 7,
	}
	cases := []struct {
		scene string
		want  int
	}{
		{SceneScoring, 1},
		{SceneMetainfo, 2},
		{SceneOverwrite, 3},
		{SceneDiagnose, 4},
		{SceneSearchRewrite, 5},
		{SceneSearchAnswer, 6},
		{SceneModeration, 7},
		// 没打标的调用不猜额度：与日志里"scene 为空显示 -"的口径一致。
		{"", 0},
		{"unknown_scene", 0},
	}
	for _, tc := range cases {
		if got := SceneDailyLimit(limits, tc.scene); got != tc.want {
			t.Errorf("SceneDailyLimit(%q)=%d，期望 %d", tc.scene, got, tc.want)
		}
	}
}

func TestChargeCallSkipsAdminAnonymousPrechargedAndUnmetered(t *testing.T) {
	gate := &fakeGate{allowed: true}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: limitsAll(20)}

	cases := []struct {
		name string
		meta CallMeta
		why  string
	}{
		{"管理员", CallMeta{Scene: SceneScoring, UserID: ctype.NewID(7), IsAdmin: true}, "固定豁免"},
		{"无归属", CallMeta{Scene: SceneScoring}, "异步链路没有用户"},
		{"已预扣", CallMeta{Scene: SceneScoring, UserID: ctype.NewID(7), Billing: BillingPrecharged}, "动作入口已扣"},
		{"只记日志", CallMeta{Scene: SceneScoring, UserID: ctype.NewID(7), Billing: BillingUnmetered}, "补评分不计费"},
	}
	for _, tc := range cases {
		lease, err := chargeCall(metaCtx(tc.meta), aiConf)
		if err != nil || lease != nil {
			t.Fatalf("%s（%s）不该占用额度: lease=%v err=%v", tc.name, tc.why, lease, err)
		}
	}
	if gate.calls != 0 {
		t.Fatalf("上述四类调用都不该访问计数端口，实际 calls=%d", gate.calls)
	}
}

func TestChargeCallSkipsUnlimitedScene(t *testing.T) {
	gate := &fakeGate{allowed: true}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: limitsAll(0)}
	lease, err := chargeCall(metaCtx(CallMeta{Scene: SceneMetainfo, UserID: ctype.NewID(7)}), aiConf)
	if err != nil || lease != nil {
		t.Fatalf("额度为 0（不限）时不该预占: lease=%v err=%v", lease, err)
	}
	if gate.calls != 0 {
		t.Fatalf("额度为 0 时不该访问计数端口: calls=%d", gate.calls)
	}
}

func TestChargeCallReservesOnceWithSceneLimit(t *testing.T) {
	gate := &fakeGate{allowed: true}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: conf.DailyLimits{Metainfo: 50}}
	lease, err := chargeCall(metaCtx(CallMeta{Scene: SceneMetainfo, UserID: ctype.NewID(7)}), aiConf)
	if err != nil {
		t.Fatalf("应放行: %v", err)
	}
	if gate.calls != 1 || gate.scene != SceneMetainfo || gate.limit != 50 {
		t.Fatalf("预占参数异常: calls=%d scene=%q limit=%d", gate.calls, gate.scene, gate.limit)
	}
	if lease == nil {
		t.Fatal("应拿到归还凭据")
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("归还失败: %v", err)
	}
}

func TestChargeCallExceededUsesDedicatedCode(t *testing.T) {
	gate := &fakeGate{allowed: false}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: limitsAll(20)}
	_, err := chargeCall(metaCtx(CallMeta{Scene: SceneSearchAnswer, UserID: ctype.NewID(7)}), aiConf)
	if !apperr.IsCode(err, apperr.CodeAIDailyLimitExceeded) {
		t.Fatalf("额度用完应返回 ai.daily_limit_exceeded，实际 %v", err)
	}
}

func TestChargeCallGateFailureFailsClosed(t *testing.T) {
	gate := &fakeGate{err: errors.New("redis down")}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: limitsAll(20)}
	_, err := chargeCall(metaCtx(CallMeta{Scene: SceneOverwrite, UserID: ctype.NewID(7)}), aiConf)
	if !apperr.IsCode(err, apperr.CodeAIQuotaUnavailable) {
		t.Fatalf("计数服务故障应失败关闭并返回 ai.quota_unavailable，实际 %v", err)
	}
}

func TestBuyActionQuotaSkipsForAdminAndAnonymous(t *testing.T) {
	gate := &fakeGate{allowed: true}
	installGate(t, gate)

	aiConf := conf.AI{DailyLimits: limitsAll(20)}
	if _, err := BuyActionQuota(context.Background(), aiConf, SceneScoring, ctype.NewID(7), true); err != nil {
		t.Fatalf("管理员不该被拒绝: %v", err)
	}
	if _, err := BuyActionQuota(context.Background(), aiConf, SceneScoring, ctype.NewID(0), false); err != nil {
		t.Fatalf("无归属调用不该被拒绝: %v", err)
	}
	if gate.calls != 0 {
		t.Fatalf("管理员与无归属调用都不该占用额度: calls=%d", gate.calls)
	}
}

// 计费档位默认是 BillingNormal：没有任何补丁时，出站调用应当真的去预占。
func TestZeroBillingMeansNormal(t *testing.T) {
	var meta CallMeta
	if meta.Billing != BillingNormal {
		t.Fatalf("零值档位应是 BillingNormal，实际 %v", meta.Billing)
	}

	gate := &fakeGate{allowed: true}
	installGate(t, gate)
	aiConf := conf.AI{DailyLimits: limitsAll(20)}
	lease, err := chargeCall(metaCtx(CallMeta{Scene: SceneDiagnose, UserID: ctype.NewID(9)}), aiConf)
	if err != nil || lease == nil {
		t.Fatalf("默认档位应预占: lease=%v err=%v", lease, err)
	}
	if gate.calls != 1 {
		t.Fatalf("默认档位应预占一次，实际 calls=%d", gate.calls)
	}
}
