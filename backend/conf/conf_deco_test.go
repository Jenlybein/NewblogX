package conf_test

import (
	"math"
	"testing"

	"myblogx/conf"
)

// 为什么值得测：Normalize 是写入路径上唯一的防线。管理员在后台填了离谱的
// 站标尺寸时，库里必须永远存不下发散值 —— 而它又不报错，所以没有测试就
// 完全看不出来"到底钳了没有"。
func TestDecoNormalize_越界钳位与非法值回落(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"远超上限钳到 96", 4000, conf.CharmLogoSizeMax},
		{"略超上限也钳回边界", 96.5, conf.CharmLogoSizeMax},
		{"略低于下限钳到 24", 10, conf.CharmLogoSizeMin},
		{"下限本身不变（边界值不漂移）", 24, 24},
		{"上限本身不变（边界值不漂移）", 96, 96},
		{"区间内的值原样保留", 60, 60},
		{"默认值 48 原样保留", 48, 48},
		{"缺省（零值）补默认值", 0, conf.CharmLogoSizeDefault},
		{"负数属于非法值，回落默认而不是钳到 24", -5, conf.CharmLogoSizeDefault},
		{"NaN 回落默认", math.NaN(), conf.CharmLogoSizeDefault},
		{"+Inf 回落默认", math.Inf(1), conf.CharmLogoSizeDefault},
		{"-Inf 回落默认", math.Inf(-1), conf.CharmLogoSizeDefault},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := conf.Deco{Charm: conf.Charm{Enable: true, LogoSize: c.in}}.Normalize()
			if got.Charm.LogoSize != c.want {
				t.Fatalf("LogoSize=%v 归一化后应为 %v，实际 %v", c.in, c.want, got.Charm.LogoSize)
			}
			// 开关不是数值，不能被归一化顺手改掉。
			if !got.Charm.Enable {
				t.Fatal("Normalize 不应改动 enable 总开关")
			}
		})
	}
}

func TestDecoNormalize_幂等(t *testing.T) {
	// 幂等是"读者站与后台再钳一次也是同一道防线"的前提：
	// 第二次归一化必须与第一次结果相同，否则两端会来回抖动。
	once := conf.Deco{Charm: conf.Charm{LogoSize: 9}}.Normalize()
	twice := once.Normalize()
	if once != twice {
		t.Fatalf("Normalize 应幂等，第一次 %+v，第二次 %+v", once, twice)
	}
}
