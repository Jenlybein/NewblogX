package conf_test

import (
	"encoding/json"
	"math"
	"os"
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

// charmWith 返回一份"其余参数都在合法默认值上"的挂件配置。
// 物理参数表只改被测的那一项，才能证明 Normalize 不会把别的字段顺手改坏。
func charmWith(mutate func(*conf.Charm)) conf.Charm {
	c := conf.Charm{
		Enable:      true,
		LogoSize:    conf.CharmLogoSizeDefault,
		RopeLength:  conf.CharmRopeLengthDefault,
		Segments:    conf.CharmSegmentsDefault,
		LineWidth:   conf.CharmLineWidthDefault,
		Gravity:     conf.CharmGravityDefault,
		Damping:     conf.CharmDampingDefault,
		IdleSwayHz:  conf.CharmIdleSwayHzDefault,
		IdleSwayAmp: conf.CharmIdleSwayAmpDefault,
		FpsCap:      conf.CharmFpsCapDefault,
		// 指针端偏移（票 #75）：整体比较时也必须在默认值上，否则"其余参数没被
		// 顺手改坏"这条断言会被两边的零值共同掩盖。
		PointerOffsetX: conf.CharmPointerOffsetXDefault,
		PointerOffsetY: conf.CharmPointerOffsetYDefault,
	}
	mutate(&c)
	return c
}

// TestDecoNormalize_帧率上限钳位 钉住票 #74 的帧率上限：30–160 fps，默认 60，整数。
//
// 为什么下限是 30 而不是 20：物理常量一律不动（1/120s 固定步长、0.05s 累加器
// 上限），30 档一帧 33.3ms 还留 16.7ms 余量；20 档一帧恰好 50ms、余量为零，
// 任何迟到都会被截断丢掉，绳子会慢放。这条规则由上限本身守住。
func TestDecoNormalize_帧率上限钳位(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"远超上限钳到 160", 1000, conf.CharmFpsCapMax},
		{"略高于上限也钳回边界", 160.1, conf.CharmFpsCapMax},
		{"略低于下限钳到 30", 29.9, conf.CharmFpsCapMin},
		{"下限本身不变（边界值不漂移）", 30, 30},
		{"上限本身不变（边界值不漂移）", 160, 160},
		{"区间内的值原样保留", 90, 90},
		{"默认值 60 原样保留", 60, 60},
		{"小数先夹边界再取整（90.4→90）", 90.4, 90},
		{"小数先夹边界再取整（90.6→91）", 90.6, 91},
		{"缺省（零值）补默认值 60", 0, conf.CharmFpsCapDefault},
		{"负数回落默认而不是钳到 30", -30, conf.CharmFpsCapDefault},
		{"NaN 回落默认", math.NaN(), conf.CharmFpsCapDefault},
		{"+Inf 回落默认", math.Inf(1), conf.CharmFpsCapDefault},
		{"-Inf 回落默认", math.Inf(-1), conf.CharmFpsCapDefault},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := (conf.Deco{Charm: conf.Charm{FpsCap: c.in}}).Normalize().Charm.FpsCap
			if got != c.want {
				t.Fatalf("FpsCap=%v 归一化后应为 %v，实际 %v", c.in, c.want, got)
			}
		})
	}
}

// TestDecoNormalize_指针端偏移钳位 钉住票 #75 的指针端偏移：每轴 -24–24，
// 默认 3，单位 CSS px（整数），**允许负值**（可以往左上偏）。
//
// 为什么不能复用现成的两条规则：`normalizePositiveRange` 把 <= 0 一律回落默认
// （0 与负偏移都被吃掉），`normalizeNonNegativeRange` 把负值夹回下限 —— 而这里的
// 下限本来就是负数，等于"负偏移被静默改成 -24"。ADR-0018 决策 9 / 约束 8 因此
// 要求第四条规则 `normalizeSignedRange`：夹 [min,max]、NaN/±Inf 回落默认、
// 0 与负数都是普通值、只是最后取整成整数。
func TestDecoNormalize_指针端偏移钳位(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"远超上限钳到 24", 100, conf.CharmPointerOffsetXMax},
		{"略超上限也钳回边界", 24.4, conf.CharmPointerOffsetXMax},
		{"上限本身不漂移", 24, 24},
		{"区间内的正值原样保留", 12, 12},
		{"默认值 3 原样保留", 3, 3},
		{"0 是普通值，不回落默认", 0, 0},
		{"负值原样保留（往左上偏）", -6, -6},
		{"下限本身不漂移", -24, -24},
		{"略低于下限钳到 -24", -24.4, conf.CharmPointerOffsetXMin},
		{"远超下限钳到 -24", -100, conf.CharmPointerOffsetXMin},
		{"小数取整（3.7→4）", 3.7, 4},
		{"负数小数取整（-3.7→-4）", -3.7, -4},
		{"负数半值向远离 0 的方向取整（-2.5→-3）", -2.5, -3},
		{"NaN 回落默认", math.NaN(), conf.CharmPointerOffsetXDefault},
		{"+Inf 回落默认", math.Inf(1), conf.CharmPointerOffsetXDefault},
		{"-Inf 回落默认", math.Inf(-1), conf.CharmPointerOffsetXDefault},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// X / Y 两轴的区间与默认值必须完全对称：同一张表同时喂给两个字段。
			normalized := conf.Deco{Charm: conf.Charm{
				PointerOffsetX: c.in,
				PointerOffsetY: c.in,
			}}.Normalize().Charm
			if normalized.PointerOffsetX != c.want {
				t.Fatalf("PointerOffsetX=%v 归一化后应为 %v，实际 %v", c.in, c.want, normalized.PointerOffsetX)
			}
			if normalized.PointerOffsetY != c.want {
				t.Fatalf("PointerOffsetY=%v 归一化后应为 %v，实际 %v", c.in, c.want, normalized.PointerOffsetY)
			}
		})
	}
}

// TestDecoNormalize_指针端偏移两轴互不影响 证明 X 与 Y 是两条独立的通道：
// 只改一轴时另一轴必须留在原处（复制粘贴把 Y 写成 X 会在这里红）。
func TestDecoNormalize_指针端偏移两轴互不影响(t *testing.T) {
	got := conf.Deco{Charm: conf.Charm{PointerOffsetX: -6, PointerOffsetY: 12}}.Normalize().Charm

	if got.PointerOffsetX != -6 {
		t.Fatalf("PointerOffsetX 应保留 -6，实际 %v", got.PointerOffsetX)
	}
	if got.PointerOffsetY != 12 {
		t.Fatalf("PointerOffsetY 应保留 12，实际 %v", got.PointerOffsetY)
	}
}

// 物理参数（挂绳长度 / 节点数 / 重力 / 阻尼）与站标尺寸共用同一条
// "越界钳到边界、缺省与非法回落默认、永不报错"的规则；但它们是四种不同的
// 量纲，所以各自一张表 —— 把阻尼的 0.94–0.995 错写成 0.94–1 会让摆动越甩越大，
// 而这条错误在跨端手调时几乎看不出来，只能靠边界用例钉住。
func TestDecoNormalize_物理参数钳位(t *testing.T) {
	cases := []struct {
		name string
		in   conf.Charm
		want conf.Charm
	}{
		// 挂绳长度：40–240 px，默认 120。
		{"挂绳长度远超上限钳到 240",
			charmWith(func(c *conf.Charm) { c.RopeLength = 4000 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthMax })},
		{"挂绳长度略超上限也钳回边界",
			charmWith(func(c *conf.Charm) { c.RopeLength = 241 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthMax })},
		{"挂绳长度略低于下限钳到 40",
			charmWith(func(c *conf.Charm) { c.RopeLength = 10 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthMin })},
		{"挂绳长度边界值不漂移",
			charmWith(func(c *conf.Charm) { c.RopeLength = 40 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = 40 })},
		{"挂绳长度区间内原样保留",
			charmWith(func(c *conf.Charm) { c.RopeLength = 200 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = 200 })},
		{"挂绳长度缺省（零值）补默认 120",
			charmWith(func(c *conf.Charm) { c.RopeLength = 0 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthDefault })},
		{"挂绳长度负数回落默认而不是钳到 40",
			charmWith(func(c *conf.Charm) { c.RopeLength = -5 }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthDefault })},
		{"挂绳长度 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.RopeLength = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthDefault })},
		{"挂绳长度 +Inf 回落默认",
			charmWith(func(c *conf.Charm) { c.RopeLength = math.Inf(1) }),
			charmWith(func(c *conf.Charm) { c.RopeLength = conf.CharmRopeLengthDefault })},

		// 节点数：4–24 节，默认 10；是计数，钳位后必须是整数。
		{"节点数远超上限钳到 24",
			charmWith(func(c *conf.Charm) { c.Segments = 100 }),
			charmWith(func(c *conf.Charm) { c.Segments = conf.CharmSegmentsMax })},
		{"节点数略低于下限钳到 4",
			charmWith(func(c *conf.Charm) { c.Segments = 1 }),
			charmWith(func(c *conf.Charm) { c.Segments = conf.CharmSegmentsMin })},
		{"节点数边界值不漂移",
			charmWith(func(c *conf.Charm) { c.Segments = 4 }),
			charmWith(func(c *conf.Charm) { c.Segments = 4 })},
		{"节点数区间内原样保留",
			charmWith(func(c *conf.Charm) { c.Segments = 16 }),
			charmWith(func(c *conf.Charm) { c.Segments = 16 })},
		{"节点数缺省（零值）补默认 10",
			charmWith(func(c *conf.Charm) { c.Segments = 0 }),
			charmWith(func(c *conf.Charm) { c.Segments = conf.CharmSegmentsDefault })},
		{"节点数负数回落默认而不是钳到 4",
			charmWith(func(c *conf.Charm) { c.Segments = -3 }),
			charmWith(func(c *conf.Charm) { c.Segments = conf.CharmSegmentsDefault })},

		// 重力：400–4000 px/s²，默认 1600。
		{"重力远超上限钳到 4000",
			charmWith(func(c *conf.Charm) { c.Gravity = 100000 }),
			charmWith(func(c *conf.Charm) { c.Gravity = conf.CharmGravityMax })},
		{"重力略低于下限钳到 400",
			charmWith(func(c *conf.Charm) { c.Gravity = 100 }),
			charmWith(func(c *conf.Charm) { c.Gravity = conf.CharmGravityMin })},
		{"重力区间内原样保留",
			charmWith(func(c *conf.Charm) { c.Gravity = 1600 }),
			charmWith(func(c *conf.Charm) { c.Gravity = 1600 })},
		{"重力缺省（零值）补默认 1600",
			charmWith(func(c *conf.Charm) { c.Gravity = 0 }),
			charmWith(func(c *conf.Charm) { c.Gravity = conf.CharmGravityDefault })},
		{"重力负数回落默认而不是钳到 400",
			charmWith(func(c *conf.Charm) { c.Gravity = -1 }),
			charmWith(func(c *conf.Charm) { c.Gravity = conf.CharmGravityDefault })},
		{"重力 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.Gravity = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.Gravity = conf.CharmGravityDefault })},

		// 阻尼：0.94–0.995（每步速度保持系数），默认 0.99。
		{"阻尼超过 1 会被钳到 0.995（否则摆动越甩越大）",
			charmWith(func(c *conf.Charm) { c.Damping = 2 }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingMax })},
		{"阻尼略高于上限也钳回边界",
			charmWith(func(c *conf.Charm) { c.Damping = 0.999 }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingMax })},
		{"阻尼低于下限钳到 0.94",
			charmWith(func(c *conf.Charm) { c.Damping = 0.5 }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingMin })},
		{"阻尼边界值不漂移",
			charmWith(func(c *conf.Charm) { c.Damping = 0.94 }),
			charmWith(func(c *conf.Charm) { c.Damping = 0.94 })},
		{"阻尼区间内原样保留",
			charmWith(func(c *conf.Charm) { c.Damping = 0.99 }),
			charmWith(func(c *conf.Charm) { c.Damping = 0.99 })},
		{"阻尼缺省（零值）补默认 0.99",
			charmWith(func(c *conf.Charm) { c.Damping = 0 }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingDefault })},
		{"阻尼负数回落默认而不是钳到 0.94",
			charmWith(func(c *conf.Charm) { c.Damping = -0.2 }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingDefault })},
		{"阻尼 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.Damping = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.Damping = conf.CharmDampingDefault })},

		// 描边起始宽度：1–6 px（小数），默认 2.5；结果保留一位小数。
		{"描边宽度远超上限钳到 6",
			charmWith(func(c *conf.Charm) { c.LineWidth = 4000 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = conf.CharmLineWidthMax })},
		{"描边宽度略低于下限钳到 1",
			charmWith(func(c *conf.Charm) { c.LineWidth = 0.2 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = conf.CharmLineWidthMin })},
		{"描边宽度四舍五入到一位小数（2.54→2.5）",
			charmWith(func(c *conf.Charm) { c.LineWidth = 2.54 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = 2.5 })},
		{"描边宽度四舍五入到一位小数（2.56→2.6）",
			charmWith(func(c *conf.Charm) { c.LineWidth = 2.56 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = 2.6 })},
		{"描边宽度缺省（零值）补默认 2.5",
			charmWith(func(c *conf.Charm) { c.LineWidth = 0 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = conf.CharmLineWidthDefault })},
		{"描边宽度负数回落默认",
			charmWith(func(c *conf.Charm) { c.LineWidth = -1 }),
			charmWith(func(c *conf.Charm) { c.LineWidth = conf.CharmLineWidthDefault })},
		{"描边宽度 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.LineWidth = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.LineWidth = conf.CharmLineWidthDefault })},

		// 静止摆动频率：0–1.5 Hz，默认 0.8。**0 是合法值**：它是"关掉轻晃"的
		// 明确意图（裁决要求 amp=0 能退回零帧），所以不能按"缺省"回落默认。
		{"静止摆动频率 0 是合法的（关掉轻晃，不回落默认）",
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = 0 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = 0 })},
		{"静止摆动频率超上限钳到 1.5",
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = 9 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = conf.CharmIdleSwayHzMax })},
		{"静止摆动频率负数回落默认（负数不是合法意图）",
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = -1 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = conf.CharmIdleSwayHzDefault })},
		{"静止摆动频率 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = conf.CharmIdleSwayHzDefault })},
		{"静止摆动频率区间内原样保留",
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = 0.8 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayHz = 0.8 })},

		// 静止摆动幅度：0–40 px/s²，默认 20。同样是"0 合法"。
		{"静止摆动幅度 0 是合法的（零帧停摆，不回落默认）",
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = 0 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = 0 })},
		{"静止摆动幅度超上限钳到 40",
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = 400 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = conf.CharmIdleSwayAmpMax })},
		{"静止摆动幅度负数回落默认",
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = -5 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = conf.CharmIdleSwayAmpDefault })},
		{"静止摆动幅度 NaN 回落默认",
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = math.NaN() }),
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = conf.CharmIdleSwayAmpDefault })},
		{"静止摆动幅度区间内原样保留",
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = 20 }),
			charmWith(func(c *conf.Charm) { c.IdleSwayAmp = 20 })},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := conf.Deco{Charm: c.in}.Normalize().Charm
			if got != c.want {
				t.Fatalf("输入 %+v 归一化后应为 %+v，实际 %+v", c.in, c.want, got)
			}
		})
	}
}

func TestDecoNormalize_幂等(t *testing.T) {
	// 幂等是"读者站与后台再钳一次也是同一道防线"的前提：
	// 第二次归一化必须与第一次结果相同，否则两端会来回抖动。
	once := conf.Deco{Charm: conf.Charm{
		LogoSize:   9,
		RopeLength: 4000,
		Segments:   100,
		Gravity:    100000,
		Damping:    2,
	}}.Normalize()
	twice := once.Normalize()
	if once != twice {
		t.Fatalf("Normalize 应幂等，第一次 %+v，第二次 %+v", once, twice)
	}
}

// decoNormalizationCase 是共享用例表的一行；三套实现（后端 Normalize、
// 读者站、后台）都读同一份 testdata，任何一端改了取整或回落规则都会红。
type decoNormalizationCase struct {
	Parameter string          `json:"parameter"`
	Input     json.RawMessage `json:"input"`
	Expected  float64         `json:"expected"`
	Reason    string          `json:"reason"`
}

// decoCaseInput 解析用例里的 input：普通数字直接取值，NaN/Infinity 用字符串
// 表达（JSON 没有 NaN 字面量）。
func decoCaseInput(t *testing.T, raw json.RawMessage) float64 {
	t.Helper()
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil {
		t.Fatalf("无法解析用例 input %s: %v", raw, err)
	}
	switch token {
	case "NaN":
		return math.NaN()
	case "Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	t.Fatalf("不认识的字符串输入 %q（只支持 NaN / Infinity / -Infinity）", token)
	return 0
}

// TestDecoNormalize_共享归一化用例表 读 `testdata/deco-charm-normalization.json`。
//
// 为什么值得测：三端各自单测只能证明"自己和自己一致"；这张表是同一组输入
// 在三端都必须给出的同一组输出。后端曾用 int 存 segments（10.6 直接 400），
// 与读者站 / 后台的 Math.round 不一致，这里会先红。
func TestDecoNormalize_共享归一化用例表(t *testing.T) {
	raw, err := os.ReadFile("testdata/deco-charm-normalization.json")
	if err != nil {
		t.Fatalf("读不到共享归一化用例表: %v", err)
	}
	var fixture struct {
		Cases []decoNormalizationCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("共享归一化用例表不是合法 JSON: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("共享归一化用例表是空的：三端夹具失效")
	}

	for _, c := range fixture.Cases {
		c := c
		t.Run(c.Parameter+"/"+c.Reason, func(t *testing.T) {
			input := decoCaseInput(t, c.Input)
			charm := conf.Charm{}
			switch c.Parameter {
			case "logo_size":
				charm.LogoSize = input
			case "rope_length":
				charm.RopeLength = input
			case "segments":
				charm.Segments = input
			case "line_width":
				charm.LineWidth = input
			case "gravity":
				charm.Gravity = input
			case "damping":
				charm.Damping = input
			case "idle_sway_hz":
				charm.IdleSwayHz = input
			case "idle_sway_amp":
				charm.IdleSwayAmp = input
			case "fps_cap":
				charm.FpsCap = input
			case "pointer_offset_x":
				charm.PointerOffsetX = input
			case "pointer_offset_y":
				charm.PointerOffsetY = input
			default:
				t.Fatalf("共享用例表出现未知参数 %q", c.Parameter)
			}
			got := conf.Deco{Charm: charm}.Normalize().Charm
			var actual float64
			switch c.Parameter {
			case "logo_size":
				actual = got.LogoSize
			case "rope_length":
				actual = got.RopeLength
			case "segments":
				actual = got.Segments
			case "line_width":
				actual = got.LineWidth
			case "gravity":
				actual = got.Gravity
			case "damping":
				actual = got.Damping
			case "idle_sway_hz":
				actual = got.IdleSwayHz
			case "idle_sway_amp":
				actual = got.IdleSwayAmp
			case "fps_cap":
				actual = got.FpsCap
			case "pointer_offset_x":
				actual = got.PointerOffsetX
			case "pointer_offset_y":
				actual = got.PointerOffsetY
			}
			if actual != c.Expected {
				t.Fatalf("%s 输入 %s 归一化后应为 %v，实际 %v", c.Parameter, c.Input, c.Expected, actual)
			}
		})
	}
}
