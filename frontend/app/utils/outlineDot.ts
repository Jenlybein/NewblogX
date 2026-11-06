/**
 * 目录树「沟槽圆点」的几何（纯函数）。
 *
 * 圆点是**装饰**：它只回答两件事——「当前文章在第几层」（横向压在那一层的发丝线上）
 * 与「这一行在树的哪个高度」（纵向取行垂直中心）。换行时圆点走一条弧，弧的形状
 * 也由这里的纯函数给出，组件只负责把弹簧进度喂进来。
 *
 * 视觉层（尺寸 / 颜色 / 与线的叠压）在 `OutlineTree.vue` 的 `.outline-tree__dot`：
 * 0.75rem 实心 `--color-ink` 圆点，正中一道 1px 纸色缝，让 1px 发丝线在视觉上
 * 穿过圆点继续往下——「点压线」一眼可辨，不另起色相。
 *
 * 横向口径与 `OutlineTree.vue` 的缩进**同一份**：每层 `1.5rem`，发丝线画在缩进
 * 起点左侧 `0.75rem` 处（`.outline-tree__item::before` 的 `left`）。所以
 * `x = depth × 1.5rem − 0.75rem`；`depth 0` 没有线，圆点压在最外侧缩进起点（`0`）。
 *
 * 弧线的量级照参考组件：法向偏移的模 = `min(0.8 × 位移长度, 14px)`，即参考里的
 * `strength = min(0.8, 14 / distance)` 乘上位移向量。纯纵向（同层换行）与纯横向
 * 位移都**不画弧**——同层移动时圆点必须一直压在引导线上，横着鼓出去就离开了线。
 * 跨层级（x/y 同时变）时 14px 上限就是**鼓包峰值**：位移 54px 的组合（相邻层、
 * 相邻行）也在中段横向偏出 12px 以上，肉眼能看出"绕过去"而不是直线平移。
 *
 * 为什么不用 `arc()`：`motion` 包未安装（`motion-v` 不转出该 API），弧线改由
 * 「弹簧驱动 0→1 进度 + 正弦偏移」自己算（ADR-0012 后续修订 2026-11-07 已登记）。
 */

/** 每层缩进（rem）：与树的 `padding-left` 同一口径。 */
export const OUTLINE_INDENT_REM = 1.5;

/** 发丝引导线相对缩进起点的偏移（rem）：线在起点左侧半个缩进步长处。 */
export const OUTLINE_GUIDE_OFFSET_REM = 0.75;

/** 弧线法向偏移的绝对值上限（px）：参考组件的量级。 */
export const OUTLINE_DOT_ARC_MAX_PX = 14;

/** 弧线法向偏移相对位移长度的上限：短距离移动也不画成"绕一圈"。 */
export const OUTLINE_DOT_ARC_MAX_RATIO = 0.8;

/** 根字号读不到时的兜底（rem 换算用）。 */
const FALLBACK_ROOT_FONT_SIZE_PX = 16;

/** 二维点（圆点坐标 / 弧偏移都用这个形状）。 */
export interface DotPoint {
  x: number;
  y: number;
}

/** 层级归整：缺失、非法与负值都按根层级（0）；小数向下取整，不猜"半层"。 */
function outlineDepth(depth: number | null | undefined): number {
  if (depth == null || !Number.isFinite(depth)) return 0;
  return Math.max(0, Math.floor(depth));
}

/**
 * 圆点横坐标（rem）：落在该行所属层级的发丝线上。
 *
 * `depth 0` 取最外侧缩进起点（`0`）——根层没有引导线，圆点压在那条线的起点位置。
 */
export function outlineDotXRem(depth: number | null | undefined): number {
  const level = outlineDepth(depth);
  if (level === 0) return 0;
  return level * OUTLINE_INDENT_REM - OUTLINE_GUIDE_OFFSET_REM;
}

/** 圆点横坐标（px）：`rem` 按根字号换算，根字号缺失时按 16px 兜底。 */
export function outlineDotXPx(depth: number | null | undefined, rootFontSizePx: number): number {
  const fontSize =
    Number.isFinite(rootFontSizePx) && rootFontSizePx > 0
      ? rootFontSizePx
      : FALLBACK_ROOT_FONT_SIZE_PX;
  return outlineDotXRem(depth) * fontSize;
}

/**
 * 弧线在进度 0.5 处的法向鼓包（px，非负标量）。
 *
 * 参考量级 `min(0.8, 14 / distance)` 乘回位移长度 = `min(0.8 × distance, 14px)`：
 * 短距离按比例鼓（最多 0.8 倍位移），长距离一律收在 14px，免得绕成一个大圈。
 * 纯纵向 / 纯横向 / 零位移为 0——这几种移动必须留在引导线或行中心的**直线**上，
 * 鼓出去就离开了线，圆点会被读成"飘"。
 */
export function outlineDotArcBulgePx(dx: number, dy: number): number {
  if (!Number.isFinite(dx) || !Number.isFinite(dy) || dx === 0 || dy === 0) {
    return 0;
  }

  return Math.min(OUTLINE_DOT_ARC_MAX_PX, OUTLINE_DOT_ARC_MAX_RATIO * Math.hypot(dx, dy));
}

/**
 * 弧线的法向偏移向量（px）：进度 0.5 处鼓出的最大值。
 *
 * 位移向量顺时针转 90° 得到法向（位移右下 → 弧往左下鼓）；退化位移返回
 * `{ x: 0, y: 0 }`（直线），因为圆点离开引导线就失去了"压线"的语义。
 */
export function outlineDotArcOffset(dx: number, dy: number): DotPoint {
  const bulge = outlineDotArcBulgePx(dx, dy);
  if (bulge === 0) {
    return { x: 0, y: 0 };
  }

  const distance = Math.hypot(dx, dy);

  return {
    x: (-dy / distance) * bulge,
    y: (dx / distance) * bulge,
  };
}

/**
 * 进度 `progress`（0→1，弹簧驱动）处的圆点坐标：线性插值 + `sin(πt)` 弧偏移。
 *
 * 两端严格落在 `from` / `to` 上（`sin(0) = sin(π) = 0`），所以弹簧收敛后圆点
 * 一定正好压在引导线与行中心上，不会留一点残余偏移。越界与非数值钳在两端。
 */
export function outlineDotPointAt(from: DotPoint, to: DotPoint, progress: number): DotPoint {
  const t = Number.isFinite(progress) ? Math.min(1, Math.max(0, progress)) : 0;
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const offset = outlineDotArcOffset(dx, dy);
  const bulge = Math.sin(Math.PI * t);

  return {
    x: from.x + dx * t + offset.x * bulge,
    y: from.y + dy * t + offset.y * bulge,
  };
}
