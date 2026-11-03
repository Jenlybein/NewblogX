import type { Size } from "./coverFraming";
import { COVER_SHAPE_RATIO_TEXT, COVER_SHAPE_SIZE_TEXT } from "./imageShaping";

/**
 * 文章封面上传的文案与徽标（纯函数）。
 *
 * 票 200 起上传前就在客户端定形（居中裁成 16:9、1600×900），所以这里不再有
 * 「这张图该直传还是先取景」的判定：比例口径与尺寸都来自 `~/utils/imageShaping`
 * 的同一份取值，页面、面板与弹窗只消费结果，不再各写一遍 16:9 / 1600 / 900。
 *
 * 本模块不碰存储：取景参数（旧数据）仍可被读取与调整，但它不再是上传的必经步骤。
 */

/** 上传入口的提示：说明会先定形，以及支持的输入格式。 */
export const COVER_FORMAT_HINT = `上传前自动裁成 ${COVER_SHAPE_RATIO_TEXT}（${COVER_SHAPE_SIZE_TEXT}）· 支持 JPG / PNG / WebP`;

/** 面板上的徽标：一个色调 + 一句文案。 */
export interface CoverBadge {
  tone: "success" | "warning";
  text: string;
}

/**
 * 徽标文案：定形完成后永远是 success —— 产物已经是 16:9，不该再催用户去取景。
 * 手动调过取景时补一句，说明这层是**可选**动作（双轨期取景入口仍可用）。
 */
export function describeCoverBadge(input: { framed: boolean }): CoverBadge {
  return {
    tone: "success",
    text: input.framed ? `已定形 ${COVER_SHAPE_SIZE_TEXT} · 已调取景` : `已定形 ${COVER_SHAPE_SIZE_TEXT}`,
  };
}

/** 尺寸读数（与后台轮播上传面板同一写法）；没有尺寸时为空串。 */
export function formatCoverSize(size?: Size | null): string {
  if (!size) return "";
  return `${size.width} × ${size.height}`;
}
