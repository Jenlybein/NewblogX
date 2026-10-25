package writer

import (
	"testing"
	"time"
)

// TestNextEventIDMonotonic 同一毫秒内连续生成必须严格递增。
//
// 这是 event_id 存在的唯一理由：日志去重与排序。同毫秒写多条是常态
// （一次请求可能同时落 runtime 与 audit），若退化成时间戳就会重复。
func TestNextEventIDMonotonic(t *testing.T) {
	now := time.Unix(1700000000, 0)
	var prev uint64
	for i := 0; i < 1000; i++ {
		id := nextEventID(now, 1)
		if id <= prev {
			t.Fatalf("第 %d 个 ID 未递增: prev=%d got=%d", i, prev, id)
		}
		prev = id
	}
}

// TestNextEventIDSeqOverflow 同一毫秒写满序列后仍不重复。
//
// 溢出时必须"借"下一毫秒，而不是把序列绕回 0 —— 绕回就会和前面 256 条撞车。
func TestNextEventIDSeqOverflow(t *testing.T) {
	now := time.Unix(1700000000, 0)
	seen := make(map[uint64]bool, 600)
	for i := 0; i < 600; i++ {
		id := nextEventID(now, 1)
		if seen[id] {
			t.Fatalf("第 %d 个 ID 重复: %d", i, id)
		}
		seen[id] = true
	}
}

// TestNextEventIDClockBackwards 时钟回拨不能让 ID 回退。
//
// 容器/虚拟机里 NTP 校时导致 time.Now() 倒退是真实存在的，
// 而 ID 一旦回退，按 event_id 排序就会错乱（表现为"日志顺序随机"）。
func TestNextEventIDClockBackwards(t *testing.T) {
	base := time.Unix(1700000000, 0)
	first := nextEventID(base, 1)

	// 时钟倒退 1 秒
	second := nextEventID(base.Add(-time.Second), 1)
	if second <= first {
		t.Fatalf("时钟回拨后 ID 回退: first=%d second=%d", first, second)
	}
}

// TestNextEventIDInstanceBits 实例号只占低 8 位、不污染时间戳部分。
func TestNextEventIDInstanceBits(t *testing.T) {
	now := time.Unix(1700000000, 0)
	a := nextEventID(now, 1)
	b := nextEventID(now, 2)

	// 两次调用的时间部分相同（同一毫秒），差异只应出现在实例/序列位。
	if a>>16 != b>>16 {
		t.Fatalf("实例号影响了时间戳位: a=%b b=%b", a, b)
	}
	if a == b {
		t.Fatal("不同实例号应当产生不同 ID")
	}
}
