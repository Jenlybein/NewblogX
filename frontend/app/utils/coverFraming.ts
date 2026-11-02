import type { CSSProperties } from "vue";

/**
 * 文章封面的 16:9 取景算术。
 *
 * 与后台轮播取景窗（frontend_admin 的 framing 工具）同一套做法：
 * 取景不裁第二份图，线上是按「焦点 + 缩放」三个数用 CSS 复现同一块取景。
 * 于是"这块图会露出什么"完全由算术决定；算术只写在组件里时，只能靠量像素来测。
 *
 * 这里只做三件事，组件只管交互：
 *   1. 比例是否与 16:9 合规（±1% 容差）；
 *   2. 参数钳制（焦点恒在 0–1、缩放恒在 1–2）；
 *   3. 拖动像素 → 焦点、缩放步进、方向键平移的换算，以及「取景参数 → CSS」。
 *
 * 本轮只把取景结果用于取景窗里的预览；参数是否落库由后续接入决定，
 * 所以这个模块不碰任何存储字段。
 */

/** 封面的固定比例，全站唯一取值点（写进 CSS 时取同一个字符串）。 */
export const COVER_FRAME_RATIO = "16 / 9";

/** 同一比例的数值形式（几何换算用，避免各组件再写一遍 16/9）。 */
export const COVER_ASPECT = 16 / 9;

/** 比例容差：与 16:9 的相对偏差在 ±1% 内视为合规（1919×1079 这类噪声不该弹取景窗）。 */
export const COVER_ASPECT_TOLERANCE = 0.01;

/** 缩放下限：1.0 表示"铺满取景框"（cover）。低于 1 就会露白，因此它同时是安全下限。 */
export const COVER_ZOOM_MIN = 1;

/** 缩放上限：2.0。再往上就是把马赛克放大，看不到更多信息。 */
export const COVER_ZOOM_MAX = 2;

/** 缩放步进：滑杆、加减号之外的所有缩放都按这个粒度落值。 */
export const COVER_ZOOM_STEP = 0.05;

/** 焦点保留到小数点后 4 位，避免浮点尾巴写进 CSS。 */
const FOCUS_DIGITS = 4;

/** 缩放保留到小数点后 2 位，刚好装得下 0.05 的步进。 */
const ZOOM_DIGITS = 2;

/** 方向键一次平移的行程（焦点比例）：按 50 次从一头走到另一头。 */
export const KEY_NUDGE_STEP = 0.02;

/** 取景参数：焦点是"图上哪一点对齐到取景框的同一点"，缩放是再放大多少倍。 */
export interface Framing {
  focus_x: number;
  focus_y: number;
  zoom: number;
}

/** 像素尺寸（原图或取景框）。 */
export interface Size {
  width: number;
  height: number;
}

/** 缺省取景：居中、不放大——首次打开与没有取景记录时用的就是它。 */
export const DEFAULT_FRAMING: Framing = { focus_x: 0.5, focus_y: 0.5, zoom: 1 };

/** 四舍五入到固定小数位：把浮点尾巴挡在写进 CSS 之前。 */
function roundTo(value: number, digits: number): number {
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

/**
 * 尺寸是否与 16:9 合规（相对偏差 ≤ 1%，闭区间）。
 *
 * 判据交叉相乘后比较，不做除法：`|w·9 − h·16| ≤ tol·16·h`。
 * 边界值 1616×900 恰好是 +1%；除法形式在边界上会漂成 1.0000000000000009，
 * 于是"正好 ±1%"被误判成不合规。尺寸非法一律判不合规——这种图本来就没法取景。
 */
export function isAspectCompliant(width: number, height: number, tolerance: number = COVER_ASPECT_TOLERANCE): boolean {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return false;
  }
  const numerator = Math.abs(width * 9 - height * 16);
  const denominator = tolerance * 16 * height;
  return numerator <= denominator;
}

/**
 * 钳制焦点到 0–1：界外贴边而不是回中（拖到角落就停在角落）；
 * 非法值（NaN / Infinity）才回中——那是"算不出来"，回 0 会把图顶到左上角。
 */
export function clampFocus(value: number): number {
  if (!Number.isFinite(value)) return 0.5;
  return roundTo(Math.min(1, Math.max(0, value)), FOCUS_DIGITS);
}

/**
 * 钳制缩放到 1–2。
 *
 * 刻意不吸附到 0.05：步进是交互粒度，不是存储约束。已经存着的 1.37
 * 若被吸附成 1.35，等于"每次打开取景窗都改一次线上效果"。
 */
export function clampZoom(value: number): number {
  if (!Number.isFinite(value)) return COVER_ZOOM_MIN;
  return roundTo(Math.min(COVER_ZOOM_MAX, Math.max(COVER_ZOOM_MIN, value)), ZOOM_DIGITS);
}

/** 吸附到 0.05 步进（给滑杆 / 加减号用），顺带钳进 1–2。 */
export function snapZoom(value: number): number {
  const safe = Number.isFinite(value) ? value : COVER_ZOOM_MIN;
  return clampZoom(Math.round(safe / COVER_ZOOM_STEP) * COVER_ZOOM_STEP);
}

/** 把任意来源（缺字段、用户输入、旧参数）的取景收成合法的三个数。 */
export function clampFraming(framing?: Partial<Framing> | null): Framing {
  return {
    focus_x: clampFocus(framing?.focus_x ?? DEFAULT_FRAMING.focus_x),
    focus_y: clampFocus(framing?.focus_y ?? DEFAULT_FRAMING.focus_y),
    zoom: clampZoom(framing?.zoom ?? DEFAULT_FRAMING.zoom),
  };
}

/**
 * 缩放走一格（`steps` 为 +1 / -1）。
 *
 * 只改 zoom、焦点不动：焦点由 `framingStyle` 的 `transform-origin` 承担，
 * 所以放大时那一钉点原地不动，不会跑框。
 */
export function stepFramingZoom(framing: Framing, steps: number): Framing {
  const base = clampFraming(framing);
  const safeSteps = Number.isFinite(steps) ? steps : 0;
  return { ...base, zoom: snapZoom(base.zoom + safeSteps * COVER_ZOOM_STEP) };
}

function isPositiveSize(size?: Size | null): size is Size {
  return Boolean(size) && Number.isFinite(size!.width) && Number.isFinite(size!.height) && size!.width > 0 && size!.height > 0;
}

/**
 * cover 的放大倍率：`object-fit: cover` 把原图等比缩放到刚好铺满取景框，
 * 取两轴里更大的比例，于是另一轴必然溢出——溢出的部分才是"可拖动的余量"。
 */
function coverScale(image: Size, frame: Size): number {
  return Math.max(frame.width / image.width, frame.height / image.height);
}

/**
 * 拖动像素 → 焦点。`dx`/`dy` 是**把图往哪边推**的像素数（与手指方向一致）。
 *
 * 分母用缩放后的溢出量：`zoom*绘制尺寸 − 框尺寸`。用未缩放的溢出量换算，
 * 放大时图会比光标走得慢；用缩放后的余量做分母，拖拽始终 1:1 跟手。
 * 分母为 0（该轴正好铺满）时不动——本来就没有可以露出来的部分。
 */
export function panFraming(framing: Framing, dx: number, dy: number, frame: Size, image: Size): Framing {
  const base = clampFraming(framing);
  if (!isPositiveSize(frame) || !isPositiveSize(image) || !Number.isFinite(dx) || !Number.isFinite(dy)) {
    return base;
  }
  const scale = coverScale(image, frame);
  const roomX = base.zoom * image.width * scale - frame.width;
  const roomY = base.zoom * image.height * scale - frame.height;
  return clampFraming({
    focus_x: roomX > 0 ? base.focus_x - dx / roomX : base.focus_x,
    focus_y: roomY > 0 ? base.focus_y - dy / roomY : base.focus_y,
    zoom: base.zoom,
  });
}

/**
 * 方向键平移：`stepsX`/`stepsY` 是"取景范围朝这个方向挪几格"。
 *
 * 与 `panFraming` 的方向约定不同是**有意的**：拖动是"推图"，方向键是"移框"；
 * 提示文案因此写"方向键移动取景范围"，两条各说一句话就讲清。
 */
export function nudgeFraming(framing: Framing, stepsX: number, stepsY: number): Framing {
  const base = clampFraming(framing);
  const x = Number.isFinite(stepsX) ? stepsX : 0;
  const y = Number.isFinite(stepsY) ? stepsY : 0;
  return clampFraming({
    focus_x: base.focus_x + x * KEY_NUDGE_STEP,
    focus_y: base.focus_y + y * KEY_NUDGE_STEP,
    zoom: base.zoom,
  });
}

/** 百分比字串：保留两位小数，顺手去掉 50.00 → 50 这类尾巴。 */
function toPercent(value: number): string {
  return `${roundTo(value * 100, 2)}%`;
}

/**
 * 取景参数 → CSS 声明。这是"预览几何 = 最终渲染几何"的唯一出口：
 *
 * 只输出百分比与 `scale()`，不含任何像素值，所以同一组参数在取景窗里、
 * 列表缩略图上和任意宽度的详情页封面里得到的是同一块取景。
 * `object-fit: cover` 铺满，`object-position` 与 `transform-origin` 取同一个
 * 比例点：前者把图上这一点对到框内这一点，后者让放大围着这一点。
 */
export function framingStyle(framing?: Partial<Framing> | null): CSSProperties {
  const { focus_x, focus_y, zoom } = clampFraming(framing);
  const position = `${toPercent(focus_x)} ${toPercent(focus_y)}`;
  return {
    objectFit: "cover",
    objectPosition: position,
    transformOrigin: position,
    transform: `scale(${zoom})`,
  };
}
