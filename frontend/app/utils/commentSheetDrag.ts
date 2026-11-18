/**
 * 评论输入面板的「把手下滑关闭」判定（票 431，spec 423 第 1 批第 2 项）。
 *
 * 这条手势只有两个出口：拖够了就关，没拖够就弹回。判定本身是纯函数，理由有三：
 *   1. 阈值（30% 面板高度 / 0.11 px/ms）是规格写死的数，不该散落在组件里；
 *   2. 组件测试里没有真实布局（`getBoundingClientRect().height` 恒为 0），
 *      只有把判定抽出来，才能钉住"刚好等于 30% 不算""速度刚好 0.11 不算"这类边界；
 *   3. 手势参数以后要调（比如改成 25%），改这里一处就够，不必翻组件。
 *
 * 两个阈值都是**严格大于**才关闭 —— 与 spec 的「位移 > 面板高度 30% 或 速度 > 0.11 px/ms」
 * 一致：等于阈值时宁可弹回，不要误关读者写了一半的评论。
 */

/** 下滑超过面板高度的这个比例就关闭。 */
export const SHEET_DISMISS_DISTANCE_RATIO = 0.3;

/** 或者甩得比这个速度更快（px/ms）也关闭。 */
export const SHEET_DISMISS_VELOCITY_PX_PER_MS = 0.11;

export interface SheetDragRelease {
  /** 松手时的下滑位移（px，向上为 0）。 */
  offsetPx: number;
  /** 从按下到松手经过的毫秒数；0 / 非法值代表"没有可用的时间证据"。 */
  elapsedMs: number;
  /** 面板高度（px）；0 代表取不到布局（只按速度判定，不按比例）。 */
  panelHeightPx: number;
}

/**
 * 松手时该不该关面板。
 *
 * - 位移：只有面板高度可信（>0）时才按 30% 判定；高度取不到时不做"任意位移都算过线"，
 *   否则布局一异常面板就会一碰就关。
 * - 速度：`elapsedMs <= 0` 时没有时间证据（同时间戳的合成事件、极端输入），
 *   一律算 0 —— 不能让它除出 Infinity 把面板甩走。
 */
export function shouldDismissSheetDrag({
  offsetPx,
  elapsedMs,
  panelHeightPx,
}: SheetDragRelease): boolean {
  const offset = Number.isFinite(offsetPx) ? Math.max(0, offsetPx) : 0;

  if (panelHeightPx > 0 && offset > panelHeightPx * SHEET_DISMISS_DISTANCE_RATIO) {
    return true;
  }

  if (!Number.isFinite(elapsedMs) || elapsedMs <= 0) {
    return false;
  }

  return offset / elapsedMs > SHEET_DISMISS_VELOCITY_PX_PER_MS;
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
