package health_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/platform/health"
)

// 装配层在只跑 HTTP 的角色里没有后台组件，注册表可能是 nil。
// 健康检查端点不该因此 panic —— 那时候"没有组件"就是正确的答案。
func TestNilRegistryIsSafe(t *testing.T) {
	var registry *health.Registry

	registry.Set("cdc_river", health.StateFailed, errors.New("不该走到这里"))

	if got := registry.Snapshot(); got != nil {
		t.Fatalf("nil 注册表的快照应为 nil，实际 %v", got)
	}
	if !registry.Healthy() {
		t.Fatal("nil 注册表应视为健康（没有组件登记，就没有组件失败）")
	}
}

func TestHealthyOnlyTurnsRedOnFailed(t *testing.T) {
	cases := []struct {
		state health.State
		want  bool
	}{
		{health.StateStarting, true},
		{health.StateRunning, true},
		{health.StateStopped, true},
		{health.StateDisabled, true},
		{health.StateFailed, false},
	}

	for _, tc := range cases {
		registry := health.New()
		registry.Set("cdc_river", tc.state, nil)

		if got := registry.Healthy(); got != tc.want {
			t.Fatalf("状态 %s 下 Healthy() = %v，期望 %v", tc.state, got, tc.want)
		}
	}
}

// 失败原因必须能带出来，否则排查时只能知道"红了"、不知道"为什么红"。
func TestFailedComponentCarriesError(t *testing.T) {
	registry := health.New()
	registry.Set("cdc_river", health.StateFailed, errors.New("canal: access denied"))

	snapshot := registry.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("期望 1 个组件，实际 %d", len(snapshot))
	}
	if snapshot[0].Error != "canal: access denied" {
		t.Fatalf("失败原因丢失，实际 %q", snapshot[0].Error)
	}
}

// 输出顺序稳定，才能和上一次的快照逐行比对。
func TestSnapshotIsSortedByName(t *testing.T) {
	registry := health.New()
	registry.Set("z_component", health.StateRunning, nil)
	registry.Set("a_component", health.StateRunning, nil)
	registry.Set("m_component", health.StateRunning, nil)

	snapshot := registry.Snapshot()
	want := []string{"a_component", "m_component", "z_component"}
	for i, name := range want {
		if snapshot[i].Name != name {
			t.Fatalf("第 %d 个组件应为 %s，实际 %s", i, name, snapshot[i].Name)
		}
	}
}

// Since 的语义是"进入当前状态的时刻"。同一状态重复上报不该刷新它，
// 否则这个时间戳会退化成"最后一次上报时间"，看不出故障已经持续多久。
func TestSinceChangesOnlyWhenStateChanges(t *testing.T) {
	registry := health.New()
	registry.Set("cdc_river", health.StateStarting, nil)
	startedAt := registry.Snapshot()[0].Since

	time.Sleep(2 * time.Millisecond)
	registry.Set("cdc_river", health.StateStarting, nil)

	if got := registry.Snapshot()[0].Since; !got.Equal(startedAt) {
		t.Fatalf("同状态重复上报不该刷新 Since：%v → %v", startedAt, got)
	}

	time.Sleep(2 * time.Millisecond)
	registry.Set("cdc_river", health.StateRunning, nil)

	if got := registry.Snapshot()[0].Since; got.Equal(startedAt) {
		t.Fatal("状态变化后 Since 必须刷新")
	}
}
