import { resolveSpringMotion, SPRING_NECK, type SpringOrJumpTransition } from "~/utils/motion";

/**
 * 消息中心分段条的纯几何与纯接线（票 297 建立，本次按参考组件的 md 档返工）。
 *
 * 分工与 `folderCardGeometry` 一致：
 *   - 这里只做算术与判定（纯函数，可单测，SSR 与客户端算出的结果逐字节相同）；
 *   - 组件只消费这里的结果，不在模板里写几何字面量；
 *   - 弹簧参数**只从 `utils/motion.ts` 的预设取**（`SPRING_NECK`），本文件不出现任何
 *     stiffness / damping 数字。`test/design/motion-spring-gates.test.ts` 会扫这一条。
 *
 * 形态（参考组件 GooeyNav；本票只复刻形态）：
 *   五段**相邻贴合**成一条分段控件；只有激活段两侧的接缝"张开"（张开量 = md 档的
 *   `separation` = 20px），张开的接缝里画一段颈部 —— 两条凹曲线夹出的腰形面，用线性渐变
 *   填充，渐变两端分别取左右相邻段的填充色。
 *   圆角只在**敞口**的一侧给：张开的接缝旁与控件两端是圆的，闭合接缝旁必须是 0，
 *   否则相邻两段的圆角会在贴合处啃出四个缺口，整条就读成"五个胶囊"而不是"一条"。
 *   颈部上下各内缩一个圆角半径（`NECK_END_INSET_PX`）：两端只落在段边的直线段上，
 *   于是圆角完整保留、圆弧与颈部之间也不会空出月牙形的页底色。
 *   闭合接缝的段位重叠 1px：贴合不留发丝缝（浏览器亚像素取整会在 0 间隙处漏一道白线）。
 */

/**
 * 闭合接缝的段位偏移（px）：负值 = 相邻两段互相压住 1px。
 *
 * 为什么不是 0：`margin-left: 0` 时两段的边线落在同一坐标，亚像素取整会把
 * 抗锯齿的白边露出来（"发丝缝"）。压 1px 让后一段盖住前一段的边。
 */
export const NECK_CLOSED_GAP = -1;

/**
 * 张开量（px）：激活段两侧接缝的分离距离，也是颈部的标称宽度。
 *
 * 取参考组件 `md` 档的 `separation` = 20。原先的 12 太窄：20px 的间隙才容得下
 * "两端满高、中间收腰"的那条曲线，12px 下曲线被压成一根发丝，看起来像没张开。
 */
export const NECK_SEPARATION_PX = 20;

/**
 * 颈部的标称坐标高度（viewBox 单位，不是像素）。
 *
 * 真实高度由 CSS 拉伸（`preserveAspectRatio="none"`）：分段条在高屏/窄屏下高度会变，
 * 把高度写进路径反而会在换行或字号变化时错位。所以路径只描述"比例"，高度交给 CSS。
 */
export const NECK_NOMINAL_SPAN = 100;

/**
 * 收腰后的最薄处（标称单位，相对**颈部自己的高度**）：55%。
 *
 * 颈部的端点是"被两段拉住"的锚点、正中是"被拉长"的证据；55% 还能看出收腰，
 * 再细会读成断线。
 */
export const NECK_WAIST = 55;

/**
 * 段的敞口圆角（px）：圆角阶梯的 `--radius-md`（field 档）。
 *
 * 只给张开的接缝一侧与控件两端；闭合接缝一侧恒为 0（见文件头的"为什么要分侧"）。
 */
export const NECK_CORNER_PX = 14;

/**
 * 颈部上下各内缩多少（px）：**等于段的敞口圆角**（`--radius-md`）。
 *
 * 为什么必须内缩到圆角的切线以内：段在接缝侧是圆角，圆角的**直线段**只从 y = r 开始。
 * 颈部端面如果按"满高"顶到条带的上下沿，端面就落在圆弧**外面**，圆弧与端面之间必然
 * 空出一个月牙形的页底色（深墨块旁边就是那块浅色"楔形"）。两种修法只能二选一：
 *   - 把月牙填掉 ⇒ 填色与块同色，圆弧被抹平，块成了方角矩形；
 *   - 让颈部端面收在切线以内 ⇒ 圆弧完整保留，月牙根本不会产生。
 * 这里选后者：颈部两端落在段边的**直线段**上，两端仍是颈部最厚处、正中收腰。
 */
export const NECK_END_INSET_PX = NECK_CORNER_PX;

/** 颈部的 viewBox：横向也按标称值，实际宽度由 --neck-gap 拉伸（x 比例跟着间隙走）。 */
export const NECK_VIEW_BOX = `0 0 ${NECK_NOMINAL_SPAN} ${NECK_NOMINAL_SPAN}`;

/** 激活块的填充（深墨面）—— 颈部的两端取它和纸面。 */
const FILL_ACTIVE = "var(--color-surface-inverse)";

/** 未激活段（分段条的轨道面）的填充 —— 颈部的另一端取它。 */
const FILL_IDLE = "var(--color-surface-raised)";

/**
 * 一段的到位状态：**接缝间隙 + 四角圆角**（同一组弹簧一起走）。
 *
 * `--neck-gap` 是这批值里唯一表达"间隙"的量，而且它可以是负的（闭合接缝的 -1px 重叠）。
 * 段位（`margin-left`）与颈部的宽度/位置都**从它派生**（见 tailwind.css 的
 * `.message-center__subnav-segment` / `.message-center-tab__neck`），而不是各写一份：
 *   - 写两份时，一个取原始值、一个取 `max(0, …)`，两侧起点不同 → 过渡中颈部会比真实
 *     间隙宽出最多 1px，浅色的那一端就从激活块的边上漏出来；反过来窄 1px 又会留一道
 *     发丝。单一变量就没有这个自由度。
 */
export interface NeckSegmentMotion extends Record<string, string> {
  borderTopLeftRadius: string;
  borderTopRightRadius: string;
  borderBottomRightRadius: string;
  borderBottomLeftRadius: string;
  /** 接缝间隙（px，可为负）：段位与颈部几何都读它。 */
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
 * 坐标系：宽高都是标称值（`span`），原点在左端上沿。上边界从 (0,0) 经中点控制点压到
 * 中部再抬回 (span,0)，下边界镜像 —— 于是**两端是颈部自己的满高、正中收腰**，像被两段
 * 拉住的液体。"满高"指颈部这条带子自己被拉满（带子上下沿 = 内缩后的段边直线段端点），
 * 不是条带的上下沿 —— 见 `NECK_END_INSET_PX` 里为什么必须内缩。
 * 真实宽高由 CSS 拉伸（`preserveAspectRatio="none"`），路径本身不掺像素。
 *
 * **纯函数**：`waist` 不在 (0, span) 开区间内、`span` 非正或非有限数时返回空串 ——
 * 退化输入宁可"不画"，也不能把 NaN 坐标交给浏览器（那会让整条 SVG 失联）。
 */
export function neckPath(waist: number = NECK_WAIST, span: number = NECK_NOMINAL_SPAN): string {
  if (!Number.isFinite(waist) || !Number.isFinite(span)) return "";
  if (span <= 0 || waist <= 0 || waist >= span) return "";

  const mid = cssNumber(span / 2);
  const lower = cssNumber(waist);
  const edge = cssNumber(span);

  return [
    "M0 0",
    `Q${mid} ${cssNumber(span - waist)} ${edge} 0`,
    `L${edge} ${edge}`,
    `Q${mid} ${lower} 0 ${edge}`,
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
 * 第 `seam` 条接缝（0..count）该不该给圆角。
 *
 * 控件两端的"外边"（0 与 count）永远敞口；中间的接缝只在张开时敞口。
 * 闭合接缝旁给圆角会在贴合处啃出缺口，所以那里必须是 0。
 */
export function neckSeamCorner(seam: number, activeIndex: number, count: number): number {
  if (!Number.isFinite(seam) || !Number.isFinite(count) || count <= 0) return 0;
  const edge = seam <= 0 || seam >= count;
  return edge || isNeckSeamOpen(seam, activeIndex) ? NECK_CORNER_PX : 0;
}

/**
 * 第 `index` 段到位后的样式（弹簧的终点）。
 *
 * 只给**一份**间隙（`--neck-gap`，可为负）+ 四角圆角；段位与颈部的宽/位都在 CSS 里
 * 从这份间隙派生（`margin-left` 取原值、颈部取 `max(0px, …)`），所以回弹期间两者
 * 不可能互相错开。
 *
 * 四角圆角分开给：左接缝决定左侧两角、右接缝决定右侧两角（`count` 用来认控件两端）。
 */
export function neckSegmentMotion(
  index: number,
  activeIndex: number,
  count: number,
): NeckSegmentMotion {
  const gap = neckSegmentGap(index, activeIndex);
  const left = neckSeamCorner(index, activeIndex, count);
  const right = neckSeamCorner(index + 1, activeIndex, count);

  return {
    borderTopLeftRadius: `${left}px`,
    borderTopRightRadius: `${right}px`,
    borderBottomRightRadius: `${right}px`,
    borderBottomLeftRadius: `${left}px`,
    "--neck-gap": `${gap}px`,
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
 * 轨道面，所以两个端点就是这两支语义 token 的排列，不新增"激活垫"色相。
 *
 * 闭合接缝（两侧都不是激活块）两端同为轨道面：过渡期间它会被拉成一条同色的桥，
 * 于是"连上"那一刻不会闪出第二种颜色。
 */
export function neckGradientStops(index: number, activeIndex: number): NeckGradientStops {
  const leftNeighbourActive = index - 1 === activeIndex;
  const rightNeighbourActive = index === activeIndex;

  return {
    from: leftNeighbourActive ? FILL_ACTIVE : FILL_IDLE,
    to: rightNeighbourActive ? FILL_ACTIVE : FILL_IDLE,
  };
}
