import type { CSSProperties } from "vue";

import type { Schema } from "~/types/contract";
import { COVER_FRAME_RATIO, framingStyle } from "./coverFraming";

/**
 * 轮播取景（ADR-0015）：只看参数，不碰图片文件。
 *
 * 固定比例是设计常量（所有断点同一个 16:9），不进运行时配置：
 * 一旦可调，历史图的裁切会跟着配置漂移。跨端一致性由静态设计断言锁住。
 *
 * 取景算术本身在 `~/utils/coverFraming`（文章封面取景窗也用同一份）：
 * 本模块只负责"轮播封面契约字段 → 样式"，避免两套取整/钳制口径。
 *
 * 命名空间里的 `Schema<…BannerCrop>` 保持与契约同源：后端加字段 / 改名都会
 * 让这里编译报错，而不是悄悄多一个读不到的数。
 */
export const BANNER_FRAME_RATIO = COVER_FRAME_RATIO;

export type BannerCrop = Schema<"internal_site_adapter_http.BannerCrop">;

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
 * 面板得到的是同一块取景。
 */
export function bannerFramingStyle(crop?: BannerCrop | null): CSSProperties {
  return framingStyle(crop);
}
