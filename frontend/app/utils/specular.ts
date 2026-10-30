/**
 * 指针镜面高光的坐标换算（纯函数）。
 *
 * 玻璃的 `::before` 用 `--mx/--my` 作为径向渐变的圆心，这两个变量是**百分比**：
 * 需要把 pointermove 的视口坐标换算成元素内的相对比例。注意事项：
 *   - 元素尺寸为 0（未挂载 / display:none）时退回中心，避免除零得到 NaN；
 *   - 指针跑出元素（拖拽 / 惯性）时钳到 0–100，CSS 变量里不能出现负数；
 *   - 保留两位小数，避免把一长串浮点写进 CSS 变量（每帧都写，能省则省）。
 */

export interface SpecularRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface SpecularPoint {
  /** 横向百分比（0–100）。 */
  mx: number;
  /** 纵向百分比（0–100）。 */
  my: number;
}

const CENTER_PERCENT = 50;

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return CENTER_PERCENT;
  const clamped = Math.min(100, Math.max(0, value));
  return Number(clamped.toFixed(2));
}

/**
 * 指针坐标 → 元素内百分比。
 *
 * @param clientX 视口横坐标（PointerEvent.clientX）
 * @param clientY 视口纵坐标（PointerEvent.clientY）
 * @param rect 元素相对视口的矩形（getBoundingClientRect 的结果）
 */
export function specularPercentage(
  clientX: number,
  clientY: number,
  rect: SpecularRect,
): SpecularPoint {
  const width = Number.isFinite(rect.width) ? rect.width : 0;
  const height = Number.isFinite(rect.height) ? rect.height : 0;

  if (width <= 0 || height <= 0 || !Number.isFinite(clientX) || !Number.isFinite(clientY)) {
    return { mx: CENTER_PERCENT, my: CENTER_PERCENT };
  }

  return {
    mx: clampPercent(((clientX - rect.left) / width) * 100),
    my: clampPercent(((clientY - rect.top) / height) * 100),
  };
}
