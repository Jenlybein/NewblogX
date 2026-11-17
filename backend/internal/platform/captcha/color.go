package captcha

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseBGColor 把 `#RRGGBB` / `#RRGGBBAA`（`#` 可省）解析成 image/color.RGBA。
//
// 为什么让配置写十六进制而不是四个整数：运营改的是"那个深蓝色再暗一点"，
// 十六进制可以直接从取色器里粘过来；四个整数得先想清楚 alpha 放哪一位。
//
// 6 位时 alpha 取不透明（0xFF）；8 位时第 7–8 位是 alpha。
func ParseBGColor(value string) (color.RGBA, error) {
	raw := strings.TrimSpace(value)
	raw = strings.TrimPrefix(raw, "#")
	switch len(raw) {
	case 6:
		raw += "FF"
	case 8:
	default:
		return color.RGBA{}, fmt.Errorf("背景色需写成 #RRGGBB 或 #RRGGBBAA，收到 %q", value)
	}
	n, err := strconv.ParseUint(raw, 16, 64)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("背景色不是合法的十六进制颜色: %q", value)
	}
	return color.RGBA{
		R: uint8(n >> 24 & 0xFF),
		G: uint8(n >> 16 & 0xFF),
		B: uint8(n >> 8 & 0xFF),
		A: uint8(n & 0xFF),
	}, nil
}
