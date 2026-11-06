import { resolveSpringMotion, SPRING_NECK, type SpringOrJumpTransition } from "~/utils/motion";

/**
 * 消息中心分段条的纯几何与纯接线（票 297；术语见 GLOSSARY「颈部」，
 * 口径见 docs/notes/2026-11-07-reading-ui-grill.md §三 3.7 与 §三 3.8）。
 *
 * 分工与 `folderCardGeometry` 一致：
 *   - 这里只做算术与判定（纯函数，可单测，SSR 与客户端算出的结果逐字节相同）；
 *   - 组件只消费这里的结果，不在模板里写几何字面量；
 *   - 弹簧参数**只从 `utils/motion.ts` 的预设取**（`SPRING_NECK`），本文件不出现任何
 *     stiffness / damping 数字。`test/design/motion-spring-gates.test.ts` 会扫这一条。
 *
 * 形态（参考组件的 GooeyNav，本票只复刻形态）：
 *   五段**相邻贴合**成一条分段控件；只有激活段两侧的接缝"张开"（张开量 =
 *   `NECK_SEPARATION_PX`），张开的接缝处画一段颈部 —— 两条凹曲线夹出的腰形面，
 *   用线性渐变填充，渐变两端分别取左右相邻段的填充色。
 *   闭合接缝的段位重叠 1px：贴合不留发丝缝（浏览器亚像素取整会在 0 间隙处漏一道白线）。
 */

/**
 * 闭合接缝的段位偏移（px）：负值 = 相邻两段互相压住 1px。
 *
 * 为什么不是 0：`margin-left: 0` 时两段的边线落在同一坐标，亚像素取整会把
 * 抗锯齿的白边露出来（"发丝缝"）。压 1px 让后一段盖住前一段的边。
 */
export const NECK_CLOSED_GAP = -1;

/** 张开量（px）：激活段两侧接缝的分离距离，也是颈部的标称宽度。 */
export const NECK_SEPARATION_PX = 12;

/**
 * 颈部的标称高度（viewBox 单位，不是像素）。
 *
 * 真实高度由 CSS 拉伸（`preserveAspectRatio="none"`）：分段条在高屏/窄屏下高度会变，
 * 把高度写进路径反而会在换行或字号变化时错位。所以路径只描述"比例"，高度交给 CSS。
 */
export const NECK_NOMINAL_SPAN = 100;

/** 闭合段的圆角（px）：圆角阶梯的 `--radius-sm`（control 档）。 */
export const NECK_RADIUS_IDLE_PX = 8;

/** 激活块的圆角（px）：涨到圆角阶梯的 `--radius-md`（field 档），弹簧过渡。 */
export const NECK_RADIUS_ACTIVE_PX = 14;

/**
 * 单侧"收腰"程度：三次贝塞尔的两个控制点都拉到 gap 的这个比例处。
 *
 * 取 1/3 是算出来的，不是调的：控制点同 x 时曲线的 t=0.5 横坐标是控制点偏移的 0.75，
 * 于是中段收腰量 = 2 × 0.75 × (1/3) × gap = gap/2 —— 颈部正中留半个间隙宽，
 * 再窄会看起来要断，再宽就不像"被拉住"。
 */
const NECK_BOW_RATIO = 1 / 3;

/** 激活块的填充（深墨面）—— 颈部的两端取它和纸面。 */
const FILL_ACTIVE = "var(--color-surface-inverse)";

/** 未激活段/分段条自身的填充（纸面）—— 颈部的另一端取它。 */
const FILL_IDLE = "var(--color-surface)";

/** 一段的到位状态：段位位移 + 圆角 + 颈部的实时宽度（同一组弹簧一起走）。 */
export interface NeckSegmentMotion extends Record<string, string> {
  marginLeft: string;
  borderRadius: string;
  /** 颈部实时宽度（px）：`left`/`width` 都读它，闭合时为 0。 */
  "--neck-gap": string;
}

/** 颈部渐变的两个端点（语义 token 名，不是色值）。 */
export interface NeckGradientStops {
  from: string;
  to: string;
}

/** 数字 → CSS 字面量：两位小数、去掉多余的 0，且不产出 `-0`。 */
function cssNumber(value: number): number {
  const rounded = Number(value.toFixed(2));
  return rounded === 0 ? 0 : rounded;
}

/**
 * 一段颈部的 SVG 路径（`d`）：两条凹曲线夹出的腰形，闭合。
 *
 * 坐标系：原点在左邻段右上角，`gap` 是接缝的横向间隙，`span` 是标称高度。
 * 左边界从 (0,0) 弯到 (0,span)，右边界镜像 —— 两条曲线都向中轴内收，于是
 * 上下贴住相邻两段、正中收成半宽的腰。
 *
 * **纯函数**：间隙 ≤ 0、任一侧不是有限数、高度 ≤ 0 时返回空串 ——
 * 退化输入宁可"不画"，也不能把 NaN 坐标交给浏览器（那会让整条 SVG 失联）。
 */
export function neckPath(gap: number, span: number): string {
  if (!Number.isFinite(gap) || !Number.isFinite(span) || gap <= 0 || span <= 0) return "";

  const bow = gap * NECK_BOW_RATIO;
  const lower = span / 3;
  const upper = (span * 2) / 3;
  const mirror = gap - bow;

  return [
    "M0 0",
    `C${cssNumber(bow)} ${cssNumber(lower)} ${cssNumber(bow)} ${cssNumber(upper)} 0 ${cssNumber(span)}`,
    `L${cssNumber(gap)} ${cssNumber(span)}`,
    `C${cssNumber(mirror)} ${cssNumber(upper)} ${cssNumber(mirror)} ${cssNumber(lower)} ${cssNumber(gap)} 0`,
    "Z",
  ].join(" ");
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
export function neckSegmentGap(index: number, activeIndex: number): number {
  if (!Number.isFinite(index) || index <= 0) return 0;
  const touchesActive = index === activeIndex || index === activeIndex + 1;
  return touchesActive ? NECK_SEPARATION_PX : NECK_CLOSED_GAP;
}

/** 这条接缝是否张开（颈部只画在张开的接缝上）。 */
export function isNeckSeamOpen(index: number, activeIndex: number): boolean {
  return neckSegmentGap(index, activeIndex) > 0;
}

/**
 * 第 `index` 段到位后的样式（弹簧的终点）。
 *
 * `--neck-gap` 与 `marginLeft` 用**同一组弹簧**驱动：颈部的 left/width 读这个变量，
 * 于是它在整段过渡里始终等于真实间隙，不会在弹性回弹时缩错位置。
 *
 * 圆角只跟"是不是激活块"走（被拉开的邻段只是让位，不涨圆角）—— 两个属性分开表达，
 * 断言的语义也更清楚：位移看接缝，圆角看激活。
 */
export function neckSegmentMotion(index: number, activeIndex: number): NeckSegmentMotion {
  const gap = neckSegmentGap(index, activeIndex);

  return {
    marginLeft: `${gap}px`,
    borderRadius: `${index === activeIndex ? NECK_RADIUS_ACTIVE_PX : NECK_RADIUS_IDLE_PX}px`,
    "--neck-gap": `${gap > 0 ? gap : 0}px`,
  };
}

/**
 * 颈部/段位的过渡：要么 `SPRING_NECK`，要么「减少动态」下的落位。
 *
 * 直接复用 `motion.ts` 的 `resolveSpringMotion`：reduce 时给的是时长 0 的 tween
 * （**落位，不是放慢的弹簧**），与 ADR-0012 后续修订（2026-11-07）的口径一致。
 */
export function neckTransition(prefersReducedMotion: boolean): SpringOrJumpTransition {
  return resolveSpringMotion(SPRING_NECK, prefersReducedMotion);
}

/** 过渡的形态名（`spring` / `jump`）：挂在 DOM 上供设计门禁与调试观察。 */
export function neckMotionKind(prefersReducedMotion: boolean): "spring" | "jump" {
  return prefersReducedMotion ? "jump" : "spring";
}

/**
 * 第 `index` 段左侧接缝的颈部渐变端点。
 *
 * 左端点取左邻段的填充色、右端点取右邻段的填充色 —— 激活块是深墨面，未激活段是
 * 纸面，所以两个端点就是这两支语义 token 的排列，不新增"激活垫"色相。
 */
export function neckGradientStops(index: number, activeIndex: number): NeckGradientStops {
  const leftNeighbourActive = index - 1 === activeIndex;

  return leftNeighbourActive
    ? { from: FILL_ACTIVE, to: FILL_IDLE }
    : { from: FILL_IDLE, to: FILL_ACTIVE };
}
