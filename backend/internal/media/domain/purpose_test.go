package domain_test

import (
	"testing"

	"myblogx/internal/media/domain"
)

// 上传用途是契约字段，只有 cover / avatar 两种；其它字面量必须被拒绝，
// 否则任务会带着"未知用途"进入确认，比例校验无从谈起。
func TestParsePurposeAcceptsOnlyCoverAndAvatar(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		want domain.Purpose
		ok   bool
	}{
		"cover":  {want: domain.PurposeCover, ok: true},
		"avatar": {want: domain.PurposeAvatar, ok: true},
		" AVATAR ": {want: domain.PurposeAvatar, ok: true},
		"":       {ok: false},
		"poster": {ok: false},
		"Cover2": {ok: false},
	}
	for input, want := range cases {
		got, ok := domain.ParsePurpose(input)
		if ok != want.ok || (ok && got != want.want) {
			t.Fatalf("ParsePurpose(%q)=%q,%v want %q,%v", input, got, ok, want.want, want.ok)
		}
	}
}

// 目标比例是设计常量，按用途定，不进运行时配置：封面 16:9、头像 1:1。
func TestPurposeTargetRatio(t *testing.T) {
	t.Parallel()

	cover, ok := domain.PurposeCover.TargetRatio()
	if !ok || cover.W != 16 || cover.H != 9 {
		t.Fatalf("封面目标比例应为 16:9，实际 %+v ok=%v", cover, ok)
	}
	avatar, ok := domain.PurposeAvatar.TargetRatio()
	if !ok || avatar.W != 1 || avatar.H != 1 {
		t.Fatalf("头像目标比例应为 1:1，实际 %+v ok=%v", avatar, ok)
	}
	if _, ok := (domain.Purpose("poster")).TargetRatio(); ok {
		t.Fatal("未知用途不应有目标比例")
	}
}

// 比例判定用"与目标比例的相对偏差 ≤1%"：这些期望值来自独立算好的
// 字面量（1600×900 是 16:9；1616×900 恰好偏 1%；1617×900 偏 1.06%）。
func TestRatioMatchesWithinOnePercent(t *testing.T) {
	t.Parallel()

	cover := domain.Ratio{W: 16, H: 9}
	avatar := domain.Ratio{W: 1, H: 1}

	cases := []struct {
		name   string
		ratio  domain.Ratio
		width  int
		height int
		want   bool
	}{
		{"封面 1600×900 合规", cover, 1600, 900, true},
		{"封面 1600×899 偏差 0.11% 合规", cover, 1600, 899, true},
		{"封面 1616×900 恰好偏 1% 合规", cover, 1616, 900, true},
		{"封面 1617×900 偏 1.06% 不合规", cover, 1617, 900, false},
		{"封面 1600×890 偏 1.12% 不合规", cover, 1600, 890, false},
		{"封面拿方图 900×900 不合规", cover, 900, 900, false},
		{"头像 256×256 合规", avatar, 256, 256, true},
		{"头像 256×254 偏差 0.78% 合规", avatar, 256, 254, true},
		{"头像 256×250 偏差 2.4% 不合规", avatar, 256, 250, false},
		{"头像拿宽图 1600×900 不合规", avatar, 1600, 900, false},
		{"零宽不合规", cover, 0, 900, false},
		{"零高不合规", cover, 1600, 0, false},
		{"负尺寸不合规", cover, -16, 9, false},
		{"空比例不合规", domain.Ratio{}, 1600, 900, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ratio.Matches(tc.width, tc.height); got != tc.want {
				t.Fatalf("Matches(%d,%d)=%v want %v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}
