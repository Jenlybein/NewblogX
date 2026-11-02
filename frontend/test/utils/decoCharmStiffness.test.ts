import { describe, expect, it } from "vitest";

import {
  CHARM_DAMPING_DEFAULT,
  CHARM_FIXED_STEP_SECONDS,
  CHARM_FPS_CAP_DEFAULT,
  CHARM_GRAVITY_DEFAULT,
  CHARM_LINE_WIDTH_DEFAULT,
  CHARM_POINTER_OFFSET_X_DEFAULT,
  CHARM_POINTER_OFFSET_Y_DEFAULT,
  CHARM_ROPE_LENGTH_DEFAULT,
  CHARM_SEGMENTS_DEFAULT,
  CHARM_STIFFNESS_DEFAULT,
  CHARM_STIFFNESS_MAX,
  CHARM_STIFFNESS_MIN,
  advanceCharmRope,
  charmRopeEnergy,
  charmRopeSegmentLengths,
  charmRopeSettled,
  charmRopeTail,
  clampCharmStiffness,
  createCharmRope,
  normalizeCharmRopeConfig,
  type CharmPoint,
  type CharmRopeConfig,
  type CharmRopeState,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：挂绳从"刚杆"变成"弹性绳"是手感问题，手感必须落成可观测的
 * 外部行为 —— 同样的拖动输入下，低刚度先被拉长、随后回弹，最终仍然收敛；
 * 刚度接近 1 时回到近似原来的硬链。这里不断言内部迭代次数，只断言这些性质，
 * 这样换求解器写法时用例仍然有效。
 */

const BASE_CONFIG: CharmRopeConfig = {
  ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT,
  segments: CHARM_SEGMENTS_DEFAULT,
  gravity: CHARM_GRAVITY_DEFAULT,
  damping: CHARM_DAMPING_DEFAULT,
  lineWidthPx: CHARM_LINE_WIDTH_DEFAULT,
  idleSwayHz: 0,
  idleSwayAmp: 0,
  fpsCap: CHARM_FPS_CAP_DEFAULT,
  pointerOffsetX: CHARM_POINTER_OFFSET_X_DEFAULT,
  pointerOffsetY: CHARM_POINTER_OFFSET_Y_DEFAULT,
  stiffness: CHARM_STIFFNESS_DEFAULT,
};

const STEP = CHARM_FIXED_STEP_SECONDS;

/** 一次拖动：指针在 12 个子步里匀速向右移动 360px（约 3600px/s）。 */
const DRAG_STEPS = 12;
const DRAG_STEP_PX = 30;
const DRAG_END_X = DRAG_STEPS * DRAG_STEP_PX;

function maxStrainPx(state: CharmRopeState): number {
  const rest = state.restLengthPx;
  return charmRopeSegmentLengths(state).reduce(
    (worst, length) => Math.max(worst, Math.abs(length - rest)),
    0,
  );
}

/** 先让竖直链在重力下走稳，避免把初始的微小瞬态算进拖动峰值。 */
function settleVertical(state: CharmRopeState, head: CharmPoint = { x: 0, y: 0 }): void {
  for (let step = 0; step < 240; step += 1) advanceCharmRope(state, STEP, head);
}

/**
 * 同一段拖动输入下，记录链的最大单节形变（px）与拖动结束时的站标位置。
 *
 * 形变越大 = 绳子被拉得越长；站标越靠后 = 滞后越明显。
 */
function dragRun(overrides: Partial<CharmRopeConfig> = {}): {
  state: CharmRopeState;
  peakStrainPx: number;
  tailAtDragEndX: number;
} {
  const state = createCharmRope({ x: 0, y: 0 }, { ...BASE_CONFIG, ...overrides });
  settleVertical(state);

  let peakStrainPx = 0;
  for (let step = 1; step <= DRAG_STEPS; step += 1) {
    advanceCharmRope(state, STEP, { x: DRAG_STEP_PX * step, y: 0 });
    peakStrainPx = Math.max(peakStrainPx, maxStrainPx(state));
  }

  return { state, peakStrainPx, tailAtDragEndX: charmRopeTail(state).x };
}

/** 拖动结束后指针停在终点，跑满 1200 子步（记录首次收敛的时刻，但继续走完）。 */
function settleAfterDrag(state: CharmRopeState, maxSteps = 1200): number {
  let settledAt = maxSteps;
  for (let step = 1; step <= maxSteps; step += 1) {
    advanceCharmRope(state, STEP, { x: DRAG_END_X, y: 0 });
    if (settledAt === maxSteps && charmRopeSettled(state)) settledAt = step;
  }
  return settledAt;
}

describe("挂绳刚度：参数与归一化", () => {
  it("刚度区间 0.2–1、默认 0.5（越接近 1 越硬）", () => {
    expect(CHARM_STIFFNESS_MIN).toBe(0.2);
    expect(CHARM_STIFFNESS_MAX).toBe(1);
    expect(CHARM_STIFFNESS_DEFAULT).toBe(0.5);
  });

  it("越界钳到边界，零与非有限值回落默认（0 是「没填」而不是最小刚度）", () => {
    expect(clampCharmStiffness(0.5)).toBe(0.5);
    expect(clampCharmStiffness(0.05)).toBe(CHARM_STIFFNESS_MIN);
    expect(clampCharmStiffness(2)).toBe(CHARM_STIFFNESS_MAX);
    expect(clampCharmStiffness(0)).toBe(CHARM_STIFFNESS_DEFAULT);
    expect(clampCharmStiffness(-1)).toBe(CHARM_STIFFNESS_DEFAULT);
    expect(clampCharmStiffness(Number.NaN)).toBe(CHARM_STIFFNESS_DEFAULT);
    expect(clampCharmStiffness(Number.POSITIVE_INFINITY)).toBe(CHARM_STIFFNESS_DEFAULT);
  });

  it("缺字段 / null 时归一化回落默认刚度（旧配置不会变成无限软的橡皮筋）", () => {
    expect(normalizeCharmRopeConfig(undefined).stiffness).toBe(CHARM_STIFFNESS_DEFAULT);
    expect(normalizeCharmRopeConfig({ stiffness: null }).stiffness).toBe(
      CHARM_STIFFNESS_DEFAULT,
    );
    expect(normalizeCharmRopeConfig({ stiffness: 0.8 }).stiffness).toBe(0.8);
  });
});

describe("挂绳刚度：拖动时的弹性与滞后", () => {
  it("默认刚度（0.5）下快速拖动出现可观测的拉伸，而不是一根刚杆", () => {
    const run = dragRun();

    expect(run.peakStrainPx).toBeGreaterThan(4);
    // 站标被拖在后面：拖动过程中尾端明显落后于指针端。
    expect(run.tailAtDragEndX).toBeLessThan(DRAG_END_X - 40);
  });

  it("刚度接近 1 时明显更硬：同样的拖动下形变更小、站标跟得更紧", () => {
    const soft = dragRun({ stiffness: CHARM_STIFFNESS_DEFAULT });
    const rigid = dragRun({ stiffness: CHARM_STIFFNESS_MAX });

    expect(rigid.peakStrainPx).toBeLessThan(soft.peakStrainPx * 0.6);
    expect(rigid.tailAtDragEndX).toBeGreaterThan(soft.tailAtDragEndX);
  });

  it("松手后回弹并收敛：拖到终点停住，链会回到静止长度且能量降到阈值以下", () => {
    const run = dragRun();
    const stepsToSettle = settleAfterDrag(run.state);

    expect(stepsToSettle).toBeLessThan(700);
    expect(charmRopeSettled(run.state)).toBe(true);
    expect(charmRopeEnergy(run.state)).toBeLessThan(0.05);
    // 回弹是真的：站标从被拖后的滞后位置追回到指针端下方。
    expect(charmRopeTail(run.state).x).toBeGreaterThan(run.tailAtDragEndX + 20);
    expect(Math.abs(charmRopeTail(run.state).x - DRAG_END_X)).toBeLessThan(1);
    // 静止后每节回到静止长度附近：弹性不等于永远拉长。
    expect(maxStrainPx(run.state)).toBeLessThan(0.5);
    expect(stateFinite(run.state)).toBe(true);
  });

  it("节点数拉到上限不再更硬：24 节的形变不小于 4 节（旧实现随节点数变硬）", () => {
    const four = dragRun({ segments: 4 });
    const max = dragRun({ segments: 24 });

    // 这里的判据是"同样拖动下至少一样软"，不要求与节点数成正比：
    // 旧实现每子步迭代 segments * 30 次，节点越多吸收误差越快，反而越硬。
    expect(max.peakStrainPx).toBeGreaterThan(four.peakStrainPx + 1);
    expect(max.peakStrainPx).toBeGreaterThan(CHARM_STIFFNESS_MIN * 5);
  });
});

function stateFinite(state: CharmRopeState): boolean {
  return state.nodes.every((node) => Number.isFinite(node.x) && Number.isFinite(node.y));
}
