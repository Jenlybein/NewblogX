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

// 挂绳几何与物理的取值范围与默认值。
//
// 单位分别是 px、节（节点数）、px/s²、无量纲（每步速度保持系数）。
// 与 site_default_settings.yaml 的 deco.charm 对齐；票 #43 的跨端一致性门禁
// 会读这一组常量，读者站与后台的 TS 常量必须逐字一致。
const (
	// CharmRopeLengthMin 是挂绳长度下限：再短就看不出"被绳子拖着"。
	CharmRopeLengthMin = 40.0
	// CharmRopeLengthMax 是挂绳长度上限：再长会横穿阅读区。
	CharmRopeLengthMax = 240.0
	// CharmRopeLengthDefault 是默认挂绳长度。
	CharmRopeLengthDefault = 120.0

	// CharmSegmentsMin 是链节点数下限：再少就不是绳子而是一根杆。
	CharmSegmentsMin = 4
	// CharmSegmentsMax 是链节点数上限：再多只是白烧 CPU。
	CharmSegmentsMax = 24
	// CharmSegmentsDefault 是默认链节点数。
	CharmSegmentsDefault = 10

	// CharmGravityMin 是重力加速度下限：再小绳子会飘得像没有重量。
	CharmGravityMin = 400.0
	// CharmGravityMax 是重力加速度上限：再大摆动会快得看不清。
	CharmGravityMax = 4000.0
	// CharmGravityDefault 是默认重力加速度。
	CharmGravityDefault = 1600.0

	// CharmDampingMin 是每步速度保持系数下限：再小摆动会僵住。
	CharmDampingMin = 0.94
	// CharmDampingMax 是每步速度保持系数上限：**必须小于 1**，
	// 否则每步都在放大速度，摆动会越甩越大（这是真正的发散值）。
	CharmDampingMax = 0.995
	// CharmDampingDefault 是默认阻尼。
	CharmDampingDefault = 0.99
)

// Deco 是装饰配置段。
type Deco struct {
	// Charm 是光标挂件的参数子块。
	Charm Charm `yaml:"charm" json:"charm"`
}

// Charm 是光标挂件的参数。
//
// 总开关 + 五个数值参数。挂绳长度、节点数、重力、阻尼由票 #41 接入物理；
// 弧垂、静止摆动、线宽、恢复默认是票 #42 / #43，不在这里提前加字段。
type Charm struct {
	// Enable 是总开关。默认开着：功能上线后读者立刻就能看到。
	Enable bool `yaml:"enable" json:"enable"`
	// LogoSize 是站标显示尺寸，单位 px。
	LogoSize float64 `yaml:"logo_size" json:"logo_size"`
	// RopeLength 是挂绳长度（松垂阈值），单位 px。
	RopeLength float64 `yaml:"rope_length" json:"rope_length"`
	// Segments 是 Verlet 链的节点数（节），整数计数。
	Segments int `yaml:"segments" json:"segments"`
	// Gravity 是重力加速度，单位 px/s²。
	Gravity float64 `yaml:"gravity" json:"gravity"`
	// Damping 是每个固定步长后保留的速度比例（无量纲，必须小于 1）。
	Damping float64 `yaml:"damping" json:"damping"`
}

// Normalize 把装饰配置收敛进合法范围，永不报错。
//
// 四条规则，按顺序：
//   - 非有限值（NaN / ±Inf）回落默认值：它们序列化进 JSON 会直接失败；
//   - 非正值回落默认值：0 是 Go 的零值（= 字段没填），负值是明显的手滑，
//     两者都不是"我想调到最小"的意图，钳成下限会把"没填"变成"填了最小值"；
//   - 其余越界值钳到边界：保证库里永远存不下发散值；
//   - 计数类（segments）钳成整数，不接受半节。
//
// 不返回 error 是有意的：管理员改一半被 400 打回只会让他猜哪里错了。
func (d Deco) Normalize() Deco {
	d.Charm.LogoSize = normalizePositiveRange(d.Charm.LogoSize, CharmLogoSizeMin, CharmLogoSizeMax, CharmLogoSizeDefault)
	d.Charm.RopeLength = normalizePositiveRange(d.Charm.RopeLength, CharmRopeLengthMin, CharmRopeLengthMax, CharmRopeLengthDefault)
	d.Charm.Gravity = normalizePositiveRange(d.Charm.Gravity, CharmGravityMin, CharmGravityMax, CharmGravityDefault)
	d.Charm.Damping = normalizePositiveRange(d.Charm.Damping, CharmDampingMin, CharmDampingMax, CharmDampingDefault)
	d.Charm.Segments = normalizeSegments(d.Charm.Segments)
	return d
}

// normalizePositiveRange 是"正数才有意义"的数值参数共用钳位规则。
//
// 四个物理参数与站标尺寸量纲完全不同，但失效模式是同一条：NaN 写进 JSON
// 会失败、0 代表没填、负数代表手滑、越界代表填错。共用一条规则而不是各写
// 一份，是为了以后加参数时不会有人只抄到"钳位"却漏掉"回落默认"。
func normalizePositiveRange(v, min, max, fallback float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	if v <= 0 {
		return fallback
	}
	return math.Min(max, math.Max(min, v))
}

// normalizeSegments 是节点数的钳位规则：计数没有 NaN / Infinity，
// 只有"没填（0 或负数）"与"越界"两种情况。
func normalizeSegments(v int) int {
	if v <= 0 {
		return CharmSegmentsDefault
	}
	if v < CharmSegmentsMin {
		return CharmSegmentsMin
	}
	if v > CharmSegmentsMax {
		return CharmSegmentsMax
	}
	return v
}
