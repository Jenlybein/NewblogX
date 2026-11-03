import type { ShapeImageTarget } from "./imageShaping";
import { COVER_SHAPE_RATIO_TEXT, COVER_SHAPE_SIZE_TEXT } from "./imageShaping";

/**
 * 文章封面上传的文案与徽标（纯函数）。
 *
 * 票 200 起上传前就在客户端定形（居中裁成 16:9、1600×900），#204 起取景窗全量退场：
 * 后台/创作页不再有取景步骤，读者侧也固定居中渲染。比例口径与尺寸都来自
 * `~/utils/imageShaping` 的同一份取值，页面、面板与弹窗只消费结果。
 */

/** 上传入口的提示：说明会先定形，以及支持的输入格式。 */
export const COVER_FORMAT_HINT = `上传前自动裁成 ${COVER_SHAPE_RATIO_TEXT}（${COVER_SHAPE_SIZE_TEXT}）· 支持 JPG / PNG / WebP`;

/** 面板上的徽标：一个色调 + 一句文案。 */
export interface CoverBadge {
  tone: "success" | "warning";
  text: string;
}

/**
 * 徽标文案：定形完成后永远是 success —— 产物已经是 16:9，没有"还需取景"这回事。
 */
export function describeCoverBadge(): CoverBadge {
  return {
    tone: "success",
    text: `已定形 ${COVER_SHAPE_SIZE_TEXT}`,
  };
}

/** 尺寸读数（与后台轮播上传面板同一写法）；没有尺寸时为空串。 */
export function formatCoverSize(size?: ShapeImageTarget | null): string {
  if (!size) return "";
  return `${size.width} × ${size.height}`;
}
