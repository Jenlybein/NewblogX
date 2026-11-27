package domain_test

import (
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 管理端评论管理的领域约定（筛选 / 分页 / 理由 / 禁言档位）。
//
// 这一层只钉取值口径：哪些值合法、默认值是什么、边界算不算越界。
// 不钉"谁来调用"（那是用例层）与"怎么拼 SQL"（那是仓储）。

func TestParseAdminStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		want    domain.AdminStatus
		wantErr bool
	}{
		{"不传等于全部", "", domain.AdminStatusAll, false},
		{"显式全部", "all", domain.AdminStatusAll, false},
		{"已发布", "published", domain.AdminStatusPublished, false},
		{"已删除", "deleted", domain.AdminStatusDeleted, false},
		{"带空白", "  deleted  ", domain.AdminStatusDeleted, false},
		// 未知取值必须报错：静默当成 all 会让运营以为筛过了，实际看到全量。
		{"未知取值", "pending", "", true},
		{"大小写敏感", "All", "", true},
	}
	for _, c := range cases {
		got, err := domain.ParseAdminStatus(c.raw)
		if c.wantErr {
			if !apperr.IsCode(err, apperr.CodeCommentAdminStatusInvalid) {
				t.Fatalf("%s: 应返回 comment.admin_status_invalid，实际 %v", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: 不该失败: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: 应为 %q，实际 %q", c.name, c.want, got)
		}
	}
}

func TestAdminStatusIsValid(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.AdminStatus{
		domain.AdminStatusAll, domain.AdminStatusPublished, domain.AdminStatusDeleted,
	} {
		if !status.IsValid() {
			t.Fatalf("%q 应合法", status)
		}
	}
	for _, status := range []domain.AdminStatus{"", "pending"} {
		if status.IsValid() {
			t.Fatalf("%q 不该合法（空串要先走 Normalize 补默认值）", status)
		}
	}
}

func TestAdminPageNormalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       domain.AdminPage
		wantPage int
		wantSize int
		wantOff  int
	}{
		{"零值用默认 20 条", domain.AdminPage{}, 1, domain.AdminDefaultLimit, 0},
		{"页码为负归 1", domain.AdminPage{Page: -3, Limit: 10}, 1, 10, 0},
		{"上限夹到 100", domain.AdminPage{Page: 2, Limit: 500}, 2, domain.AdminMaxLimit, 100},
		{"正常透传", domain.AdminPage{Page: 3, Limit: 10}, 3, 10, 20},
	}
	for _, c := range cases {
		got := c.in.Normalize()
		if got.Page != c.wantPage || got.Limit != c.wantSize {
			t.Fatalf("%s: 收敛成 (%d,%d)，实际 %+v", c.name, c.wantPage, c.wantSize, got)
		}
		if off := c.in.Offset(); off != c.wantOff {
			t.Fatalf("%s: 偏移量应为 %d，实际 %d", c.name, c.wantOff, off)
		}
	}
}

func TestAdminFilterNormalize(t *testing.T) {
	t.Parallel()

	in := domain.AdminFilter{Keyword: "  spam  ", UserKeyword: "\t路人 \n"}
	got := in.Normalize()

	if got.Keyword != "spam" || got.UserKeyword != "路人" {
		t.Fatalf("关键词应去首尾空白: %+v", got)
	}
	// 空状态必须补成全部：仓储按"没有 status 条件"处理，但用例层要能断言
	// "确实是不筛状态"，而不是一个未定义的零值。
	if got.Status != domain.AdminStatusAll {
		t.Fatalf("空状态应变 all，实际 %q", got.Status)
	}

	// 已有的状态不能被 Normalize 改写。
	explicit := domain.AdminFilter{Status: domain.AdminStatusDeleted}.Normalize()
	if explicit.Status != domain.AdminStatusDeleted {
		t.Fatalf("显式状态被改写了: %q", explicit.Status)
	}
}

func TestNormalizeAdminMuteDays(t *testing.T) {
	t.Parallel()

	for _, days := range []int{0, 1, 7, 30} {
		got, err := domain.NormalizeAdminMuteDays(days)
		if err != nil {
			t.Fatalf("档位 %d 应合法，实际 %v", days, err)
		}
		// 永久档必须原样是 0：折成 365 / 3650 会让"永久"变成一个会到期的谎话。
		if got != days {
			t.Fatalf("档位 %d 应原样返回，实际 %d", days, got)
		}
	}
	for _, days := range []int{-1, 3, 365, 3650} {
		if _, err := domain.NormalizeAdminMuteDays(days); !apperr.IsCode(err, apperr.CodeCommentAdminDaysInvalid) {
			t.Fatalf("档位 %d 应被拒绝（只能 1/7/30/0），实际 %v", days, err)
		}
	}
}

func TestNormalizeAdminReason(t *testing.T) {
	t.Parallel()

	if _, err := domain.NormalizeAdminReason("  "); !apperr.IsCode(err, apperr.CodeCommentAdminReasonRequired) {
		t.Fatalf("空白理由应报 comment.admin_reason_required，实际 %v", err)
	}
	if _, err := domain.NormalizeAdminReason(strings.Repeat("长", domain.MaxAdminReasonLen+1)); !apperr.IsCode(err, apperr.CodeCommentAdminReasonTooLong) {
		t.Fatalf("超长理由应报 comment.admin_reason_too_long，实际 %v", err)
	}

	got, err := domain.NormalizeAdminReason("  刷屏  ")
	if err != nil {
		t.Fatalf("合法理由不该失败: %v", err)
	}
	if got != "刷屏" {
		t.Fatalf("理由应去首尾空白，实际 %q", got)
	}

	// 边界正好 200 字必须允许（中文按 rune 计数，不是字节数）。
	exact := strings.Repeat("长", domain.MaxAdminReasonLen)
	if _, err := domain.NormalizeAdminReason(exact); err != nil {
		t.Fatalf("正好 %d 字应允许，实际 %v", domain.MaxAdminReasonLen, err)
	}
}

func TestNormalizeAdminOptionalReason(t *testing.T) {
	t.Parallel()

	got, err := domain.NormalizeAdminOptionalReason("   ")
	if err != nil {
		t.Fatalf("封禁理由可留空，实际 %v", err)
	}
	if got != "" {
		t.Fatalf("空白理由应归零，实际 %q", got)
	}
	if _, err := domain.NormalizeAdminOptionalReason(strings.Repeat("长", domain.MaxAdminReasonLen+1)); !apperr.IsCode(err, apperr.CodeCommentAdminReasonTooLong) {
		t.Fatalf("超长理由仍要拒绝，实际 %v", err)
	}
}

// 编译期确认时间字段用的是指针（"不限"与"零点"必须能区分）。
var (
	_ *time.Time = domain.AdminFilter{}.FromAt
	_ ctype.ID   = domain.AdminFilter{}.UserID
)
