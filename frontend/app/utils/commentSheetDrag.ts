/**
 * 评论输入面板的「把手下滑关闭」判定（票 431，spec 423 第 1 批第 2 项；
 * 松手速度口径由票 437 评审修正）。
 *
 * 这条手势只有两个出口：拖够了就关，没拖够就弹回。判定本身是纯函数，理由有三：
 *   1. 阈值（30% 面板高度 / 0.11 px/ms）是规格写死的数，不该散落在组件里；
 *   2. 组件测试里没有真实布局（`getBoundingClientRect().height` 恒为 0），
 *      只有把判定抽出来，才能钉住"刚好等于 30% 不算""速度刚好 0.11 不算"这类边界；
 *   3. 手势参数以后要调（比如改成 25%），改这里一处就够，不必翻组件。
 *
 * **速度用瞬时值，不是全程平均**（票 437 评审）：按「按下到松手」的平均速度判定，
 * 慢拖 5 秒再甩一下只会得到 0.03 px/ms（关不掉），而快拖半屏后停住再松手反而
 * 因为平均速度高被关掉 —— 两个方向都和手感相反。现在只取**最后 100ms** 的采样：
 * 窗口内至少两点才算"有速度证据"，窗口内没有新采样就退回位移阈值判定。
 * 采样与窗口/取点规则和捏捏松手落位共用 `utils/releaseVelocity.ts`（票 423），
 * 这里只保留"评论面板自己的窗口"这条命名常量。
 *
 * 两个阈值都是**严格大于**才关闭 —— 与 spec 的「位移 > 面板高度 30% 或 速度 > 0.11 px/ms」
 * 一致：等于阈值时宁可弹回，不要误关读者写了一半的评论。
 */

import { releaseVelocity, SHEET_RELEASE_VELOCITY_WINDOW_MS } from "./releaseVelocity";

/** 下滑超过面板高度的这个比例就关闭。 */
export const SHEET_DISMISS_DISTANCE_RATIO = 0.3;

/** 或者甩得比这个速度更快（px/ms）也关闭。 */
export const SHEET_DISMISS_VELOCITY_PX_PER_MS = 0.11;

/** 评论面板的松手采样窗口（毫秒）：常量与实现集中在 `utils/releaseVelocity.ts`。 */
export { SHEET_RELEASE_VELOCITY_WINDOW_MS };

/** 一次拖动期的 pointermove 采样：位移（px，向下为正）+ 时间戳（毫秒）。 */
export interface SheetDragSample {
  offsetPx: number;
  atMs: number;
}

export interface SheetDragRelease {
  /** 松手时的下滑位移（px，向上为 0）。 */
  offsetPx: number;
  /** 面板高度（px）；0 代表取不到布局（只按速度判定，不按比例）。 */
  panelHeightPx: number;
  /** 松手时刻（毫秒，来自 `pointerup` 的 timeStamp）。 */
  releaseAtMs: number;
  /** 拖动期的 pointermove 采样（时间升序；可以包含按下那一刻的 0 位移）。 */
  samples: readonly SheetDragSample[];
}

/**
 * 松手瞬间的下滑速度（px/ms）。
 *
 * 只看 `[releaseAtMs - 窗口, releaseAtMs)` 内的采样，取窗口内**最早一点**到松手点的
 * 位移差。窗口内没有有效采样（或时间差 ≤0）时返回 0 —— 没有速度证据就不猜，
 * 由位移阈值决定去留，不能除出 Infinity 把面板甩走。
 */
export function sheetReleaseVelocityPxPerMs({
  offsetPx,
  releaseAtMs,
  samples,
}: Pick<SheetDragRelease, "offsetPx" | "releaseAtMs" | "samples">): number {
  const offset = Number.isFinite(offsetPx) ? offsetPx : 0;
  // 共用算法要求"最后一个采样就是松手点"：把松手位移追加上去，而不是让算法读
  // 一个额外的 `releaseOffset` 参数（两处消费者的输入形状因此完全一样）。
  const releaseSamples = [
    ...samples.map((sample) => ({ x: 0, y: sample.offsetPx, t: sample.atMs })),
    { x: 0, y: offset, t: releaseAtMs },
  ];

  return releaseVelocity(releaseSamples, releaseAtMs, SHEET_RELEASE_VELOCITY_WINDOW_MS).y;
}

/**
 * 松手时该不该关面板。
 *
 * - 位移：只有面板高度可信（>0）时才按 30% 判定；高度取不到时不做"任意位移都算过线"，
 *   否则布局一异常面板就会一碰就关。
 * - 速度：取最后 100ms 的瞬时速度（见 `sheetReleaseVelocityPxPerMs`）；窗口内
 *   没有采样时按 0 处理，交给位移阈值。
 */
export function shouldDismissSheetDrag(release: SheetDragRelease): boolean {
  const offset = Number.isFinite(release.offsetPx) ? Math.max(0, release.offsetPx) : 0;

  if (release.panelHeightPx > 0 && offset > release.panelHeightPx * SHEET_DISMISS_DISTANCE_RATIO) {
    return true;
  }

  return sheetReleaseVelocityPxPerMs(release) > SHEET_DISMISS_VELOCITY_PX_PER_MS;
}

/**
 * 拖动期的位移：只认向下（向上归零），保证面板不会往上跑出视口。
 * 非有限值（NaN / Infinity）归零，坏事件不该把面板顶到奇怪位置。
 */
export function sheetDragOffset(startY: number, currentY: number): number {
  if (!Number.isFinite(startY) || !Number.isFinite(currentY)) {
    return 0;
  }
  return Math.max(0, currentY - startY);
}
