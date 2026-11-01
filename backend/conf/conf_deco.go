// 装饰配置（第 8 个运行时配置段）
//
// 只放"纯装饰"参数：它们不影响任何业务链路，改错了最多难看，不会让站点坏掉。
// 开这个段而不是把挂件参数塞进 site 段，是因为装饰会越来越多（背景、边框），
// 而每开一个新段都要动一遍段注册的八个点 —— 第 8 段一次付清这笔成本。
//
// 本票只落光标挂件的子块 charm；将来加别的装饰不用再开第 9 段。
package conf

import (
	"encoding/json"
	"math"
)

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
	// 它让组合式退回"静止即停帧"（见票 #42 的实施裁决）。
	CharmIdleSwayAmpMin = 0.0
	// CharmIdleSwayAmpMax 是静止摆动幅度上限（px/s²）。
	CharmIdleSwayAmpMax = 40.0
	// CharmIdleSwayAmpDefault 是默认静止摆动幅度。
	CharmIdleSwayAmpDefault = 20.0

	// CharmFpsCapMin 是帧率上限下限（fps）。取 30 而不是 20 是有依据的：
	// 一帧 33.3ms 对 0.05s 的累加器上限还留 16.7ms 余量，而 20 档一帧恰好
	// 50ms、余量为零 —— 任何迟到都会被截断丢掉，那会让绳子**慢放**。
	CharmFpsCapMin = 30.0
	// CharmFpsCapMax 是帧率上限上限（fps）：再高在现实显示器上没有意义，
	// 等价于"每帧"（160 档在 60Hz 屏上就等于不限）。
	CharmFpsCapMax = 160.0
	// CharmFpsCapDefault 是默认帧率上限：与 60Hz 屏对齐（每帧一次）。
	CharmFpsCapDefault = 60.0

	// CharmPointerOffsetXMin 是"指针端偏移"水平分量的下限，单位 px。
	// **负数在这里是合法值**：它表示指针端挂在光标左侧（可往左上偏）。
	CharmPointerOffsetXMin = -24.0
	// CharmPointerOffsetXMax 是水平偏移的上限（px）：再大指针端与光标热点就明显脱节，
	// 看起来像"绳子断了"（ADR-0018 代价段）。
	CharmPointerOffsetXMax = 24.0
	// CharmPointerOffsetXDefault 是默认水平偏移（px）：指针端从光标右侧 3px 处悬挂。
	CharmPointerOffsetXDefault = 3.0

	// CharmPointerOffsetYMin 是垂直分量的下限（px，视口坐标系里向下为正）。
	// 负数表示往上偏。
	CharmPointerOffsetYMin = -24.0
	// CharmPointerOffsetYMax 是垂直偏移的上限（px）。
	CharmPointerOffsetYMax = 24.0
	// CharmPointerOffsetYDefault 是默认垂直偏移（px）：指针端从光标下方 3px 处悬挂。
	CharmPointerOffsetYDefault = 3.0
)

// Deco 是装饰配置段。
type Deco struct {
	// Charm 是光标挂件的参数子块。
	Charm Charm `yaml:"charm" json:"charm"`
}

// Charm 是光标挂件的参数。
//
// 总开关 + 十一个数值参数。票 #41 接入挂绳长度、节点数、重力、阻尼；
// 票 #42 接入描边宽度、静止摆动频率与幅度（弧垂由"绳长 vs 实际距离"产生，
// 没有单独的"sag"参数）；票 #74 接入帧率上限（自适应降速车道已删除，
// 只剩一条动画帧调度，见 ADR-0018）；票 #75 接入指针端偏移 X / Y
// （指针端节点 = 指针热点 + 偏移，渲染不再单独偏一次）。
type Charm struct {
	// Enable 是总开关。默认开着：功能上线后读者立刻就能看到。
	Enable bool `yaml:"enable" json:"enable"`
	// LogoSize 是站标显示尺寸，单位 px。
	LogoSize float64 `yaml:"logo_size" json:"logo_size"`
	// RopeLength 是挂绳长度（松垂阈值），单位 px。
	RopeLength float64 `yaml:"rope_length" json:"rope_length"`
	// Segments 是 Verlet 链的节点数（节），整数计数。
	//
	// 用 float64 而不是 int 是有意的：写入路径"永不报错"的承诺必须对数字成立。
	// 用 int 时 `{"segments":10.6}` 会在 JSON 绑定时直接 400，管理员根本到不了
	// Normalize；Normalize 里再做四舍五入，10.6 节就按 11 节落库。
	Segments float64 `yaml:"segments" json:"segments"`
	// LineWidth 是描边起始宽度，单位 px（收尾宽度固定为它的 0.6 倍）。
	LineWidth float64 `yaml:"line_width" json:"line_width"`
	// Gravity 是重力加速度，单位 px/s²。
	Gravity float64 `yaml:"gravity" json:"gravity"`
	// Damping 是每个固定步长后保留的速度比例（无量纲，必须小于 1）。
	Damping float64 `yaml:"damping" json:"damping"`
	// IdleSwayHz 是静止摆动的频率，单位 Hz。0 表示关掉轻晃。
	IdleSwayHz float64 `yaml:"idle_sway_hz" json:"idle_sway_hz"`
	// IdleSwayAmp 是静止摆动的加速度幅度，单位 px/s²，作用在自由端：
	// 横向加速度按节点序号加权（指针端 0、尾端 1），不是每个节点同一份。
	// 0 表示关掉轻晃（读者站据此退回静止即停帧）；频率为 0 同样视为关掉。
	IdleSwayAmp float64 `yaml:"idle_sway_amp" json:"idle_sway_amp"`
	// FpsCap 是最高渲染帧率，单位 fps，整数。
	//
	// 语义是**上限**而不是保证：高于显示器刷新率时等价于"每帧"，高刷屏上的
	// 实际帧率落在刷新率的整数分之一（144Hz + 90 档 = 72fps）。它只决定
	// "每秒画几次"，不影响绳子运动速度 —— 物理仍按固定步长按真实时间推进。
	FpsCap float64 `yaml:"fps_cap" json:"fps_cap"`
	// PointerOffsetX 是"指针端偏移"的水平分量，单位 CSS px（与 clientX 同一视口
	// 坐标系，不随 DPI 缩放）。默认 3，区间 -24–24，**允许负值**（往左偏）。
	//
	// 语义是"指针端节点 = 指针热点 + (X, Y)"：挂绳的物理与渲染都从换算后的
	// 头节点出发，渲染层不得再单独偏一次（ADR-0018 决策 8 / 约束 7）。
	PointerOffsetX float64 `yaml:"pointer_offset_x" json:"pointer_offset_x"`
	// PointerOffsetY 是指针端偏移的垂直分量，单位 CSS px（向下为正）。
	PointerOffsetY float64 `yaml:"pointer_offset_y" json:"pointer_offset_y"`
}

// charmDecode 是 Charm 的 JSON 解码中间层：数值字段用 *float64、开关用 *bool，
// 才能把"JSON 里没给这个键（或给了 null）"与"给了 0 / false"分开。
//
// 只在 UnmarshalJSON 里用；yaml 路径不经过它（见 UnmarshalJSON 的边界说明）。
type charmDecode struct {
	Enable         *bool    `json:"enable"`
	LogoSize       *float64 `json:"logo_size"`
	RopeLength     *float64 `json:"rope_length"`
	Segments       *float64 `json:"segments"`
	LineWidth      *float64 `json:"line_width"`
	Gravity        *float64 `json:"gravity"`
	Damping        *float64 `json:"damping"`
	IdleSwayHz     *float64 `json:"idle_sway_hz"`
	IdleSwayAmp    *float64 `json:"idle_sway_amp"`
	FpsCap         *float64 `json:"fps_cap"`
	PointerOffsetX *float64 `json:"pointer_offset_x"`
	PointerOffsetY *float64 `json:"pointer_offset_y"`
}

// UnmarshalJSON 让 JSON 的 null / 缺键回落到默认值（票 #77）。
//
// 为什么必须在这一层处理：float64 无法区分"没给这个键"与"给了 0" ——
// encoding/json 把 null 与缺键都解成 0，而 0 在轻晃两项与两轴指针端偏移上
// 是**合法语义**（关掉轻晃 / 指针端压在热点上），Normalize 不能把它们一律
// 回落默认值。先解码成指针再落值，两件事就分开了：nil → 默认值，
// 非 nil → 原值（含 0）；越界与取整仍由 Normalize 负责。
//
// 这是行为变化：body 里缺 enable 过去会落成 bool 零值 false，现在与 yaml
// 默认种子一致，视为开启（true）；显式 false 仍然是 false。
//
// 边界：yaml 种子路径（site_default_settings.yaml，经 envyaml 解码）**不走**
// UnmarshalJSON。所以这里不实现 UnmarshalYAML —— 种子文件里没有 null，缺键时
// yaml 会落成 Go 零值，随后由 Normalize 统一处理（与旧行为一致）。
func (c *Charm) UnmarshalJSON(data []byte) error {
	var decoded charmDecode
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = Charm{
		Enable:         charmBoolOrDefault(decoded.Enable, true),
		LogoSize:       charmFloatOrDefault(decoded.LogoSize, CharmLogoSizeDefault),
		RopeLength:     charmFloatOrDefault(decoded.RopeLength, CharmRopeLengthDefault),
		Segments:       charmFloatOrDefault(decoded.Segments, CharmSegmentsDefault),
		LineWidth:      charmFloatOrDefault(decoded.LineWidth, CharmLineWidthDefault),
		Gravity:        charmFloatOrDefault(decoded.Gravity, CharmGravityDefault),
		Damping:        charmFloatOrDefault(decoded.Damping, CharmDampingDefault),
		IdleSwayHz:     charmFloatOrDefault(decoded.IdleSwayHz, CharmIdleSwayHzDefault),
		IdleSwayAmp:    charmFloatOrDefault(decoded.IdleSwayAmp, CharmIdleSwayAmpDefault),
		FpsCap:         charmFloatOrDefault(decoded.FpsCap, CharmFpsCapDefault),
		PointerOffsetX: charmFloatOrDefault(decoded.PointerOffsetX, CharmPointerOffsetXDefault),
		PointerOffsetY: charmFloatOrDefault(decoded.PointerOffsetY, CharmPointerOffsetYDefault),
	}
	return nil
}

// charmFloatOrDefault 把指针解码出来的数值落回值：nil（缺键 / null）用默认值，
// 非 nil 原样返回 —— 显式 0 是管理员"关掉轻晃 / 压在热点上"的明确意图。
func charmFloatOrDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

// charmBoolOrDefault 与 charmFloatOrDefault 同一条规则，用于"缺省即开启"的总开关。
func charmBoolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// Normalize 把装饰配置收敛进合法范围，永不报错。
//
// 规则按失效模式分四类（数值参数共用，具体参数只是选一条）：
//   - 非有限值（NaN / ±Inf）一律回落默认值：它们序列化进 JSON 会直接失败；
//   - 非正值回落默认值：0 是 Go 的零值（= 字段没填），负值是明显的手滑，
//     两者都不是"我想调到最小"的意图，钳成下限会把"没填"变成"填了最小值"；
//   - 0 / 负数都是合法取值的区间（指针端偏移）：只夹到 [min, max]；
//   - 其余越界值钳到边界：保证库里永远存不下发散值。
//
// 计数类另有约束：segments 先四舍五入再钳到边界，fps_cap 与指针端偏移先夹边界
// 再取整 —— 顺序反了会把越界值取整又放大一档。
//
// 不返回 error 是有意的：管理员改一半被 400 打回只会让他猜哪里错了。
func (d Deco) Normalize() Deco {
	d.Charm.LogoSize = normalizePositiveRange(d.Charm.LogoSize, CharmLogoSizeMin, CharmLogoSizeMax, CharmLogoSizeDefault)
	d.Charm.RopeLength = normalizePositiveRange(d.Charm.RopeLength, CharmRopeLengthMin, CharmRopeLengthMax, CharmRopeLengthDefault)
	d.Charm.Gravity = normalizePositiveRange(d.Charm.Gravity, CharmGravityMin, CharmGravityMax, CharmGravityDefault)
	d.Charm.Damping = normalizePositiveRange(d.Charm.Damping, CharmDampingMin, CharmDampingMax, CharmDampingDefault)
	d.Charm.LineWidth = normalizeLineWidth(d.Charm.LineWidth)
	// 轻晃的两个参数把 0 当作"关掉轻晃"的合法意图（裁决要求 amp=0 能退回停帧），
	// 所以它们走"非负区间"规则而不是"正数"规则。
	d.Charm.IdleSwayHz = normalizeNonNegativeRange(d.Charm.IdleSwayHz, CharmIdleSwayHzMin, CharmIdleSwayHzMax, CharmIdleSwayHzDefault)
	d.Charm.IdleSwayAmp = normalizeNonNegativeRange(d.Charm.IdleSwayAmp, CharmIdleSwayAmpMin, CharmIdleSwayAmpMax, CharmIdleSwayAmpDefault)
	d.Charm.Segments = normalizeSegments(d.Charm.Segments)
	// 帧率上限是计数（整数 fps）：先夹边界再取整，规则见 normalizeFpsCap。
	d.Charm.FpsCap = normalizeFpsCap(d.Charm.FpsCap)
	// 指针端偏移允许负数，只能走"允许负数的区间"规则（见 normalizeSignedRange）；
	// 用 normalizePositiveRange / normalizeNonNegativeRange 都会把负偏移吃掉。
	d.Charm.PointerOffsetX = normalizePointerOffset(d.Charm.PointerOffsetX, CharmPointerOffsetXMin, CharmPointerOffsetXMax, CharmPointerOffsetXDefault)
	d.Charm.PointerOffsetY = normalizePointerOffset(d.Charm.PointerOffsetY, CharmPointerOffsetYMin, CharmPointerOffsetYMax, CharmPointerOffsetYDefault)
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

// normalizeSignedRange 是"允许负数"的数值参数（指针端偏移 X / Y）的钳位规则。
//
// 它和另外两条的区别只有"0 与负数怎么算"：
//   - normalizePositiveRange 把 <= 0 一律回落默认值；
//   - normalizeNonNegativeRange 保留 0，但把负数夹回 min；
//   - 这里 0 与负数都是**普通取值**，只夹到 [min, max]。
//
// 指针端偏移的 min 本来就是负数（-24），用前两条里的任何一条都会让"往左上偏"
// 被静默吃掉（ADR-0018 决策 9 / 约束 8）。NaN / ±Inf 仍回落默认：它们序列化进
// JSON 会直接失败。0 是合法值：指针端正好压在光标热点上，视觉上等于关掉偏移。
func normalizeSignedRange(v, min, max, fallback float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return math.Min(max, math.Max(min, v))
}

// normalizePointerOffset 是指针端偏移单轴的钳位规则：先夹到 -24–24，再取整。
//
// 顺序不能反（与 normalizeFpsCap 同一条口径）：24.6 要先被夹到 24 再取整，
// 反过来会得到 25。`math.Round` 对负数按"远离 0"取整（-2.5 → -3），读者站 /
// 后台用同一口径（`Math.sign(v) * Math.round(Math.abs(v))`）—— 直接写
// `Math.round` 会在负半值上与 Go 分叉，跨端共享夹具抓着这一点。
func normalizePointerOffset(v, min, max, fallback float64) float64 {
	v = normalizeSignedRange(v, min, max, fallback)
	return math.Round(v)
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

// normalizeSegments 是节点数的钳位规则：计数没有整数边界输入，
// 只有"没填（0 或负数）"与"越界 / 小数"三种情况。
// 与读者站 / 后台的 `Math.round` 保持一致：先四舍五入再夹到 4–24。
func normalizeSegments(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return CharmSegmentsDefault
	}
	if v <= 0 {
		return CharmSegmentsDefault
	}
	rounded := math.Round(v)
	return math.Min(CharmSegmentsMax, math.Max(CharmSegmentsMin, rounded))
}

// normalizeFpsCap 是帧率上限的钳位规则：先夹到 30–160、再取整成整数。
//
// 顺序不能反：160.6 要先被夹到 160 再取整，反过来会得到 161（越界值被取整
// 又放大了一档）。与读者站 / 后台的 `Math.round(clamp(...))` 同一条规则，
// 跨端共享夹具抓着这个顺序。
func normalizeFpsCap(v float64) float64 {
	v = normalizePositiveRange(v, CharmFpsCapMin, CharmFpsCapMax, CharmFpsCapDefault)
	return math.Round(v)
}
