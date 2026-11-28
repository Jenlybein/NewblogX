package app

import (
	"testing"

	confsite "myblogx/conf/site"
)

// 2026-11-28 / ADR-0037 §5.2：`aiFailureLandingFor` 的分流表。
//
// 只测这个纯函数，不测落点函数——落点函数的行为（转人工建任务 / 次日重投 /
// 退回草稿 / 直接放行）各自已有用例，而"选对落点"是这里唯一的决策。
//
// 边界（本文件的全部意义）：
//   - 四档取值各自映射到自己；
//   - **未知取值 / 空串一律 manual** —— 失败方向必须是"交给人"，绝不能是"放行"。
//     一次手改库写错的值不该变成"文章不经合规判定就公开"。
func TestAIFailureLandingFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		action string
		want   aiFailureLanding
	}{
		{confsite.AIFailureManual, aiFailureLandingManual},
		{confsite.AIFailureHold, aiFailureLandingHold},
		{confsite.AIFailureDraft, aiFailureLandingDraft},
		{confsite.AIFailureRelease, aiFailureLandingRelease},
		// 未知 / 空串：不许放行。
		{"", aiFailureLandingManual},
		{"auto", aiFailureLandingManual},
		{"Release", aiFailureLandingManual},
		{" release", aiFailureLandingManual},
	}

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			if got := aiFailureLandingFor(tc.action); got != tc.want {
				t.Fatalf("取值 %q 应落到 %d，实际 %d", tc.action, tc.want, got)
			}
		})
	}
}

// TestAIFailureLandingFor_未知取值绝不放行 把最危险的那条单独拧出来。
//
// 合并进上表也可以，但这是一条"安全方向"断言：它红了意味着一次配置写错就能让
// 不合规的文章直接对读者可见。单独一条用例让它在报告里显眼。
func TestAIFailureLandingFor_未知取值绝不放行(t *testing.T) {
	t.Parallel()

	for _, action := range []string{"", "auto", "0", "true", "RELEASE"} {
		if got := aiFailureLandingFor(action); got == aiFailureLandingRelease {
			t.Fatalf("未知取值 %q 绝不能被解释成「直接放行」", action)
		}
	}
}

// TestAIFailureAction读取现值守点 钉住 aiFailureAction() 的兜底方向。
//
// 读不到运行时配置时必须返回 manual（不是空串、也不是 release）：
// 装配缺失的默认方向与"配置写错"同一条口径 —— 交给人。
func TestAIFailureAction读取现值守点(t *testing.T) {
	t.Parallel()

	if got := aiFailureAction(nil); got != confsite.AIFailureManual {
		t.Fatalf("infra 为 nil 时应回落 manual，实际 %q", got)
	}
	if got := aiFailureAction(&Infra{}); got != confsite.AIFailureManual {
		t.Fatalf("Effective 为 nil 时应回落 manual，实际 %q", got)
	}
}
