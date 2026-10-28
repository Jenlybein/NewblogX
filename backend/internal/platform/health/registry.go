// Package health 汇总进程内各后台组件的运行状态。
//
// # 为什么需要这个包
//
// 2026-10-29 的线上事故暴露了一类"半死"状态：river（MySQL binlog → ES 投影）
// 启动失败，代码里只有一行 `River 运行失败: %v` 的日志，进程照常监听端口、
// 健康检查照常返回 200。站点看起来完全正常，但新发布的文章永远进不了 ES，
// 首页列表永远是空的 —— 而且没有任何地方会报错。
//
// 这类故障的共同点是：组件死了，但"活着"的信号没有任何一处能反映出来。
// 日志写在没人盯的地方，等于没有。所以把状态变成可查询的：
// 组件启动/失败时登记，健康检查端点汇总返回，探针一眼看出是哪个组件挂了。
//
// 刻意不做的事：
//   - 不做主动探测（不去 ping ES/MySQL）。这里只回答"组件自己报告的状态"，
//     主动探测会把健康检查变成慢接口，而且会掩盖真实故障点。
//   - 不持有组件引用。注册表只存名字和状态，避免变成一个隐式的全局容器。
package health

import (
	"sort"
	"sync"
	"time"
)

// State 是组件的运行状态。
type State string

const (
	// StateStarting 已开始启动，尚未确认进入正常循环。
	StateStarting State = "starting"
	// StateRunning 正常运行中。
	StateRunning State = "running"
	// StateStopped 主动停止（通常是进程退出时的优雅停机），不是故障。
	StateStopped State = "stopped"
	// StateFailed 启动或运行失败。**唯一会让就绪检查变红的状态**。
	StateFailed State = "failed"
	// StateDisabled 配置里没启用。记录它同样有价值：
	// 否则"CDC 没跑"和"CDC 没启用"在观测上完全无法区分。
	StateDisabled State = "disabled"
)

// Component 是单个组件的状态快照。
type Component struct {
	Name  string    `json:"name"`
	State State     `json:"state"`
	Error string    `json:"error,omitempty"`
	Since time.Time `json:"since"`
}

// Registry 是进程内的组件状态注册表，并发安全。
//
// 零值不可用，请用 New 构造。方法对 nil 接收者安全：装配层某些角色
// （如 -role=api）根本没有后台组件，传 nil 也不该让健康检查崩掉。
type Registry struct {
	mu    sync.RWMutex
	comps map[string]Component
}

// New 创建空注册表。
func New() *Registry {
	return &Registry{comps: make(map[string]Component)}
}

// Set 登记组件状态。cause 只应在 state 为 StateFailed 时传入。
//
// 状态没变时不刷新时间戳：Since 的语义是"进入当前状态的时刻"，
// 每次调用都覆盖会让它退化成"最后一次报告的时刻"，失去诊断价值。
func (r *Registry) Set(name string, state State, cause error) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if prev, ok := r.comps[name]; ok && prev.State == state && prev.Error == errText(cause) {
		return
	}

	r.comps[name] = Component{
		Name:  name,
		State: state,
		Error: errText(cause),
		Since: time.Now(),
	}
}

// Snapshot 返回按组件名排序的状态快照。
//
// 排序是为了让输出稳定：健康检查的结果会被日志、监控、人眼反复比对，
// map 的随机顺序会让"这次和上次有什么不同"变成一场拼图。
func (r *Registry) Snapshot() []Component {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Component, 0, len(r.comps))
	for _, c := range r.comps {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Healthy 判断是否所有组件都不处于 StateFailed。
//
// 空注册表视为健康：只跑 HTTP 的角色本来就没有后台组件，
// 不能因为"没登记"就判定为坏。
func (r *Registry) Healthy() bool {
	for _, c := range r.Snapshot() {
		if c.State == StateFailed {
			return false
		}
	}
	return true
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
