import type { CSSProperties } from "vue";

import type { Schema } from "~/types/contract";

/**
 * 轮播取景（ADR-0014）：只看参数，不碰图片文件。
 *
 * 固定比例是设计常量（所有断点同一个 16:9），不进运行时配置：
 * 一旦可调，历史图的裁切会跟着配置漂移。跨端一致性由静态设计断言锁住。
 *
 * 命名空间里的 `Schema<…BannerCrop>` 保持与契约同源：后端加字段 / 改名都会
 * 让这里编译报错，而不是悄悄多一个读不到的数。
 */
export const BANNER_FRAME_RATIO = "16 / 9";

export type BannerCrop = Schema<"internal_site_adapter_http.BannerCrop">;

const DEFAULT_FOCUS = 0.5;
const DEFAULT_ZOOM = 1;
const MIN_FOCUS = 0;
const MAX_FOCUS = 1;
/** 缩放的下界是 1：铺满基准态。小于 1 会把图缩进面板里，露出容器底色。 */
const MIN_ZOOM = 1;
const MAX_ZOOM = 2;

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/** 与后台 `clampFraming` 同一套精度：焦点 4 位、缩放 2 位，避免两端样式尾数不同。 */
function roundTo(value: number, digits: number): number {
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

/** 缺字段 / 非有限值（NaN、Infinity）= 缺省，而不是把坏值写进样式。 */
function finiteOr(value: number | undefined, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

/** 0–1 的相对坐标 → CSS 百分比；去掉多余的尾零，避免 33.333333% 这类噪声。 */
function percent(value: number): string {
  return `${Number((value * 100).toFixed(2))}%`;
}

function scale(value: number): string {
  return `scale(${value})`;
}

/**
 * 取景参数 → 图片层的样式对象（组件只消费它，不自己算几何）。
 *
 * 四层职责叠起来就是最终效果，后台取景窗的 `framingStyle()` 逐条相同：
 * - `object-fit: cover`：先按容器比例铺满；
 * - `object-position` 取焦点：把图上焦点对到框内同一比例点；
 * - `transform-origin` 取焦点：缩放的锚点落在同一点上；
 * - `transform: scale(zoom)`：在锚点处放大，焦点那一块留在原处。
 *
 * 只输出百分比与 `scale()`、不含任何像素值，所以后台预览框与阅读站任意宽度的
 * 面板得到的是同一块取景（#44 US9）。
 *
 * 钳制是"兜底"而不是"改写数据"：越界值仍由后端拒绝（ADR-0014 决策 4）。
 * 焦点恒在 0–1、缩放恒在 1–2，因此放大后的图始终盖满元素，不会露出面板底色。
 */
export function bannerFramingStyle(crop?: BannerCrop | null): CSSProperties {
  const focusX = roundTo(clamp(finiteOr(crop?.focus_x, DEFAULT_FOCUS), MIN_FOCUS, MAX_FOCUS), 4);
  const focusY = roundTo(clamp(finiteOr(crop?.focus_y, DEFAULT_FOCUS), MIN_FOCUS, MAX_FOCUS), 4);
  const zoom = roundTo(clamp(finiteOr(crop?.zoom, DEFAULT_ZOOM), MIN_ZOOM, MAX_ZOOM), 2);
  const position = `${percent(focusX)} ${percent(focusY)}`;

  return {
    objectFit: "cover",
    objectPosition: position,
    transformOrigin: position,
    transform: scale(zoom),
  };
}
