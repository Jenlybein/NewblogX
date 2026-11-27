package http

import (
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 管理端评论列表的 HTTP 层辅助判据（日期边界与审计目标 ID）。
//
// 这里只测不依赖 Service 的纯函数：handler 本身的参数校验一律走用例层的
// 错误码（comment.admin_*），HTTP 层唯一自己解析的就是 from / to 两个日期。

func TestAdminDayBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		end     bool
		want    *time.Time
		wantErr bool
	}{
		{"空串表示不限", "  ", false, nil, false},
		{"起始日期取当日零点", "2026-11-01", false, at(2026, 11, 1), false},
		// 右边界取次日零点、查询用 <：这样"含当天"在毫秒精度下也不漏最后一毫秒。
		{"结束日期取次日零点", "2026-11-01", true, at(2026, 11, 2), false},
		{"也接受 RFC3339", "2026-11-01T10:00:00+08:00", false, atTime(2026, 11, 1, 2), false},
		{"格式错按参数错误", "2026/11/01", false, nil, true},
		{"只有月日", "11-01", false, nil, true},
	}
	for _, c := range cases {
		got, err := adminDayBoundary(c.raw, c.end)
		if c.wantErr {
			if !apperr.IsCode(err, apperr.CodeCommonInvalidArgument) {
				t.Fatalf("%s: 应返回 common.invalid_argument，实际 %v", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: 不该失败: %v", c.name, err)
		}
		if c.want == nil {
			if got != nil {
				t.Fatalf("%s: 应为 nil（不限），实际 %v", c.name, got)
			}
			continue
		}
		if got == nil || !got.Equal(*c.want) {
			t.Fatalf("%s: 应为 %v，实际 %v", c.name, *c.want, got)
		}
	}
}

// at 返回本地时区当日零点；atTime 返回 UTC 时刻（RFC3339 会换算成 UTC 表示）。
func at(year int, month time.Month, day int) *time.Time {
	v := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	return &v
}

func atTime(year int, month time.Month, day, hour int) *time.Time {
	v := time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
	local := v.In(time.Local)
	return &local
}

func TestJoinAdminIDs(t *testing.T) {
	t.Parallel()

	if got := joinAdminIDs(nil); got != "" {
		t.Fatalf("空列表应拼成空串，实际 %q", got)
	}
	ids := []ctype.ID{ctype.NewID(11), ctype.NewID(22)}
	if got := joinAdminIDs(ids); got != "11,22" {
		t.Fatalf("应拼成 11,22，实际 %q", got)
	}
}
