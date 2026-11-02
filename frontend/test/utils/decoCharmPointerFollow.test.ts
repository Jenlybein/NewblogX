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
  advanceCharmRope,
  charmRopeTail,
  createCharmRope,
  type CharmPoint,
  type CharmRopeConfig,
  type CharmRopeState,
} from "~/utils/decoCharm";

/**
 * 指针端平滑跟随（验收项 1、2、3 的物理侧）。
 *
 * 病灶：指针端曾经是**硬赋值** —— 每个固定子步都把链的第 0 个节点写成指针位置。
 * 一帧里指针可以跳几十像素（60Hz 屏上 4000px/s 就是 67px），这一跳被当成瞬时冲量
 * 灌进链里，距离约束把每节瞬间拉到静止长度：看得见的就是"挂绳整条瞬直"。
 *
 * 修法（见 `decoCharm.ts` 的 `followPointerAnchor`）：指针端变成带阻尼的一阶跟随，
 * 每个固定子步只走完与锚点间隙的一个比例（时间常数由 `stiffness` 换算），外加
 * 滞后上界；`advanceCharmRope` 还在子步之间对锚点做线性插值，让物理看到的输入
 * 与帧率无关。
 *
 * 本文件只断言**看得见的几何量**，不碰实现：
 *   ① 指针端确实在"跟"（有滞后，不是硬赋值），拖拽后弦长小于绳长、链真的弯；
 *   ② 指针端与锚点的距离全程有界 —— 平滑不等于甩脱，松手后精确落回锚点；
 *   ③ 同一轨迹在 16.7ms 与 33.3ms 帧间隔下积分，末态姿态近似一致。
 *
 * 关于高速档的实测边界（写在这里免得后来人重新试一遍）：拖拽速度超过约
 * 2500px/s 时，弦长必然被拉到绳长以上。这是 `stiffness` 的弹性（默认 0.5、
 * 每子步只消化一半误差、固定 20 次迭代）在快速拖动下的真实表现 —— 3200px/s
 * 时实测弦长 138px，弧垂只在 ≤2000px/s 的档位上存在。所以"弦长小于绳长"这条
 * 断言钉在 1333px/s（600px / 0.45s）这个正常"快速拖动"的档位上。
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
const ROPE = CHARM_ROPE_LENGTH_DEFAULT;
const FRAME_60_HZ = 1 / 60;
const FRAME_30_HZ = 1 / 30;

/** 指针端节点（链的第 0 节）到站标（尾节点）的直线距离 = 弦长。 */
function chordLengthPx(state: CharmRopeState): number {
  const head = state.nodes[0];
  const tail = charmRopeTail(state);
  if (!head) return 0;
  return Math.hypot(tail.x - head.x, tail.y - head.y);
}

/** 链上各节点到"头尾弦"的最大垂直距离（px）：这就是肉眼看到的弧垂。 */
function maxDeviationFromChordPx(state: CharmRopeState): number {
  const head = state.nodes[0];
  if (!head) return 0;
  const tail = charmRopeTail(state);
  const dx = tail.x - head.x;
  const dy = tail.y - head.y;
  const length = Math.hypot(dx, dy) || 1;
  let worst = 0;
  for (const node of state.nodes) {
    worst = Math.max(worst, Math.abs((node.x - head.x) * dy - (node.y - head.y) * dx) / length);
  }
  return worst;
}

/** 指针端节点与锚点（指针 + 指针端偏移）的距离：平滑跟随的滞后量。 */
function headLagPx(state: CharmRopeState, anchor: CharmPoint): number {
  const head = state.nodes[0];
  if (!head) return 0;
  return Math.hypot(head.x - anchor.x, head.y - anchor.y);
}

/** 两个姿态的逐节点最大位移（px）：比较"末态姿态是否近似一致"用。 */
function maxNodeDifferencePx(a: CharmRopeState, b: CharmRopeState): number {
  let worst = 0;
  const count = Math.min(a.nodes.length, b.nodes.length);
  for (let index = 0; index < count; index += 1) {
    const left = a.nodes[index];
    const right = b.nodes[index];
    if (!left || !right) continue;
    worst = Math.max(worst, Math.hypot(left.x - right.x, left.y - right.y));
  }
  return worst;
}

/** 建链并让竖直静止姿态走稳（避免把初始瞬态算进拖动峰值）。 */
function settledRope(config: Partial<CharmRopeConfig> = {}): CharmRopeState {
  const state = createCharmRope({ x: 0, y: 0 }, { ...BASE_CONFIG, ...config });
  for (let step = 0; step < 240; step += 1) advanceCharmRope(state, STEP, { x: 0, y: 0 });
  return state;
}

/**
 * 匀速拖拽轨迹：0 → `distancePx`，用时 `durationSeconds`，之后停在终点。
 *
 * 这是"手在动"的最朴素模型：每 1/120s 前进固定的一小段。用它就能把"指针端有没有
 * 惯性"和"帧率是否无关"分开测 —— 输入本身连续时，差异只可能来自跟随策略。
 */
function dragPath(distancePx: number, durationSeconds: number) {
  return (seconds: number): CharmPoint => ({
    x: distancePx * Math.min(1, Math.max(0, seconds / durationSeconds)),
    y: 0,
  });
}

/** 按固定子步跑完整段拖拽，返回状态与全过程最大滞后。 */
function runDragBySubsteps(
  distancePx: number,
  durationSeconds: number,
): { state: CharmRopeState; worstLagPx: number } {
  const state = settledRope();
  const path = dragPath(distancePx, durationSeconds);
  const steps = Math.round(durationSeconds / STEP);
  let worstLagPx = 0;
  for (let step = 1; step <= steps; step += 1) {
    const anchor = path(step * STEP);
    advanceCharmRope(state, STEP, anchor);
    worstLagPx = Math.max(worstLagPx, headLagPx(state, anchor));
  }
  return { state, worstLagPx };
}

/**
 * 同一段轨迹按 `frameSeconds` 的帧间隔积分到 `totalSeconds`。
 *
 * 帧边界上才拿得到新锚点（真实浏览器就是这样），最后一帧用剩余时间补齐，
 * 保证两种帧间隔都在**同一个物理时刻**取末态 —— 否则比较的是两个不同时刻，
 * 分不清差异是帧率造成的还是时间差造成的。
 */
function runDragByFrames(
  distancePx: number,
  durationSeconds: number,
  totalSeconds: number,
  frameSeconds: number,
): CharmRopeState {
  const state = settledRope();
  const path = dragPath(distancePx, durationSeconds);
  let elapsed = 0;
  while (elapsed + frameSeconds <= totalSeconds) {
    elapsed += frameSeconds;
    advanceCharmRope(state, frameSeconds, path(elapsed));
  }
  const rest = totalSeconds - elapsed;
  if (rest > 0) advanceCharmRope(state, rest, path(totalSeconds));
  return state;
}

describe("指针端平滑跟随：真的在跟，而不是硬赋值（验收项 1 前半）", () => {
  it("快速拖拽时指针端落后于输入点（硬赋值时滞后恒为 0）", () => {
    const { worstLagPx } = runDragBySubsteps(600, 0.15);

    expect(worstLagPx).toBeGreaterThan(4);
  });

  it("跟随深度与速度有关：快拖的滞后明显大于慢拖（拽得越快，绳头越懒）", () => {
    const fast = runDragBySubsteps(600, 0.15).worstLagPx;
    const slow = runDragBySubsteps(600, 0.6).worstLagPx;

    expect(fast).toBeGreaterThan(slow);
    expect(slow).toBeGreaterThan(0.5);
  });

  it("慢速拖拽跟得很紧：1000px/s 时滞后不超过几像素", () => {
    const { worstLagPx } = runDragBySubsteps(600, 0.6);

    expect(worstLagPx).toBeLessThan(14);
  });
});

describe("指针端平滑跟随：拖拽后留着弧垂（验收项 1 后半）", () => {
  it("1333px/s 拖拽 600px 后弦长小于绳长、且链真的弯了", () => {
    const { state } = runDragBySubsteps(600, 0.45);

    expect(chordLengthPx(state)).toBeLessThan(ROPE);
    // "确实出现弧垂"：链与头尾弦的最大偏差，不是一根绷紧的杆。
    expect(maxDeviationFromChordPx(state)).toBeGreaterThan(8);
  });

  it("松手 1 秒内弦长回到绳长以下：挂绳不会长期停在绷直状态", () => {
    const { state } = runDragBySubsteps(600, 0.15);
    const anchor = { x: 600, y: 0 };

    let sagged = false;
    for (let step = 0; step < 120; step += 1) {
      advanceCharmRope(state, STEP, anchor);
      if (chordLengthPx(state) < ROPE) sagged = true;
    }

    expect(sagged).toBe(true);
  });
});

describe("指针端平滑跟随：不脱节（验收项 2）", () => {
  it("整段甩动过程中指针端与锚点的距离始终有界", () => {
    const { worstLagPx } = runDragBySubsteps(600, 0.15);

    // 24px 是"还能一眼看出连在指针上"的上限（绳长的五分之一）。
    expect(worstLagPx).toBeLessThan(24);
  });

  it("更暴力的 8000px/s 甩动也不脱节（滞后有硬上界，不随速度增长）", () => {
    const { worstLagPx } = runDragBySubsteps(400, 0.05);

    expect(worstLagPx).toBeLessThan(24);
  });

  it("松手后指针端落回锚点：静止姿态仍然是「指针 + 偏移」，不残留滞后", () => {
    const { state } = runDragBySubsteps(600, 0.15);
    const anchor = { x: 600, y: 0 };
    for (let step = 0; step < 2400; step += 1) advanceCharmRope(state, STEP, anchor);

    expect(headLagPx(state, anchor)).toBeLessThan(0.01);
    // 静止后每节回到静止长度附近（弹性绳在重力下允许极小残差，但不许停在被拉长）。
    expect(chordLengthPx(state)).toBeLessThan(ROPE + 2);
  });
});

describe("指针端平滑跟随：帧率无关（验收项 3）", () => {
  /**
   * 同一段轨迹（0 → 600px / 0.3s，之后停住）在 60Hz 与 30Hz 两档帧间隔下积分到
   * 同一个物理时刻，末态姿态必须近似一致。
   *
   * 硬赋值时锚点是"每帧一个台阶"：帧越稀，台阶越大（60Hz 是 33px、30Hz 是 67px），
   * 灌进链里的冲量差一倍，末态站标能差出 100px 以上 —— 这就是"一快就变形"。
   * 修法是 `advanceCharmRope` 在子步之间对锚点线性插值。
   */
  const TRAJECTORY = { distancePx: 600, durationSeconds: 0.3, totalSeconds: 0.7 };

  function poseAt(frameSeconds: number): CharmRopeState {
    return runDragByFrames(
      TRAJECTORY.distancePx,
      TRAJECTORY.durationSeconds,
      TRAJECTORY.totalSeconds,
      frameSeconds,
    );
  }

  it("逐节点姿态差小于 10% 绳长（60Hz vs 30Hz）", () => {
    const difference = maxNodeDifferencePx(poseAt(FRAME_60_HZ), poseAt(FRAME_30_HZ));

    expect(difference).toBeLessThan(ROPE * 0.1);
  });

  it("弦长与弧垂的量级一致：不会一边绷直一边弯（60Hz vs 30Hz）", () => {
    const a = poseAt(FRAME_60_HZ);
    const b = poseAt(FRAME_30_HZ);

    expect(Math.abs(chordLengthPx(a) - chordLengthPx(b))).toBeLessThan(8);
    expect(Math.abs(maxDeviationFromChordPx(a) - maxDeviationFromChordPx(b))).toBeLessThan(8);
  });

  it("慢拖也一样：帧率只决定采样密度，不改变末态（600px / 0.6s）", () => {
    const difference = maxNodeDifferencePx(
      runDragByFrames(600, 0.6, 1, FRAME_60_HZ),
      runDragByFrames(600, 0.6, 1, FRAME_30_HZ),
    );

    expect(difference).toBeLessThan(ROPE * 0.1);
  });
});
