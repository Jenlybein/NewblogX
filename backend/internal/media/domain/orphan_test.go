package domain_test

import (
	"testing"
	"time"

	"myblogx/internal/media/domain"
)

// 冷静期是误删的最后一道防线：零值/负值必须回落默认值，而不是被当成
// "关掉保护"。这组用例把这条口径钉住。
func TestOrphanScanConfigNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   domain.OrphanScanConfig
		want domain.OrphanScanConfig
	}{
		{"零值回落默认", domain.OrphanScanConfig{}, domain.OrphanScanConfig{GraceDays: 14, BatchLimit: 50}},
		{"负值回落默认", domain.OrphanScanConfig{GraceDays: -3, BatchLimit: -1}, domain.OrphanScanConfig{GraceDays: 14, BatchLimit: 50}},
		{"超过硬上限被夹回", domain.OrphanScanConfig{GraceDays: 7, BatchLimit: 9999}, domain.OrphanScanConfig{GraceDays: 7, BatchLimit: 500}},
		{"合法值原样保留", domain.OrphanScanConfig{GraceDays: 30, BatchLimit: 20}, domain.OrphanScanConfig{GraceDays: 30, BatchLimit: 20}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Normalize(); got != tc.want {
				t.Fatalf("Normalize(%+v)=%+v want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestOrphanScanConfigGracePeriod(t *testing.T) {
	cfg := domain.OrphanScanConfig{GraceDays: 14}
	if got := cfg.GracePeriod(); got != 14*24*time.Hour {
		t.Fatalf("冷静期换算错误: %s", got)
	}
}

// 审核中尚未定性、封禁可能还要留作证据：两者都不得进入候选清单。
func TestCleanableStatusesExcludesReviewingAndBlocked(t *testing.T) {
	allowed := make(map[domain.Status]bool)
	for _, status := range domain.CleanableStatuses() {
		allowed[status] = true
	}
	for _, status := range []domain.Status{domain.StatusReviewing, domain.StatusBlocked, domain.StatusDeleted, domain.StatusUnknown} {
		if allowed[status] {
			t.Fatalf("状态 %s 不该进入候选清单", status)
		}
	}
	if !allowed[domain.StatusPass] {
		t.Fatal("审核通过是正常状态，必须在候选清单的白名单里")
	}
}
