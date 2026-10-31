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

	// CharmLineWidthMin 是描边起始宽度下限：1 px，再细就看不见了。
	CharmLineWidthMin = 1.0
	// CharmLineWidthMax 是描边起始宽度上限：6 px，再粗会抢正文。
	CharmLineWidthMax = 6.0
	// CharmLineWidthDefault 是默认描边起始宽度（收尾固定为它的 0.6 倍，不进配置）。
	CharmLineWidthDefault = 2.5

	// CharmIdleSwayHzMin 是静止摆动频率下限。**0 是合法值**：它表示关掉轻晃。
	CharmIdleSwayHzMin = 0.0
	// CharmIdleSwayHzMax 是静止摆动频率上限：再快就不是"轻晃"了。
	CharmIdleSwayHzMax = 1.5
	// CharmIdleSwayHzDefault 是默认静止摆动频率（0.8Hz 的周期是 1.25s）。
	CharmIdleSwayHzDefault = 0.8

	// CharmIdleSwayAmpMin 是静止摆动幅度下限（px/s²）。**0 是合法值**：
	// 它让组合式退回"静止即停帧"的零帧车道（见票 #42 的实施裁决）。
	CharmIdleSwayAmpMin = 0.0
	// CharmIdleSwayAmpMax 是静止摆动幅度上限（px/s²）。
	CharmIdleSwayAmpMax = 40.0
	// CharmIdleSwayAmpDefault 是默认静止摆动幅度。
	CharmIdleSwayAmpDefault = 20.0
)

// Deco 是装饰配置段。
type Deco struct {
	// Charm 是光标挂件的参数子块。
	Charm Charm `yaml:"charm" json:"charm"`
}

// Charm 是光标挂件的参数。
//
// 总开关 + 八个数值参数。票 #41 接入挂绳长度、节点数、重力、阻尼；
// 票 #42 接入描边宽度、静止摆动频率与幅度（弧垂由"绳长 vs 实际距离"产生，
// 没有单独的"sag"参数）。
type Charm struct {
	// Enable 是总开关。默认开着：功能上线后读者立刻就能看到。
	Enable bool `yaml:"enable" json:"enable"`
	// LogoSize 是站标显示尺寸，单位 px。
	LogoSize float64 `yaml:"logo_size" json:"logo_size"`
	// RopeLength 是挂绳长度（松垂阈值），单位 px。
	RopeLength float64 `yaml:"rope_length" json:"rope_length"`
	// Segments 是 Verlet 链的节点数（节），整数计数。
	Segments int `yaml:"segments" json:"segments"`
	// LineWidth 是描边起始宽度，单位 px（收尾宽度固定为它的 0.6 倍）。
	LineWidth float64 `yaml:"line_width" json:"line_width"`
	// Gravity 是重力加速度，单位 px/s²。
	Gravity float64 `yaml:"gravity" json:"gravity"`
	// Damping 是每个固定步长后保留的速度比例（无量纲，必须小于 1）。
	Damping float64 `yaml:"damping" json:"damping"`
	// IdleSwayHz 是静止摆动的频率，单位 Hz。0 表示关掉轻晃。
	IdleSwayHz float64 `yaml:"idle_sway_hz" json:"idle_sway_hz"`
	// IdleSwayAmp 是静止摆动的加速度幅度，单位 px/s²，作用在自由端。
	// 0 表示关掉轻晃（读者站据此退回静止零帧车道）。
	IdleSwayAmp float64 `yaml:"idle_sway_amp" json:"idle_sway_amp"`
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
	d.Charm.LineWidth = normalizeLineWidth(d.Charm.LineWidth)
	// 轻晃的两个参数把 0 当作"关掉轻晃"的合法意图（裁决要求 amp=0 能退回零帧），
	// 所以它们走"非负区间"规则而不是"正数"规则。
	d.Charm.IdleSwayHz = normalizeNonNegativeRange(d.Charm.IdleSwayHz, CharmIdleSwayHzMin, CharmIdleSwayHzMax, CharmIdleSwayHzDefault)
	d.Charm.IdleSwayAmp = normalizeNonNegativeRange(d.Charm.IdleSwayAmp, CharmIdleSwayAmpMin, CharmIdleSwayAmpMax, CharmIdleSwayAmpDefault)
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

// normalizeNonNegativeRange 是"0 有合法含义"的数值参数（静止摆动频率 / 幅度）
// 的钳位规则。
//
// 它和 normalizePositiveRange 只差一点：0 不回落默认值，而是原样保留 ——
// 这是管理员"关掉轻晃"的明确意图。负数与 NaN 仍然回落默认，因为负数在这里
// 没有物理意义（不是"更慢/更小"）。
func normalizeNonNegativeRange(v, min, max, fallback float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	if v < 0 {
		return fallback
	}
	return math.Min(max, math.Max(min, v))
}

// normalizeLineWidth 是描边起始宽度的钳位规则。
//
// 除了"跟其它正数参数一样"的越界钳位 / 缺省回落，它还多做一件事：
// 保留一位小数。线宽是唯一一个"小数位有实际意义"的长度参数（2.5 与 2.56
// 在 1px 级的细线上肉眼没差别），把库里的值收成 0.1 的整数倍，可以避免
// 后台输入框回填出一串长浮点。
func normalizeLineWidth(v float64) float64 {
	v = normalizePositiveRange(v, CharmLineWidthMin, CharmLineWidthMax, CharmLineWidthDefault)
	return math.Round(v*10) / 10
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
