package page_test

import (
	"testing"

	"myblogx/internal/kernel/page"
)

// TestPageInfoHelpers 锁定分页参数的收敛规则。
//
// 这些规则看着琐碎，但每一条都是防"前端乱传参把库打穿"：
//   - 页码 <= 0 收敛到 1（不是 0，offset 会变负数）；
//   - 每页条数 > 100 回落到 10（不是"夹到 100"，是直接当非法值）；
//   - 页码超过最大页时夹到最大页（否则白跑一次空查询）。
func TestPageInfoHelpers(t *testing.T) {
	t.Parallel()

	p := page.PageInfo{Page: -1, Limit: 1000}
	if got := p.GetPage(100); got != 1 {
		t.Fatalf("GetPage 应把非法页码收敛到 1，实际 %d", got)
	}
	if got := p.GetLimit(); got != 10 {
		t.Fatalf("GetLimit 应把超限条数回落到 10，实际 %d", got)
	}
	if got := p.GetOffset(100); got != 0 {
		t.Fatalf("GetOffset 应为 0，实际 %d", got)
	}

	p = page.PageInfo{Page: 2, Limit: 5}
	if got := p.GetOffset(100); got != 5 {
		t.Fatalf("GetOffset 计算错误，期望 5 实际 %d", got)
	}
}

// TestPageInfoClampsToLastPage 页码越界时夹到最大页。
func TestPageInfoClampsToLastPage(t *testing.T) {
	t.Parallel()

	p := page.PageInfo{Page: 999, Limit: 10}
	// 25 条 / 每页 10 → 3 页
	if got := p.GetPage(25); got != 3 {
		t.Fatalf("越界页码应夹到 3，实际 %d", got)
	}
	if got := p.GetOffset(25); got != 20 {
		t.Fatalf("越界页码的 offset 应为 20，实际 %d", got)
	}
}

// TestPageInfoNoCount 不知道总数时只做下界收敛。
//
// 这条语义容易被"顺手统一"改掉：GetOffsetNoCount 不能调用 GetPage，
// 因为 GetPage 需要一个 count，而这里恰恰是"还没数"的场景。
func TestPageInfoNoCount(t *testing.T) {
	t.Parallel()

	p := page.PageInfo{Page: 0, Limit: 20}
	if got := p.GetOffsetNoCount(); got != 0 {
		t.Fatalf("第 0 页按第 1 页算，offset 应为 0，实际 %d", got)
	}

	p = page.PageInfo{Page: 3, Limit: 20}
	if got := p.GetOffsetNoCount(); got != 40 {
		t.Fatalf("第 3 页 offset 应为 40，实际 %d", got)
	}
}
