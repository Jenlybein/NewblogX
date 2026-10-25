package writer

import (
	"sync"
	"time"
)

// event_id 的位布局（共 64 位）：
//
//	| 48 位毫秒时间戳 | 8 位实例号 | 8 位毫秒内序列 |
//
// # 为什么不用雪花 ID
//
// 日志要的只有两件事：**同一实例内单调**、**跨实例不冲突**。
// 雪花 ID 能满足，但它要求全局 workerID 配置正确、且必须显式初始化
// （idgen.Init）—— 没初始化就写不出日志。日志是最不该依赖额外初始化步骤的东西：
// 恰恰是启动阶段出错时，你最需要日志。
//
// 这里的实例号直接取 config 里已有的 System.ServerID，零初始化。
//
// # 为什么不直接用时间戳
//
// 同一毫秒会写多条（一次请求可能同时落 runtime + audit 两条），
// 只靠时间戳会重复。加 8 位序列后同毫秒可容纳 256 条。
const (
	eventIDSeqBits      = 8
	eventIDInstanceBits = 8
	eventIDMaxSeq       = 1<<eventIDSeqBits - 1
)

// eventIDState 是进程内的序列状态。
//
// 全局一个而不是每实例一个：一个进程就是一个日志写入者，
// 实例号只是拼进 ID 的标签，不影响序列状态。
var eventIDState struct {
	mu     sync.Mutex
	lastMs int64
	seq    uint64
}

// nextEventID 生成一个事件 ID。
//
// 单调性靠两点保证：
//   - 毫秒前进 → 序列归零；
//   - 毫秒不变（或时钟回拨）→ 序列自增。
//
// 同一毫秒写满 256 条时，把毫秒"借"到下一毫秒继续——宁可让 ID 里的时间戳
// 比真实时间快 1ms，也不允许重复：event_id 是日志去重与排序的依据。
func nextEventID(now time.Time, instance uint32) uint64 {
	ms := now.UnixMilli()

	eventIDState.mu.Lock()
	defer eventIDState.mu.Unlock()

	if ms > eventIDState.lastMs {
		eventIDState.lastMs = ms
		eventIDState.seq = 0
	} else {
		// 同毫秒或时钟回拨：都基于 lastMs 递推，避免 ID 回退。
		ms = eventIDState.lastMs
		if eventIDState.seq >= eventIDMaxSeq {
			eventIDState.lastMs++
			ms = eventIDState.lastMs
			eventIDState.seq = 0
		} else {
			eventIDState.seq++
		}
	}

	return uint64(ms)<<(eventIDInstanceBits+eventIDSeqBits) |
		uint64(instance&0xFF)<<eventIDSeqBits |
		eventIDState.seq
}
