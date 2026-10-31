// 装饰配置（第 8 个运行时配置段）
//
// 只放"纯装饰"参数：它们不影响任何业务链路，改错了最多难看，不会让站点坏掉。
// 开这个段而不是把挂件参数塞进 site 段，是因为装饰会越来越多（背景、边框），
// 而每开一个新段都要动一遍段注册的八个点 —— 第 8 段一次付清这笔成本。
//
// 本票只落光标挂件的子块 charm；将来加别的装饰不用再开第 9 段。
package conf

import "math"

// 光标挂件站标尺寸的取值范围与默认值，单位：px。
//
// 与 site_default_settings.yaml 的 deco.charm.logo_size 必须一致：
// 常量是写入路径的钳位边界，yaml 是"库里缺这段"时的补种值，漂移了就会
// 出现"后台能存 48、前端却按 24 渲染"。
const (
	// CharmLogoSizeMin 是站标显示尺寸下限（小于它就看不清是什么东西）。
	CharmLogoSizeMin = 24.0
	// CharmLogoSizeMax 是站标显示尺寸上限（再大就会压住正文）。
	CharmLogoSizeMax = 96.0
	// CharmLogoSizeDefault 是后台没有填过时用的站标尺寸。
	CharmLogoSizeDefault = 48.0
)

// Deco 是装饰配置段。
type Deco struct {
	// Charm 是光标挂件的参数子块。
	Charm Charm `yaml:"charm" json:"charm"`
}

// Charm 是光标挂件的参数。
//
// 本票只有两个参数：总开关与站标尺寸。挂绳长度、节点数等留给后续票，
// 届时它们在这里各加一行 —— 不需要再动段注册。
type Charm struct {
	// Enable 是总开关。默认开着：功能上线后读者立刻就能看到。
	Enable bool `yaml:"enable" json:"enable"`
	// LogoSize 是站标显示尺寸，单位 px。
	LogoSize float64 `yaml:"logo_size" json:"logo_size"`
}

// Normalize 把装饰配置收敛进合法范围，永不报错。
//
// 三条规则，按顺序：
//   - 非有限值（NaN / ±Inf）回落默认值：它们序列化进 JSON 会直接失败；
//   - 非正值回落默认值：0 是 Go 的零值（= 字段没填），负值是明显的手滑，
//     两者都不是"我想调到最小"的意图，钳成 24 会把"没填"变成"填了最小值"；
//   - 其余越界值钳到边界：保证库里永远存不下发散值。
//
// 不返回 error 是有意的：管理员改一半被 400 打回只会让他猜哪里错了。
func (d Deco) Normalize() Deco {
	d.Charm.LogoSize = normalizeLogoSize(d.Charm.LogoSize)
	return d
}

// normalizeLogoSize 是 LogoSize 的钳位规则本体。
func normalizeLogoSize(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return CharmLogoSizeDefault
	}
	if v <= 0 {
		return CharmLogoSizeDefault
	}
	return math.Min(CharmLogoSizeMax, math.Max(CharmLogoSizeMin, v))
}
