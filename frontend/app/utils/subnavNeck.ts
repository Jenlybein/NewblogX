import { resolveSpringMotion, SPRING_NECK, type SpringOrJumpTransition } from "~/utils/motion";

/**
 * 消息中心分段条的纯几何与纯接线（票 297 建立；颈部按审稿意见整体退役）。
 *
 * 分工与 `folderCardGeometry` 一致：
 *   - 这里只做算术与判定（纯函数，可单测，SSR 与客户端算出的结果逐字节相同）；
 *   - 组件只消费这里的结果，不在模板里写几何字面量；
 *   - 弹簧参数**只从 `utils/motion.ts` 的预设取**（`SPRING_NECK`），本文件不出现任何
 *     stiffness / damping 数字。`test/design/motion-spring-gates.test.ts` 会扫这一条。
 *
 * 形态（参考组件 GooeyNav，但**不再画连接形状**）：
 *   五段**相邻贴合**成一条分段控件；激活段两侧的接缝张开（张开量 = `SEAM_OPEN_PX`），
 *   张开的接缝里**什么都不画** —— 激活块就是一块独立的深色圆角块，两侧留干净的间隙。
 *   圆角只在**敞口**的一侧给：张开的接缝旁与控件两端是圆的，闭合接缝旁必须是 0，
 *   否则相邻两段的圆角会在贴合处啃出四个缺口，整条就读成"五个胶囊"而不是"一条"。
 *   闭合接缝的段位重叠 1px：贴合不留发丝缝（浏览器亚像素取整会在 0 间隙处漏一道白线）。
 *
 * 文件名沿用票 297 的 `subnavNeck`（调用点与测试都以它为入口）：里面的 "neck" 现在只剩
 * `SPRING_NECK` 这一个预设名 —— 段位偏移仍由它驱动（见 `seamTransition`），它是全站唯一
 * 允许布局属性动画的地方。
 */

/**
 * 闭合接缝的段位偏移（px）：负值 = 相邻两段互相压住 1px。
 *
 * 为什么不是 0：`margin-left: 0` 时两段的边线落在同一坐标，亚像素取整会把
 * 抗锯齿的白边露出来（"发丝缝"）。压 1px 让后一段盖住前一段的边。
 */
export const SEAM_CLOSED_GAP = -1;

/**
 * 张开量（px）：激活段两侧接缝的分离距离。
 *
 * 原来取 20 是为了给颈部让出画布；颈部退役后 20 会读成"一道被掰开的缝"（两块碎掉的
 * 半条），收到 10 才读成"一块独立的胶囊 + 干净的间隙"。
 */
export const SEAM_OPEN_PX = 10;

/**
 * 段的敞口圆角（px）：圆角阶梯的 `--radius-md`（field 档）。
 *
 * 只给张开的接缝一侧与控件两端；闭合接缝一侧恒为 0（见文件头的"为什么要分侧"）。
 */
export const SEAM_CORNER_PX = 14;

/**
 * 一段的到位状态：**接缝间隙 + 四角圆角**（同一组弹簧一起走）。
 *
 * `--seam-gap` 是这批值里唯一表达"间隙"的量，而且它可以是负的（闭合接缝的 -1px 重叠）。
 * 段位（`margin-left`）**从它派生**（见 tailwind.css 的 `.message-center__subnav-segment`），
 * 不在 JS 里另写一份 `margin-left`：两份值一旦不同步，回弹期间段位就会与到位状态错开。
 */
export interface SeamSegmentMotion extends Record<string, string> {
  borderTopLeftRadius: string;
  borderTopRightRadius: string;
  borderBottomRightRadius: string;
  borderBottomLeftRadius: string;
  /** 左侧接缝的间隙（px，可为负）：段位读它。 */
  "--seam-gap": string;
}

/**
 * 第 `index` 段左侧接缝的间隙（px）。
 *
 * - `index === 0`：控件最左端没有接缝，返回 0；
 * - 接缝紧挨激活块（激活段自己的左接缝，或激活段右边那一段的左接缝）：张开；
 * - 其余：闭合（重叠 1px）。
 *
 * `activeIndex` 越界（没有任何一段匹配当前路由）时全部按闭合处理，不抛错。
 */
export function seamGap(index: number, activeIndex: number): number {
  if (!Number.isFinite(index) || index <= 0) return 0;
  const touchesActive = index === activeIndex || index === activeIndex + 1;
  return touchesActive ? SEAM_OPEN_PX : SEAM_CLOSED_GAP;
}

/** 这条接缝是否张开（段在这个接缝侧是敞口的）。 */
export function isSeamOpen(index: number, activeIndex: number): boolean {
  return seamGap(index, activeIndex) > 0;
}

/**
 * 第 `seam` 条接缝（0..count）该不该给圆角。
 *
 * 控件两端的"外边"（0 与 count）永远敞口；中间的接缝只在张开时敞口。
 * 闭合接缝旁给圆角会在贴合处啃出缺口，所以那里必须是 0。
 */
export function seamCorner(seam: number, activeIndex: number, count: number): number {
  if (!Number.isFinite(seam) || !Number.isFinite(count) || count <= 0) return 0;
  const edge = seam <= 0 || seam >= count;
  return edge || isSeamOpen(seam, activeIndex) ? SEAM_CORNER_PX : 0;
}

/**
 * 第 `index` 段到位后的样式（弹簧的终点）。
 *
 * 只给**一份**间隙（`--seam-gap`，可为负）+ 四角圆角；段位在 CSS 里从这份间隙派生
 * （`margin-left` 取原值），所以回弹期间段位与圆角不可能互相错开。
 *
 * 四角圆角分开给：左接缝决定左侧两角、右接缝决定右侧两角（`count` 用来认控件两端）。
 */
export function seamSegmentMotion(
  index: number,
  activeIndex: number,
  count: number,
): SeamSegmentMotion {
  const gap = seamGap(index, activeIndex);
  const left = seamCorner(index, activeIndex, count);
  const right = seamCorner(index + 1, activeIndex, count);

  return {
    borderTopLeftRadius: `${left}px`,
    borderTopRightRadius: `${right}px`,
    borderBottomRightRadius: `${right}px`,
    borderBottomLeftRadius: `${left}px`,
    "--seam-gap": `${gap}px`,
  };
}

/**
 * 段位的过渡：要么 `SPRING_NECK`，要么「减少动态」下的落位。
 *
 * 直接复用 `motion.ts` 的 `resolveSpringMotion`：reduce 时给的是时长 0 的 tween
 * （**落位，不是放慢的弹簧**），与 ADR-0012 后续修订（2026-11-07）的口径一致。
 */
export function seamTransition(prefersReducedMotion: boolean): SpringOrJumpTransition {
  return resolveSpringMotion(SPRING_NECK, prefersReducedMotion);
}

/** 过渡的形态名（`spring` / `jump`）：挂在 DOM 上供设计门禁与调试观察。 */
export function seamMotionKind(prefersReducedMotion: boolean): "spring" | "jump" {
  return prefersReducedMotion ? "jump" : "spring";
}
