import { describe, expect, it } from "vitest";

import {
  CHARM_DAMPING_DEFAULT,
  CHARM_DAMPING_MAX,
  CHARM_DAMPING_MIN,
  CHARM_FIXED_STEP_SECONDS,
  CHARM_GRAVITY_DEFAULT,
  CHARM_GRAVITY_MAX,
  CHARM_GRAVITY_MIN,
  CHARM_IDLE_FRAME_INTERVAL_MS,
  CHARM_IDLE_SWAY_AMP_DEFAULT,
  CHARM_IDLE_SWAY_AMP_MAX,
  CHARM_IDLE_SWAY_AMP_MIN,
  CHARM_IDLE_SWAY_HZ_DEFAULT,
  CHARM_IDLE_SWAY_HZ_MAX,
  CHARM_IDLE_SWAY_HZ_MIN,
  CHARM_LINE_WIDTH_DEFAULT,
  CHARM_LINE_WIDTH_MAX,
  CHARM_LINE_WIDTH_MIN,
  CHARM_LOGO_SIZE_DEFAULT,
  CHARM_LOGO_SIZE_MAX,
  CHARM_LOGO_SIZE_MIN,
  CHARM_MAX_FRAME_SECONDS,
  CHARM_MAX_SUBSTEPS,
  CHARM_ROPE_LENGTH_DEFAULT,
  CHARM_ROPE_LENGTH_MAX,
  CHARM_ROPE_LENGTH_MIN,
  CHARM_SEGMENTS_DEFAULT,
  CHARM_SEGMENTS_MAX,
  CHARM_SEGMENTS_MIN,
  CHARM_TAPER_RATIO,
  advanceCharmRope,
  charmLayerEnabled,
  charmRopeEnergy,
  charmRopePath,
  charmRopeSegmentLengths,
  charmRopeSettled,
  charmRopeTail,
  charmTetherGeometry,
  clampCharmDamping,
  clampCharmGravity,
  clampCharmIdleSwayAmp,
  clampCharmIdleSwayHz,
  clampCharmLineWidth,
  clampCharmLogoSize,
  clampCharmRopeLength,
  clampCharmSegments,
  createCharmRope,
  normalizeCharmRopeConfig,
  type CharmPoint,
  type CharmRopeConfig,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：挂件里所有"算错就看得见"的决定都在这个模块 ——
 * 门控、参数钳位、固定步长积分、弧垂与收尾描边。物理部分尤其不能断言
 * "某一帧的坐标"（那会把实现焊死在测试里），只能断言不变量：不 NaN、
 * 不发散、能量衰减、节点间距在容差内。积分器一旦依赖动画帧时序，
 * 120Hz 屏上的绳子就会比 60Hz 快一倍 —— 这是票面点名要防的真 bug。
 *
 * 票 #42 新增三条：收尾宽度恰好是起始的 0.6 倍（断言在几何里）、弧垂只由
 * 真实松弛（绳长 − 实际距离）产生（重力不参与）、轻晃是自由端的真实形变
 * 且必须能退回零帧。
 */

const ROPE: CharmRopeConfig = {
  ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT,
  segments: CHARM_SEGMENTS_DEFAULT,
  gravity: CHARM_GRAVITY_DEFAULT,
  damping: 0.99,
  lineWidthPx: CHARM_LINE_WIDTH_DEFAULT,
  idleSwayHz: 0,
  idleSwayAmp: 0,
};

/** 一次固定步长的帧间隔（积分器应当正好推进一个子步）。 */
const FRAME_AT_120HZ = CHARM_FIXED_STEP_SECONDS;

/** 全部节点都是有限值（NaN 进 SVG 会让整条路径静默消失）。 */
function allFinite(values: readonly number[]): boolean {
  return values.every((value) => Number.isFinite(value));
}

function nodeCoords(state: ReturnType<typeof createCharmRope>): number[] {
  return state.nodes.flatMap((node) => [node.x, node.y]);
}

/** 从初始状态出发跑一段确定的帧序列，用于"确定性"断言。 */
function runFixedSequence(): { nodes: Array<{ x: number; y: number }>; energy: number } {
  const state = createCharmRope({ x: 100, y: 100 }, ROPE);
  const heads = [
    { x: 140, y: 110 },
    { x: 80, y: 140 },
    { x: 130, y: 200 },
    { x: 100, y: 100 },
  ];
  for (const head of heads) {
    for (let frame = 0; frame < 20; frame += 1) {
      advanceCharmRope(state, 0.0125, head);
    }
  }
  return { nodes: state.nodes.map((node) => ({ x: node.x, y: node.y })), energy: charmRopeEnergy(state) };
}

/** 一条竖直的"几何链"：用于把弧垂 / 收尾宽度从物理里隔离出来断言。 */
function straightPoints(chordPx: number, count = 11): CharmPoint[] {
  return Array.from({ length: count }, (_, index) => ({
    x: 0,
    y: (chordPx * index) / (count - 1),
  }));
}

describe("光标挂件的参数常量", () => {
  it("挂绳长度 40–240 px，默认 120（与后端 conf_deco.go 同一组数）", () => {
    expect(CHARM_ROPE_LENGTH_MIN).toBe(40);
    expect(CHARM_ROPE_LENGTH_MAX).toBe(240);
    expect(CHARM_ROPE_LENGTH_DEFAULT).toBe(120);
  });

  it("节点数 4–24 节，默认 10（计数，不是像素）", () => {
    expect(CHARM_SEGMENTS_MIN).toBe(4);
    expect(CHARM_SEGMENTS_MAX).toBe(24);
    expect(CHARM_SEGMENTS_DEFAULT).toBe(10);
  });

  it("重力 400–4000 px/s²，默认 1600", () => {
    expect(CHARM_GRAVITY_MIN).toBe(400);
    expect(CHARM_GRAVITY_MAX).toBe(4000);
    expect(CHARM_GRAVITY_DEFAULT).toBe(1600);
  });

  it("阻尼 0.94–0.995，默认 0.99；上限必须小于 1（否则越甩越大）", () => {
    expect(CHARM_DAMPING_MIN).toBe(0.94);
    expect(CHARM_DAMPING_MAX).toBe(0.995);
    expect(CHARM_DAMPING_DEFAULT).toBe(0.99);
    expect(CHARM_DAMPING_MAX).toBeLessThan(1);
  });

  it("描边起始宽度 1–6 px，默认 2.5；收尾固定为起始的 0.6 倍（不进配置）", () => {
    expect(CHARM_LINE_WIDTH_MIN).toBe(1);
    expect(CHARM_LINE_WIDTH_MAX).toBe(6);
    expect(CHARM_LINE_WIDTH_DEFAULT).toBe(2.5);
    expect(CHARM_TAPER_RATIO).toBe(0.6);
  });

  it("静止摆动 0–1.5 Hz / 0–40 px/s²，默认 0.8 / 20；下限 0 是合法的「关掉轻晃」", () => {
    expect(CHARM_IDLE_SWAY_HZ_MIN).toBe(0);
    expect(CHARM_IDLE_SWAY_HZ_MAX).toBe(1.5);
    expect(CHARM_IDLE_SWAY_HZ_DEFAULT).toBe(0.8);
    expect(CHARM_IDLE_SWAY_AMP_MIN).toBe(0);
    expect(CHARM_IDLE_SWAY_AMP_MAX).toBe(40);
    expect(CHARM_IDLE_SWAY_AMP_DEFAULT).toBe(20);
  });

  it("怠速车道约 20fps（50ms 一帧），明显低于满速 60fps", () => {
    expect(CHARM_IDLE_FRAME_INTERVAL_MS).toBe(50);
    expect(1000 / CHARM_IDLE_FRAME_INTERVAL_MS).toBeLessThan(60);
  });

  it("固定步长是 1/120s、单帧最多 8 个子步、累加器上限 0.05s", () => {
    // 这三个数是"在 120Hz 屏上不快一倍"的全部依据，钉死它们。
    expect(CHARM_FIXED_STEP_SECONDS).toBeCloseTo(1 / 120, 12);
    expect(CHARM_MAX_SUBSTEPS).toBe(8);
    expect(CHARM_MAX_FRAME_SECONDS).toBe(0.05);
    // 0.05s 内最多只会有 6 个子步，子步上限是给未来改步长留的保险。
    expect(CHARM_MAX_FRAME_SECONDS / CHARM_FIXED_STEP_SECONDS).toBeLessThanOrEqual(CHARM_MAX_SUBSTEPS);
  });

  it("站标尺寸区间与默认值与后端一致（24–96，默认 48）", () => {
    expect(CHARM_LOGO_SIZE_MIN).toBe(24);
    expect(CHARM_LOGO_SIZE_MAX).toBe(96);
    expect(CHARM_LOGO_SIZE_DEFAULT).toBe(48);
  });
});

describe("clampCharmLogoSize", () => {
  it("越界钳到边界（不产生发散值）", () => {
    expect(clampCharmLogoSize(4000)).toBe(CHARM_LOGO_SIZE_MAX);
    expect(clampCharmLogoSize(10)).toBe(CHARM_LOGO_SIZE_MIN);
    expect(clampCharmLogoSize(96.5)).toBe(CHARM_LOGO_SIZE_MAX);
  });

  it("区间内的值原样保留，边界值不漂移", () => {
    expect(clampCharmLogoSize(60)).toBe(60);
    expect(clampCharmLogoSize(24)).toBe(24);
    expect(clampCharmLogoSize(96)).toBe(96);
  });

  it("缺省（0）与非法值（负数 / NaN / Infinity）回落默认值", () => {
    // 0 是"字段没填"的零值，不是"我想调到最小"的意图。
    expect(clampCharmLogoSize(0)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(-5)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(Number.NaN)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(Number.POSITIVE_INFINITY)).toBe(CHARM_LOGO_SIZE_DEFAULT);
  });
});

describe("挂绳物理参数钳位（读者站这最后一道防线）", () => {
  it("挂绳长度：越界钳到 40/240，零与非法回落 120", () => {
    expect(clampCharmRopeLength(4000)).toBe(CHARM_ROPE_LENGTH_MAX);
    expect(clampCharmRopeLength(10)).toBe(CHARM_ROPE_LENGTH_MIN);
    expect(clampCharmRopeLength(0)).toBe(CHARM_ROPE_LENGTH_DEFAULT);
    expect(clampCharmRopeLength(-5)).toBe(CHARM_ROPE_LENGTH_DEFAULT);
    expect(clampCharmRopeLength(Number.NaN)).toBe(CHARM_ROPE_LENGTH_DEFAULT);
    expect(clampCharmRopeLength(200)).toBe(200);
  });

  it("节点数：钳成整数，越界到 4/24，零与非法回落 10", () => {
    expect(clampCharmSegments(100)).toBe(CHARM_SEGMENTS_MAX);
    expect(clampCharmSegments(1)).toBe(CHARM_SEGMENTS_MIN);
    expect(clampCharmSegments(10.7)).toBe(11);
    expect(clampCharmSegments(0)).toBe(CHARM_SEGMENTS_DEFAULT);
    expect(clampCharmSegments(-3)).toBe(CHARM_SEGMENTS_DEFAULT);
    expect(clampCharmSegments(Number.NaN)).toBe(CHARM_SEGMENTS_DEFAULT);
  });

  it("重力：越界钳到 400/4000，零与非法回落 1600", () => {
    expect(clampCharmGravity(100000)).toBe(CHARM_GRAVITY_MAX);
    expect(clampCharmGravity(100)).toBe(CHARM_GRAVITY_MIN);
    expect(clampCharmGravity(0)).toBe(CHARM_GRAVITY_DEFAULT);
    expect(clampCharmGravity(-1)).toBe(CHARM_GRAVITY_DEFAULT);
    expect(clampCharmGravity(Number.NaN)).toBe(CHARM_GRAVITY_DEFAULT);
  });

  it("阻尼：钳到 0.94/0.995，零与非法回落 0.99（等于 1 会被钳回上限）", () => {
    expect(clampCharmDamping(2)).toBe(0.995);
    expect(clampCharmDamping(1)).toBe(0.995);
    expect(clampCharmDamping(0.5)).toBe(0.94);
    expect(clampCharmDamping(0)).toBe(0.99);
    expect(clampCharmDamping(Number.NaN)).toBe(0.99);
    expect(clampCharmDamping(0.97)).toBe(0.97);
  });
});

describe("票 #42 的三个新参数钳位", () => {
  it("描边宽度：钳到 1/6 并保留一位小数，零与非法回落 2.5", () => {
    expect(clampCharmLineWidth(10)).toBe(CHARM_LINE_WIDTH_MAX);
    expect(clampCharmLineWidth(0.2)).toBe(CHARM_LINE_WIDTH_MIN);
    expect(clampCharmLineWidth(2.54)).toBe(2.5);
    expect(clampCharmLineWidth(2.56)).toBe(2.6);
    expect(clampCharmLineWidth(0)).toBe(CHARM_LINE_WIDTH_DEFAULT);
    expect(clampCharmLineWidth(-1)).toBe(CHARM_LINE_WIDTH_DEFAULT);
    expect(clampCharmLineWidth(Number.NaN)).toBe(CHARM_LINE_WIDTH_DEFAULT);
  });

  it("静止摆动频率：0 原样保留（= 关掉轻晃），越界钳到 1.5，负数/NaN 回落 0.8", () => {
    expect(clampCharmIdleSwayHz(0)).toBe(0);
    expect(clampCharmIdleSwayHz(9)).toBe(CHARM_IDLE_SWAY_HZ_MAX);
    expect(clampCharmIdleSwayHz(0.8)).toBe(0.8);
    expect(clampCharmIdleSwayHz(-1)).toBe(CHARM_IDLE_SWAY_HZ_DEFAULT);
    expect(clampCharmIdleSwayHz(Number.NaN)).toBe(CHARM_IDLE_SWAY_HZ_DEFAULT);
  });

  it("静止摆动幅度：0 原样保留（= 零帧车道），越界钳到 40，负数/NaN 回落 20", () => {
    expect(clampCharmIdleSwayAmp(0)).toBe(0);
    expect(clampCharmIdleSwayAmp(400)).toBe(CHARM_IDLE_SWAY_AMP_MAX);
    expect(clampCharmIdleSwayAmp(20)).toBe(20);
    expect(clampCharmIdleSwayAmp(-5)).toBe(CHARM_IDLE_SWAY_AMP_DEFAULT);
    expect(clampCharmIdleSwayAmp(Number.NaN)).toBe(CHARM_IDLE_SWAY_AMP_DEFAULT);
  });
});

describe("normalizeCharmRopeConfig", () => {
  it("缺字段 / 全空时回落默认值（配置还没取回来也能建链）", () => {
    expect(normalizeCharmRopeConfig(undefined)).toEqual({
      ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT,
      segments: CHARM_SEGMENTS_DEFAULT,
      gravity: CHARM_GRAVITY_DEFAULT,
      damping: 0.99,
      lineWidthPx: CHARM_LINE_WIDTH_DEFAULT,
      idleSwayHz: CHARM_IDLE_SWAY_HZ_DEFAULT,
      idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT,
    });
  });

  it("每个字段各自钳位，部分缺失不影响其余（摆动为 0 时仍是 0）", () => {
    expect(
      normalizeCharmRopeConfig({
        ropeLengthPx: 4000,
        segments: 100,
        gravity: 100000,
        damping: 2,
        lineWidthPx: 10,
        idleSwayHz: 9,
        idleSwayAmp: 0,
      }),
    ).toEqual({
      ropeLengthPx: CHARM_ROPE_LENGTH_MAX,
      segments: CHARM_SEGMENTS_MAX,
      gravity: CHARM_GRAVITY_MAX,
      damping: 0.995,
      lineWidthPx: CHARM_LINE_WIDTH_MAX,
      idleSwayHz: CHARM_IDLE_SWAY_HZ_MAX,
      idleSwayAmp: 0,
    });
    expect(normalizeCharmRopeConfig({ ropeLengthPx: 80 })).toEqual({
      ropeLengthPx: 80,
      segments: CHARM_SEGMENTS_DEFAULT,
      gravity: CHARM_GRAVITY_DEFAULT,
      damping: 0.99,
      lineWidthPx: CHARM_LINE_WIDTH_DEFAULT,
      idleSwayHz: CHARM_IDLE_SWAY_HZ_DEFAULT,
      idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT,
    });
  });
});

describe("charmLayerEnabled", () => {
  const base = { finePointerHover: true, prefersReducedMotion: false, configEnabled: true };

  it("三条门控同时满足才启用（精确指针 + 未减少动态 + 总开关）", () => {
    expect(charmLayerEnabled(base)).toBe(true);
  });

  it("触屏 / 无精确指针时不启用", () => {
    expect(charmLayerEnabled({ ...base, finePointerHover: false })).toBe(false);
  });

  it("系统开启「减少动态」时不启用（是整站不出现，不是放慢）", () => {
    expect(charmLayerEnabled({ ...base, prefersReducedMotion: true })).toBe(false);
  });

  it("后台关掉总开关时不启用", () => {
    expect(charmLayerEnabled({ ...base, configEnabled: false })).toBe(false);
  });
});

describe("createCharmRope：静止姿态是确定的", () => {
  it("初始链从指针垂直下垂，每节等长（总长 = 绳长）", () => {
    const state = createCharmRope({ x: 100, y: 200 }, { ...ROPE, ropeLengthPx: 120, segments: 4 });

    expect(state.nodes).toHaveLength(5);
    expect(state.nodes[0]).toEqual({ x: 100, y: 200 });
    expect(state.nodes[1]).toEqual({ x: 100, y: 230 });
    expect(state.nodes[4]).toEqual({ x: 100, y: 320 });
    expect(charmRopeTail(state)).toEqual({ x: 100, y: 320 });
    // 竖直悬挂、松弛为 0：收尾几何也必须是直的。
    expect(charmTetherGeometry(state.nodes, 120, 2.5).maxDeviationPx).toBeLessThan(0.01);
  });

  it("非有限指针不产出 NaN 路径（SVG 会整条消失）", () => {
    const state = createCharmRope({ x: Number.NaN, y: Number.POSITIVE_INFINITY }, ROPE);

    expect(allFinite(nodeCoords(state))).toBe(true);
    expect(charmRopePath(state)).not.toContain("NaN");
    expect(allFinite(charmTetherGeometry(state.nodes, ROPE.ropeLengthPx, 2.5).widths)).toBe(true);
  });
});

describe("固定步长累加器：帧率无关", () => {
  it("一次 500ms 的大帧只按累加器上限推进（6 个子步，不是 60）", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    // 5 秒的帧延迟（切后台/断点）不能被当成 5 秒的物理时间。
    const subSteps = advanceCharmRope(state, 0.5, { x: 0, y: 0 });

    expect(subSteps).toBe(6); // 0.05s / (1/120s)
    expect(subSteps).toBeLessThanOrEqual(CHARM_MAX_SUBSTEPS);
    expect(state.accumulatorSeconds).toBeLessThan(CHARM_FIXED_STEP_SECONDS);
    expect(allFinite(nodeCoords(state))).toBe(true);
    // 链没有被"甩爆"：尾节点离头节点不会超过松垂绳长一个数量级。
    expect(charmRopeTail(state).y).toBeLessThan(CHARM_ROPE_LENGTH_MAX * 2);
  });

  it("一串 1ms 小帧不会比真实流逝的时间推进得更快", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    let subSteps = 0;
    for (let frame = 0; frame < 240; frame += 1) {
      subSteps += advanceCharmRope(state, 0.001, { x: 0, y: 0 });
    }

    // 240ms 最多推进 240ms 的物理时间；允许少一个未满步长的余量。
    expect(subSteps * CHARM_FIXED_STEP_SECONDS).toBeLessThanOrEqual(0.24 + 1e-9);
    expect(subSteps * CHARM_FIXED_STEP_SECONDS).toBeGreaterThanOrEqual(0.24 - CHARM_FIXED_STEP_SECONDS);
    expect(state.accumulatorSeconds).toBeLessThan(CHARM_FIXED_STEP_SECONDS);
  });

  it("单步以上但不到上限的帧按整子步推进，余量留在累加器里", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    // 20ms = 2.4 个步长 → 推进 2 步，剩 0.0033s 留在累加器。
    expect(advanceCharmRope(state, 0.02, { x: 0, y: 0 })).toBe(2);
    expect(state.accumulatorSeconds).toBeGreaterThan(0);
    expect(state.accumulatorSeconds).toBeLessThan(CHARM_FIXED_STEP_SECONDS);
  });

  it("物理推进只由传入的 dt 决定：同样的初始状态 + 同样的帧序列 → 唯一输出", () => {
    // 这条抓的是"偷看 Date.now / performance.now / Math.random"的实现：
    // 一旦积分器自己读时钟，两次运行就会得到不同的节点坐标。
    const first = runFixedSequence();
    const second = runFixedSequence();

    expect(first).toEqual(second);
    expect(allFinite(first.nodes.flatMap((node) => [node.x, node.y]))).toBe(true);
    expect(Number.isFinite(first.energy)).toBe(true);
  });
});

describe("物理不变量（不断言某一帧的坐标，只断言性质）", () => {
  it("剧烈甩动后没有 NaN，能量有限且不发散", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    const heads = [
      { x: 5000, y: 0 },
      { x: 0, y: 5000 },
      { x: -5000, y: 0 },
      { x: 0, y: -5000 },
      { x: 0, y: 0 },
    ];
    let peak = 0;
    for (const head of heads) {
      for (let frame = 0; frame < 100; frame += 1) {
        advanceCharmRope(state, 0.016, head);
        peak = Math.max(peak, charmRopeEnergy(state));
      }
    }

    expect(allFinite(nodeCoords(state))).toBe(true);
    expect(Number.isFinite(peak)).toBe(true);
    expect(peak).toBeLessThan(1e9);
    expect(charmRopePath(state)).not.toContain("NaN");
  });

  it("节点间距始终在约束容差内，链长守恒（绳子不会被拉成弹弓）", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    const rest = ROPE.ropeLengthPx / ROPE.segments;
    // 先甩两下再让它稳定下来：约束求解器在动态阶段允许微小的暂时误差。
    advanceCharmRope(state, 0.05, { x: 120, y: 40 });
    for (let frame = 0; frame < 600; frame += 1) {
      advanceCharmRope(state, FRAME_AT_120HZ, { x: 120, y: 40 });
    }

    const spacings = charmRopeSegmentLengths(state);
    expect(spacings).toHaveLength(ROPE.segments);
    for (const spacing of spacings) {
      expect(Math.abs(spacing - rest)).toBeLessThanOrEqual(0.5);
    }
    const total = spacings.reduce((sum, spacing) => sum + spacing, 0);
    expect(Math.abs(total - ROPE.ropeLengthPx)).toBeLessThanOrEqual(ROPE.segments * 0.5);
  });

  it("指针停住后能量衰减并收敛（静态页面必须能停帧）", () => {
    const state = createCharmRope({ x: 0, y: 0 }, ROPE);
    advanceCharmRope(state, 0.05, { x: 90, y: 0 });
    // 记录 100 步与 700 步时的能量：衰减必须是真的，而不是抖到一个死值。
    let energyAt100 = Number.POSITIVE_INFINITY;
    for (let step = 1; step <= 700; step += 1) {
      advanceCharmRope(state, FRAME_AT_120HZ, { x: 90, y: 0 });
      if (step === 100) energyAt100 = charmRopeEnergy(state);
    }
    const energyAt700 = charmRopeEnergy(state);

    expect(Number.isFinite(energyAt100)).toBe(true);
    expect(energyAt700).toBeLessThan(energyAt100 * 0.5);
    expect(charmRopeSettled(state)).toBe(true);
    expect(charmRopeEnergy(state)).toBeLessThan(0.05);
  });

  it("收敛阈值是绝对下限：静止的竖直链一开始就算收敛（不空转）", () => {
    const state = createCharmRope({ x: 100, y: 100 }, ROPE);

    expect(charmRopeEnergy(state)).toBe(0);
    expect(charmRopeSettled(state)).toBe(true);
  });
});

describe("弧垂只来自真实松弛，不来自重力（票 #42 验收项）", () => {
  it("两端距离恰好等于绳长（松弛 0）时是直线，横向偏移 ~0", () => {
    const geometry = charmTetherGeometry(straightPoints(120), 120, CHARM_LINE_WIDTH_DEFAULT);

    expect(geometry.slackPx).toBe(0);
    expect(geometry.maxDeviationPx).toBeLessThan(0.01);
  });

  it("松弛越多弧越大（同样的两端方向，只让绳长超过实际距离）", () => {
    const taut = charmTetherGeometry(straightPoints(120), 120, CHARM_LINE_WIDTH_DEFAULT);
    const small = charmTetherGeometry(straightPoints(100), 120, CHARM_LINE_WIDTH_DEFAULT);
    const big = charmTetherGeometry(straightPoints(80), 120, CHARM_LINE_WIDTH_DEFAULT);

    expect(small.slackPx).toBe(20);
    expect(big.slackPx).toBe(40);
    expect(small.maxDeviationPx).toBeGreaterThan(taut.maxDeviationPx + 1);
    expect(big.maxDeviationPx).toBeGreaterThan(small.maxDeviationPx + 1);
  });

  it("把 gravity 从最小调到最大，静止（无轻晃）链的弧垂都不加深", () => {
    // 静止的竖直悬挂链：弦长 = 绳长 → 松弛 0 → 直线。重力只改变摆动瞬态，
    // 不能被调成"下垂感"—— 这是票面点名的反"重力 = 下垂"回归测试。
    function restingSag(gravity: number): number {
      const state = createCharmRope(
        { x: 100, y: 100 },
        { ...ROPE, gravity, idleSwayAmp: 0 },
      );
      advanceCharmRope(state, CHARM_MAX_FRAME_SECONDS, { x: 160, y: 120 });
      for (let step = 0; step < 2000; step += 1) {
        advanceCharmRope(state, FRAME_AT_120HZ, { x: 160, y: 120 });
      }
      const geometry = charmTetherGeometry(state.nodes, state.config.ropeLengthPx, state.config.lineWidthPx);
      return geometry.maxDeviationPx;
    }

    const low = restingSag(CHARM_GRAVITY_MIN);
    const high = restingSag(CHARM_GRAVITY_MAX);

    expect(low).toBeLessThan(1);
    expect(high).toBeLessThan(1);
    expect(Math.abs(high - low)).toBeLessThan(1);
  });
});

describe("收尾描边：起始宽度 → 0.6 倍（票 #42 验收项）", () => {
  it("起始宽度 = line_width，收尾恰好为 0.6 × line_width", () => {
    const geometry = charmTetherGeometry(straightPoints(100), 120, 2.5);

    expect(geometry.widths[0]).toBe(2.5);
    expect(geometry.widths[geometry.widths.length - 1]).toBeCloseTo(1.5, 12);
    expect(geometry.widths[geometry.widths.length - 1]! / geometry.widths[0]!).toBeCloseTo(0.6, 12);
  });

  it("换任意起始宽度，比例都还是 0.6（收尾不是第二个配置项）", () => {
    for (const width of [1, 2.5, 4, 6]) {
      const geometry = charmTetherGeometry(straightPoints(100), 120, width);
      expect(geometry.widths.at(-1)! / geometry.widths[0]!).toBeCloseTo(CHARM_TAPER_RATIO, 12);
    }
  });

  it("发出的是一条闭合带形多边形（单根 SVG stroke 变不了宽度）", () => {
    const geometry = charmTetherGeometry(straightPoints(100), 120, 2.5);

    expect(geometry.path.startsWith("M")).toBe(true);
    expect(geometry.path).toContain("Z");
    expect(geometry.path).not.toContain("NaN");
    // 带形两侧各一条边：左 N−1 段 + 尾端连接 1 段 + 右 N−1 段 = 2N−1。
    expect(geometry.path.match(/L/gu)?.length).toBe(geometry.centerline.length * 2 - 1);
  });

  it("渲染入口 charmRopePath 用的就是同一份几何（不是第二套画法）", () => {
    const state = createCharmRope({ x: 10, y: 20 }, { ...ROPE, ropeLengthPx: 120, segments: 4 });

    expect(charmRopePath(state)).toBe(
      charmTetherGeometry(state.nodes, state.config.ropeLengthPx, state.config.lineWidthPx).path,
    );
  });
});

describe("静止轻晃：自由端的真实形变（票 #42 验收项）", () => {
  it("idle_sway_amp > 0 时自由端真的横向摆动，指针端仍钉死", () => {
    const state = createCharmRope(
      { x: 0, y: 0 },
      { ...ROPE, idleSwayHz: CHARM_IDLE_SWAY_HZ_DEFAULT, idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT },
    );
    let peakTailX = 0;
    let peakEnergy = 0;
    for (let step = 0; step < 1440; step += 1) {
      advanceCharmRope(state, FRAME_AT_120HZ, { x: 0, y: 0 });
      peakTailX = Math.max(peakTailX, Math.abs(charmRopeTail(state).x));
      peakEnergy = Math.max(peakEnergy, charmRopeEnergy(state));
    }

    expect(peakTailX).toBeGreaterThan(0.5);
    expect(state.nodes[0]).toEqual({ x: 0, y: 0 });
    // 轻晃期间能量反复越过收敛阈值：怠速车道必须持续跑（否则会"停在一个歪姿势上"）。
    expect(peakEnergy).toBeGreaterThan(0.05);
  });

  it("idle_sway_amp == 0 时完全没有横向形变（可以退回零帧车道）", () => {
    const state = createCharmRope({ x: 0, y: 0 }, { ...ROPE, idleSwayAmp: 0, idleSwayHz: 0 });
    for (let step = 0; step < 720; step += 1) {
      advanceCharmRope(state, FRAME_AT_120HZ, { x: 0, y: 0 });
    }

    expect(Math.abs(charmRopeTail(state).x)).toBeLessThan(1e-9);
    expect(charmRopeSettled(state)).toBe(true);
  });
});
