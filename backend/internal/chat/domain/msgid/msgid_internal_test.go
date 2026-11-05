package msgid

import (
	"testing"

	"myblogx/internal/kernel/ctype"
)

// 号段是与正常消息主键不相交的实现细节，所以属性只在包内验：
// 派生结果必须落在保留区间内，且不会"看起来像"一条普通雪花消息。
func TestDerivedIDFallsIntoReservedSegment(t *testing.T) {
	sender := ctype.NewID(100)
	for _, key := range []string{"a", "b", "client-key-1", "", "很长的幂等键-with-mixed-字符"} {
		got := FromClientKey(sender, key)
		if got.Uint64() < clientKeyIDBase {
			t.Fatalf("幂等键 %q 的派生主键必须落在保留区间 [%d, 2^63)，实际 %s",
				key, clientKeyIDBase, got)
		}
	}
}
