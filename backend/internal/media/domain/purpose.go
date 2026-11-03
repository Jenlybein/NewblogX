package domain

import "strings"

// Purpose 是一次上传任务声明的用途。
//
// 用途只属于上传任务（Redis 里的临时状态），不进图片表：同一张图片
// 秒传命中后可能被别的用途复用，把用途焊死在图片行上会撒谎。
type Purpose string

const (
	// PurposeCover 封面：目标比例 16:9。
	PurposeCover Purpose = "cover"
	// PurposeAvatar 头像：目标比例 1:1。
	PurposeAvatar Purpose = "avatar"
)

// ParsePurpose 把请求里的用途字面量收成枚举；未知值返回 false。
func ParsePurpose(raw string) (Purpose, bool) {
	switch p := Purpose(strings.ToLower(strings.TrimSpace(raw))); p {
	case PurposeCover, PurposeAvatar:
		return p, true
	default:
		return "", false
	}
}

// Ratio 是图片的宽高比，W/H 均为正整数。
type Ratio struct {
	W int
	H int
}

// TargetRatio 返回该用途的目标比例；目标比例是设计常量，不进运行时配置。
func (p Purpose) TargetRatio() (Ratio, bool) {
	switch p {
	case PurposeCover:
		return Ratio{W: 16, H: 9}, true
	case PurposeAvatar:
		return Ratio{W: 1, H: 1}, true
	default:
		return Ratio{}, false
	}
}

// Matches 判断真实像素尺寸与目标比例的相对偏差是否在 ±1%（含）以内。
//
// 用整数交叉相乘而不是浮点：|实际比例/目标比例 - 1| ≤ 1/100 等价于
// 99·W·height ≤ 100·width·H ≤ 101·W·height（实际比例 = width/height）。
// 尺寸都是 int（最大几千），乘积用 int64 不会溢出，也避开了 0.01 的
// 二进制表示误差把边界判反。
func (r Ratio) Matches(width, height int) bool {
	if r.W <= 0 || r.H <= 0 || width <= 0 || height <= 0 {
		return false
	}
	base := int64(r.W) * int64(height)
	actual := 100 * int64(width) * int64(r.H)
	return 99*base <= actual && actual <= 101*base
}
