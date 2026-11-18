/**
 * 松手瞬时速度（px/ms）：拖拽手势共用的纯函数（票 423）。
 *
 * 为什么抽出来：评论输入面板的下滑关闭要算"松手那一刻甩多快"。这里集中算法；
 * 采样窗口按使用场景登记成命名常量，由调用方传入，组件里不许再写毫秒。
 * 捏捏（Jelly）的拖拽松手落位已随「固定右下角、不可拖」退场，本模块只服务评论面板。
 *
 * 口径：
 *   - 只认 `[atMs - windowMs, atMs)` 内的采样，取**窗口内最早一点**作为起点；
 *     窗口外的旧采样不参与 —— 慢拖几秒再甩一下不该被平均成"没甩"，
 *     快拖半屏后停住再松手也不该因为平均速度高被误判。
 *   - 至少要有两个有效采样（调用方必须把**松手点本身**作为最后一个采样传入），
 *     时间差 ≤0 或没有窗口内采样时返回 `{x:0, y:0}`，由位移阈值决定去留。
 *
 * 坐标语义由调用方约定：y 向下为正（评论面板用 y=下滑位移、x 恒为 0）。
 */

/** 一次拖拽期的指针采样：坐标（px）+ 时间戳（毫秒）。 */
export interface ReleaseVelocitySample {
  x: number;
  y: number;
  t: number;
}

/**
 * 评论输入面板下滑关闭的采样窗口（毫秒）。
 *
 * 100ms 而不是"最后一帧"：采样本身有间隔（高刷屏 8ms、普通 16ms、掉帧时更久），
 * 100ms 至少能收到两点；再长就会把"拖到一半停手"也算进速度里。
 */
export const SHEET_RELEASE_VELOCITY_WINDOW_MS = 100;

/**
 * 松手瞬间速度（px/ms）。
 *
 * @param samples 拖动期采样（时间升序；**最后一个必须是松手点**，且 `t === atMs`）。
 * @param atMs 松手时刻（毫秒，来自 `pointerup` 的 timeStamp）。
 * @param windowMs 只回看这么久内的采样。
 */
export function releaseVelocity(
  samples: readonly ReleaseVelocitySample[],
  atMs: number,
  windowMs: number,
): { x: number; y: number } {
  if (!Number.isFinite(atMs) || !Number.isFinite(windowMs) || windowMs <= 0) {
    return { x: 0, y: 0 };
  }
  if (samples.length < 2) return { x: 0, y: 0 };

  const last = samples[samples.length - 1];
  if (!last || !Number.isFinite(last.x) || !Number.isFinite(last.y)) return { x: 0, y: 0 };

  const windowStart = atMs - windowMs;
  let first: ReleaseVelocitySample | null = null;
  for (const sample of samples) {
    if (!Number.isFinite(sample.x) || !Number.isFinite(sample.y) || !Number.isFinite(sample.t)) {
      continue;
    }
    // 松手点自己不能当"起点"（位移差为 0），窗口外的旧采样也不参与。
    if (sample.t < windowStart || sample.t >= atMs) continue;
    if (!first || sample.t < first.t) first = sample;
  }

  if (!first) return { x: 0, y: 0 };

  const elapsed = atMs - first.t;
  if (!(elapsed > 0)) return { x: 0, y: 0 };

  return {
    x: (last.x - first.x) / elapsed,
    y: (last.y - first.y) / elapsed,
  };
}
