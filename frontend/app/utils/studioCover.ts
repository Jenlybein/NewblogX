import { COVER_ASPECT_TOLERANCE, isAspectCompliant, type Size } from "./coverFraming";

/**
 * 文章封面上传的"走哪条路"判定（纯函数）。
 *
 * 与后台轮播上传同一套口径：图与 16:9 的偏差 ≤1% 就直接上传；
 * 超出 1% 先进取景窗，确认后才上传。判据只在这里定义一次，
 * 页面、面板与弹窗都消费同一份结果，不会出现"面板说不合规、上传却放行"。
 *
 * 本模块不碰存储：取景参数只用于预览，字段落库由后续接入。
 */

/** 面向用户的推荐比例写法（提示文案与徽标共用）。 */
export const COVER_PRIMARY_ASPECT = "16:9";

/** 上传入口的推荐比例与格式提示。 */
export const COVER_FORMAT_HINT = `推荐 ${COVER_PRIMARY_ASPECT} · 支持 JPG / PNG / WebP`;

/** 选图后走的两条路径。 */
export type CoverUploadRoute = "direct" | "frame";

/**
 * 这张图应该走哪条路径：合规直接上传，其余（含读不到尺寸）先取景。
 *
 * 尺寸缺失 / 非法也归到 `frame`：这种图根本没法确定它在 16:9 的框里
 * 会不会露白，直接上传只会产出一张线上必然被裁得奇怪的封面。
 */
export function resolveCoverUploadRoute(size?: Size | null): CoverUploadRoute {
  if (!size) return "frame";
  return isAspectCompliant(size.width, size.height, COVER_ASPECT_TOLERANCE) ? "direct" : "frame";
}

/** 面板上的合规徽标：一个色调 + 一句文案。 */
export interface CoverBadge {
  tone: "success" | "warning";
  text: string;
}

/**
 * 徽标文案随路径与取景状态变化：
 * 合规 → 16:9 合规；不合规未取景 → 催一次取景；不合规已取景 → 已取景。
 */
export function describeCoverBadge(input: { route: CoverUploadRoute; framed: boolean }): CoverBadge {
  if (input.route === "direct") {
    return { tone: "success", text: `${COVER_PRIMARY_ASPECT} 合规` };
  }
  return { tone: "warning", text: input.framed ? "已取景" : "比例不合规，需取景" };
}

/** 尺寸读数（与后台轮播上传面板同一写法）；没有尺寸时为空串。 */
export function formatCoverSize(size?: Size | null): string {
  if (!size) return "";
  return `${size.width} × ${size.height}`;
}
