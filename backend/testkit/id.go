package testkit

import (
	"sync/atomic"
)

// SeqID 生成进程内单调递增的整数 ID，用于替代雪花 / 随机 ID。
//
// 测试里断言“ID 非零且递增”比断言“等于某个雪花值”有意义得多；
// 需要跨实例唯一时，调用方自己加前缀即可。
type SeqID struct {
	n atomic.Uint64
}

// NewSeqID 创建一个从 start 开始计数的 ID 生成器。
// 首次 Next() 返回 start+1，便于用 0 表示“未分配”。
func NewSeqID(start uint64) *SeqID {
	s := &SeqID{}
	s.n.Store(start)
	return s
}

// Next 返回下一个 ID，并发安全。
func (s *SeqID) Next() uint64 { return s.n.Add(1) }
