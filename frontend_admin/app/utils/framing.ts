/**
 * 取景（framing）算术。
 *
 * 为什么单独抽成一个纯函数模块：取景的结果**不上传第二份图**，线上是按
 * 「焦点 + 缩放」三个数用 CSS 复现同一块取景（见 issue #44）。于是"这块图
 * 会露出什么"完全由算术决定，而算术一旦只写在组件里，就只能靠挂载组件、量
 * 像素来测——量不准，也拦不住"拖到角落露白"这类事故。
 *
 * 这里的函数只做三件事，组件只管交互：
 *   1. 比例是否合规（16:9，容差 ±1%）；
 *   2. 参数钳制（焦点恒在 0–1、缩放恒在 1–2、步进 0.05）；
 *   3. 锚点与缩放的边界换算（拖动像素 → 焦点；缩放 → 焦点保持不动的位置）。
 *
 * 术语见 `GLOSSARY.md`：取景 / 焦点 / 缩放 / 轮播封面。
 */

import type { CSSProperties } from 'vue'

/** 轮播封面的固定比例，全站唯一取值点（后台与读者站各自一份，跨端靠门禁对齐）。 */
export const BANNER_ASPECT = 16 / 9

/** 比例容差：与 16:9 的相对偏差在 ±1% 内视为合规（1919×1079 这类噪声不该弹取景窗）。 */
export const ASPECT_TOLERANCE = 0.01

/** 缩放下限：1.0 表示"铺满取景框"（cover）。低于 1 就会露白，因此它同时是安全下限。 */
export const ZOOM_MIN = 1

/** 缩放上限：2.0。再往上就是把马赛克放大，运营看不到更多信息。 */
export const ZOOM_MAX = 2

/** 缩放步进：滑杆、加减号、方向键之外的所有缩放都按这个粒度落值。 */
export const ZOOM_STEP = 0.05

/** 焦点（0–1 的一张图内部坐标）保留到小数点后 4 位，避免浮点尾巴写进 CSS。 */
const FOCUS_DIGITS = 4

/** 缩放保留到小数点后 2 位，刚好装得下 0.05 的步进。 */
const ZOOM_DIGITS = 2

/** 取景参数：焦点是"图上哪一点对齐到取景框的同一点"，缩放是再放大多少倍。 */
export interface Framing {
  focus_x: number
  focus_y: number
  zoom: number
}

/** 像素尺寸（原图或取景框）。 */
export interface Size {
  width: number
  height: number
}

/** 缺省取景：居中、不放大——历史图没有取景记录时用的就是它。 */
export const DEFAULT_FRAMING: Framing = { focus_x: 0.5, focus_y: 0.5, zoom: 1 }

/**
 * 比例是否与 16:9 合规（相对偏差 ≤ 1%，闭区间）。
 *
 * 用相对偏差而不是绝对差：`16/9` 是无限小数，用绝对差会让合规线随比例大小漂移。
 * 尺寸非法（0 / 负数 / NaN / Infinity）一律判不合规——这种图本来就没法取景。
 */
export function isAspectCompliant(width: number, height: number, tolerance: number = ASPECT_TOLERANCE): boolean {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return false
  }
  // 判据交叉相乘后比较，不做除法：
  //   |w/h − 16/9| ≤ tol·16/9  ⇔  |w·9 − h·16| ≤ tol·16·h
  // 除法形式在边界上会飘（1616×900 恰好 +1%，`1616/900/(16/9) - 1` 却算成
  // 0.010000000000000009，于是"正好 ±1%"被误判成不合规）。乘法这边全是整数，
  // 边界值落在哪一侧是确定的。
  const numerator = Math.abs(width * 9 - height * 16)
  const denominator = tolerance * 16 * height
  return numerator <= denominator
}

/** 四舍五入到固定小数位：把浮点尾巴挡在写进 CSS / 存进库之前。 */
function roundTo(value: number, digits: number): number {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

/**
 * 钳制焦点到 0–1。
 *
 * 界外贴边而不是"钳到中线"：运营拖到角落时希望图跟着停在角落，
 * 而不是弹回中间。非法值（NaN / Infinity）才回中——那是"算不出来"，
 * 回 0 会把图顶到左上角，看起来像 bug。
 */
export function clampFocus(value: number): number {
  if (!Number.isFinite(value)) return 0.5
  return roundTo(Math.min(1, Math.max(0, value)), FOCUS_DIGITS)
}

/**
 * 钳制缩放到 1–2。
 *
 * 刻意**不**吸附到 0.05：步进是交互粒度（滑杆、加减号），不是存储约束。
 * 已经存着的 1.37 若被吸附成 1.35，等于"每次打开取景窗都改一次线上效果"。
 */
export function clampZoom(value: number): number {
  if (!Number.isFinite(value)) return ZOOM_MIN
  return roundTo(Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, value)), ZOOM_DIGITS)
}

/** 吸附到 0.05 的步进（给滑杆 / 加减号 / 方向键用），顺带钳进 1–2。 */
export function snapZoom(value: number): number {
  const safe = Number.isFinite(value) ? value : ZOOM_MIN
  return clampZoom(Math.round(safe / ZOOM_STEP) * ZOOM_STEP)
}

/** 把任意来源（缺字段、库里的旧值、用户输入）的取景参数收成合法的三个数。 */
export function clampFraming(framing: Partial<Framing> | null | undefined): Framing {
  return {
    focus_x: clampFocus(framing?.focus_x ?? DEFAULT_FRAMING.focus_x),
    focus_y: clampFocus(framing?.focus_y ?? DEFAULT_FRAMING.focus_y),
    zoom: clampZoom(framing?.zoom ?? DEFAULT_FRAMING.zoom),
  }
}

/**
 * 缩放走一格（`steps` 为 +1 / -1，或滑杆直接给绝对值时为 0）。
 *
 * 缩放只改 zoom，焦点不动：焦点是"图上哪一点钉在取景框的同一点"，
 * 它由 `framingStyle` 的 `transform-origin` 承担，所以放大时那一钉点原地不动，
 * 不会跑框。
 */
export function stepFramingZoom(framing: Framing, steps: number): Framing {
  const base = clampFraming(framing)
  const safeSteps = Number.isFinite(steps) ? steps : 0
  return { ...base, zoom: snapZoom(base.zoom + safeSteps * ZOOM_STEP) }
}

/** 方向键一次平移的行程（焦点比例）：按 50 次从一头走到另一头，够细也够快。 */
export const KEY_NUDGE_STEP = 0.02

function isPositiveSize(size: Size | null | undefined): size is Size {
  return Boolean(size) && Number.isFinite(size!.width) && Number.isFinite(size!.height) && size!.width > 0 && size!.height > 0
}

/**
 * cover 的放大倍率：`object-fit: cover` 把原图等比缩放到刚好铺满取景框，
 * 取两轴里更大的那个比例，于是另一轴必然溢出——溢出的部分才是"可拖动的余量"。
 */
function coverScale(image: Size, frame: Size): number {
  return Math.max(frame.width / image.width, frame.height / image.height)
}

/**
 * 拖动像素 → 焦点。`dx`/`dy` 是**把图往哪边推**的像素数（与手指方向一致）。
 *
 * 为什么不是 `dx / 溢出量`：`transform: scale(zoom)` 之后，屏幕上看到的溢出
 * 是 `zoom*绘制尺寸 − 框尺寸`，用未缩放的溢出量换算会让图比光标走得慢（放大时
 * 尤其明显）。这里用缩放后的余量做分母，拖拽始终 1:1 跟手。
 *
 * 分母为 0（该轴正好铺满，比如 16:9 图放在 zoom 1）时不动——本来就没有可以
 * 露出来的部分，硬拖只会露出空白。
 */
export function panFraming(framing: Framing, dx: number, dy: number, frame: Size, image: Size): Framing {
  const base = clampFraming(framing)
  if (!isPositiveSize(frame) || !isPositiveSize(image) || !Number.isFinite(dx) || !Number.isFinite(dy)) {
    return base
  }
  const scale = coverScale(image, frame)
  const roomX = base.zoom * image.width * scale - frame.width
  const roomY = base.zoom * image.height * scale - frame.height
  return clampFraming({
    focus_x: roomX > 0 ? base.focus_x - dx / roomX : base.focus_x,
    focus_y: roomY > 0 ? base.focus_y - dy / roomY : base.focus_y,
    zoom: base.zoom,
  })
}

/**
 * 方向键平移：`stepsX`/`stepsY` 是"取景范围朝这个方向挪几格"。
 *
 * 注意与 `panFraming` 的方向约定不同，这是有意的：拖动是**推图**（把图往右推
 * 就看见左边），方向键是**移框**（按右箭头就看见右边）。两条都各说一句话就
 * 讲清，混着用才讲不清；提示文案因此写"方向键移动取景范围"。
 *
 * 按**焦点比例**走而不是按像素走：键盘平移要的是"按一下挪一点"的手感，
 * 若换算成像素，超宽图（溢出几千 px）按一次几乎不动、窄图一次跳一大截。
 */
export function nudgeFraming(framing: Framing, stepsX: number, stepsY: number): Framing {
  const base = clampFraming(framing)
  const x = Number.isFinite(stepsX) ? stepsX : 0
  const y = Number.isFinite(stepsY) ? stepsY : 0
  return clampFraming({
    focus_x: base.focus_x + x * KEY_NUDGE_STEP,
    focus_y: base.focus_y + y * KEY_NUDGE_STEP,
    zoom: base.zoom,
  })
}

/** 屏幕矩形，坐标以取景框左上角为原点（图片框外的部分是负数）。 */
export interface Rect {
  left: number
  top: number
  width: number
  height: number
}

/**
 * 算出图片在取景框里的**最终**矩形（已含 cover 与 zoom）。
 *
 * 这是"不露白"的判据：图片矩形必须完全盖住 `0..frame.width × 0..frame.height`。
 * 它之所以恒成立，是因为 `zoom ≥ 1` 只放大不缩小、`focus ∈ 0–1` 只会让图往里
 * 挪到边界为止——两条都写进钳制里了，所以组件不需要在拖动时再补一层保险。
 */
export function framedImageRect(image: Size, frame: Size, framing: Framing): Rect {
  if (!isPositiveSize(image) || !isPositiveSize(frame)) {
    return { left: 0, top: 0, width: 0, height: 0 }
  }
  const { focus_x, focus_y, zoom } = clampFraming(framing)
  const scale = coverScale(image, frame)
  const drawnWidth = image.width * scale
  const drawnHeight = image.height * scale
  // object-position: p% 把图上该点对到框内该点 → 未缩放时的左上角偏移。
  const offsetX = -focus_x * (drawnWidth - frame.width)
  const offsetY = -focus_y * (drawnHeight - frame.height)
  // transform-origin 也在同一比例点上，于是缩放是把矩形往两边撑开。
  const originX = focus_x * frame.width
  const originY = focus_y * frame.height
  return {
    left: roundTo(originX + (offsetX - originX) * zoom, 2),
    top: roundTo(originY + (offsetY - originY) * zoom, 2),
    width: roundTo(drawnWidth * zoom, 2),
    height: roundTo(drawnHeight * zoom, 2),
  }
}

/**
 * 预览要用的四条 CSS 声明（读者站用同一套语义复现同一块取景）。
 *
 * 类型直接用 Vue 的 `CSSProperties`（只 import type，运行时没有依赖）：
 * 这四条本来就是"能贴到 :style 上"的东西，自带一个同形 interface 反而
 * 在模板里赋不进 `:style`——类型系统会要求它跟 CSSProperties 逐键兼容。
 */
export type FramingStyle = Pick<CSSProperties, 'objectFit' | 'objectPosition' | 'transformOrigin' | 'transform'>

/** 百分比字串：保留两位小数，顺手去掉 50.00 → 50 这类尾巴。 */
function toPercent(value: number): string {
  return `${roundTo(value * 100, 2)}%`
}

/**
 * 取景参数 → CSS 声明。这是"预览几何 = 最终渲染几何"的唯一出口：
 *
 * 只输出百分比与 `scale()`，**不含任何像素值**，所以同一组参数在预览框里和
 * 线上任意宽度的面板里得到的是同一块取景。读者站（#46）照抄这四条即可，
 * 不需要第二套算术。
 *
 * `object-fit: cover` 铺满、`object-position` 与 `transform-origin` 取同一个
 * 比例点：前者把图上这一点对到框内这一点，后者让放大围着这一点，于是
 * "焦点钉住、图变大"。
 */
export function framingStyle(framing: Framing): FramingStyle {
  const { focus_x, focus_y, zoom } = clampFraming(framing)
  const position = `${toPercent(focus_x)} ${toPercent(focus_y)}`
  return {
    objectFit: 'cover',
    objectPosition: position,
    transformOrigin: position,
    transform: `scale(${zoom})`,
  }
}
