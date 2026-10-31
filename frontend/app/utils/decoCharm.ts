/**
 * 光标挂件（cursor charm）的参数与几何（纯函数）。
 *
 * 为什么单独成模块：挂件里所有"算错就看得见"的决定都在这里 —— 门控三条件、
 * 站标尺寸钳位、站标落点、挂绳路径。它们全是纯函数，所以可以脱离浏览器
 * 逐点断言；组件与组合式只负责接线（事件、生命周期、淡入）。
 *
 * 本票**不做物理**：站标固定在指针下方一个绳长处，挂绳就是一条直线。
 * 逐帧的 Verlet 链、弧垂、停帧都留给票 #41，届时这里会多出积分器，
 * 而 `charmLogoCenter` / `charmTetherPath` 的签名不用变（绳长已经是参数）。
 *
 * 三条坑：
 *   - 坐标可能是 NaN / Infinity（首帧、视口切换）：SVG 的 `d` 里出现 NaN 会让
 *     整条路径静默消失，所以一律收敛到 0；
 *   - 站标尺寸 0 是"配置字段没填"的零值，不是"我要最小"的意图 ——
 *     回落默认值，而不是钳到下限；
 *   - 坐标保留一位小数：长浮点会让 `d` 字符串每帧都不同，白白制造 diff 噪声。
 */

/** 精确指针 + 可悬停设备的媒体查询：触屏与手写笔都匹配不上。 */
export const CHARM_FINE_POINTER_QUERY = "(hover: hover) and (pointer: fine)";

/**
 * 挂绳长度（px）。
 *
 * 本票是模块常量 —— 票 #41 会把它换成运行时配置参数并接上物理，
 * 所以不要在这里提前加参数（那会让"配置段只有两个参数"的验收对不上）。
 */
export const CHARM_TETHER_LENGTH_PX = 120;

/** 站标尺寸下限（px）：再小就看不清是什么东西。 */
export const CHARM_LOGO_SIZE_MIN = 24;

/** 站标尺寸上限（px）：再大就会压住正文。 */
export const CHARM_LOGO_SIZE_MAX = 96;

/** 站标尺寸默认值（px）：与后端 `deco.charm.logo_size` 的默认值一致。 */
export const CHARM_LOGO_SIZE_DEFAULT = 48;

/** 挂件里的平面坐标（视口坐标系，与 `clientX / clientY` 同一套）。 */
export interface CharmPoint {
  x: number;
  y: number;
}

/** 启用判定的三个条件。 */
export interface CharmGateInput {
  /** `(hover: hover) and (pointer: fine)` 是否匹配。 */
  finePointerHover: boolean;
  /** 系统是否要求减少动态。 */
  prefersReducedMotion: boolean;
  /** 后台总开关。 */
  configEnabled: boolean;
}

/**
 * 挂件层是否启用：精确指针 + 未要求减少动态 + 后台总开关。
 *
 * 触屏设备上没有"悬停"这回事，跟着手指的绳子毫无意义；「减少动态」要求的是
 * 整站不出现，而不是放慢。两个都满足时才谈得上渲染。
 */
export function charmLayerEnabled(input: CharmGateInput): boolean {
  return input.finePointerHover && !input.prefersReducedMotion && input.configEnabled;
}

/**
 * 站标尺寸归一化：越界钳到边界、非正值与非法值回落默认值。
 *
 * 后端在写入路径上已经钳过一次，这里是最后一道防线（配置可能来自旧库、
 * 手改的 JSON、或者别人直接调接口）。
 */
export function clampCharmLogoSize(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return CHARM_LOGO_SIZE_DEFAULT;
  return Math.min(CHARM_LOGO_SIZE_MAX, Math.max(CHARM_LOGO_SIZE_MIN, value));
}

/** 把单个坐标收敛到有限值（非有限值一律当 0，避免 NaN 传进 SVG）。 */
function finite(value: number): number {
  return Number.isFinite(value) ? value : 0;
}

/** 保留一位小数，避免长浮点每帧写出不同的 `d` 字符串。 */
function round1(value: number): number {
  return Number(finite(value).toFixed(1));
}

/** 站标中心点：指针正下方一个绳长处（本票不做物理，位置是确定的）。 */
export function charmLogoCenter(
  pointer: CharmPoint,
  tetherLengthPx: number = CHARM_TETHER_LENGTH_PX,
): CharmPoint {
  const length = Math.max(0, finite(tetherLengthPx));
  return { x: finite(pointer.x), y: finite(pointer.y) + length };
}

/** 挂绳路径：从指针到站标中心的一条直线（`d` 属性）。 */
export function charmTetherPath(pointer: CharmPoint, logoCenter: CharmPoint): string {
  return `M${round1(pointer.x)} ${round1(pointer.y)}L${round1(logoCenter.x)} ${round1(logoCenter.y)}`;
}
