package app

import (
	"testing"

	"myblogx/internal/article/domain"
)

// 2026-11-28 / ADR-0037 §5.1：提交落点的组合矩阵。
//
// decideSubmit 是纯函数，但它合成的优先级（风险 > 质量 > 无判据 ⇒ 可见 > 待判定）
// 是全链路最容易漂的一处：四个入参、十六条组合，任何一次改动都可能让某条组合
// 悄悄变了语义（例如"两关都关"被写成了草稿）。表驱动把全部组合钉住。
func TestDecideSubmit_全组合(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		skipExamining       bool
		scoreEnabled        bool
		aiComplianceEnabled bool
		gateRejected        bool
		riskRejected        bool
		wantStatus          domain.Status
		wantVisibility      domain.Visibility
		wantRejected        bool
	}{
		{
			name:                "风险命中优先于一切：即便免审核 + 两关全关，也落已拒绝",
			skipExamining:       true,
			scoreEnabled:        false,
			aiComplianceEnabled: false,
			riskRejected:        true,
			wantStatus:          domain.StatusRejected,
			wantVisibility:      domain.VisibilityVisible,
			wantRejected:        true,
		},
		{
			name:                "质量不达标落草稿（即便免审核开启也不放行）",
			skipExamining:       true,
			scoreEnabled:        true,
			aiComplianceEnabled: true,
			gateRejected:        true,
			wantStatus:          domain.StatusDraft,
			wantVisibility:      domain.VisibilityVisible,
		},
		{
			name:                "两关都关 ⇒ 提交即可见（与免审核同一条分支）",
			scoreEnabled:        false,
			aiComplianceEnabled: false,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityVisible,
		},
		{
			name:                "只关评分 ⇒ 直接可见（没有质量判据可等）",
			scoreEnabled:        false,
			aiComplianceEnabled: true,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityVisible,
		},
		{
			name:                "只关 AI 合规 ⇒ 直接可见（没有模型判定可等）",
			scoreEnabled:        true,
			aiComplianceEnabled: false,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityVisible,
		},
		{
			name:                "免审核 ⇒ 直接可见（两关都开也一样）",
			skipExamining:       true,
			scoreEnabled:        true,
			aiComplianceEnabled: true,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityVisible,
		},
		{
			name:                "两关都开且都不拦 ⇒ 待可见（等模型判定）",
			scoreEnabled:        true,
			aiComplianceEnabled: true,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityPendingReview,
		},
		{
			name:                "两关都开 + 质量过线 + 免审关闭 ⇒ 待可见",
			scoreEnabled:        true,
			aiComplianceEnabled: true,
			skipExamining:       false,
			wantStatus:          domain.StatusPublished,
			wantVisibility:      domain.VisibilityPendingReview,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome := decideSubmit(
				tc.skipExamining, tc.scoreEnabled, tc.aiComplianceEnabled,
				tc.gateRejected, tc.riskRejected,
			)
			if outcome.Status != tc.wantStatus || outcome.Visibility != tc.wantVisibility {
				t.Fatalf("落点期望 %s/%s，实际 %s/%s",
					tc.wantStatus, tc.wantVisibility, outcome.Status, outcome.Visibility)
			}
			if outcome.Rejected != tc.wantRejected {
				t.Fatalf("Rejected 期望 %v，实际 %v", tc.wantRejected, outcome.Rejected)
			}
		})
	}
}

// 「免审核不绕过关键词处置」由结构保证：命中处置在过滤那一步就短路，走不到这里。
// 这条用例是那份保证的文档化 —— 一旦有人把处置判定搬进 decideSubmit，它就会红。
func TestDecideSubmit_不接收命中处置(t *testing.T) {
	t.Parallel()

	// 签名里只有五个布尔位：skipExamining / scoreEnabled / aiComplianceEnabled /
	// gateRejected / riskRejected。若将来有人加 hitTier 参数，这条注释与用例一起改。
	outcome := decideSubmit(true, true, true, false, false)
	if outcome.Status != domain.StatusPublished {
		t.Fatalf("免审核应直接可见，实际 %s", outcome.Status)
	}
}
