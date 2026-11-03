import { describe, expect, it } from "vitest";

import { COVER_SHAPE_RATIO_TEXT, COVER_SHAPE_SIZE_TEXT } from "~/utils/imageShaping";
import { COVER_FORMAT_HINT, describeCoverBadge, formatCoverSize } from "~/utils/studioCover";

/**
 * 文章封面的上传前定形（票 200；#204 取景退场）。
 *
 * 没有「合规直传 / 不合规先取景」的分叉：选中的图先在客户端定形成 16:9
 * （1600×900），预览与上传都只认定形产物，也没有取景步骤。
 * 比例口径与尺寸不在这里重写，直接消费 `~/utils/imageShaping` 的同一份取值。
 */

describe("describeCoverBadge：定形徽标", () => {
  it("定形完成后标 success，文案里带产物尺寸（没有「需取景 / 已调取景」）", () => {
    expect(describeCoverBadge()).toEqual({
      tone: "success",
      text: `已定形 ${COVER_SHAPE_SIZE_TEXT}`,
    });
  });
});

describe("提示文案与尺寸读数", () => {
  it("提示写清「上传前自动裁成 16:9（1600 × 900）」，而不是「推荐比例」", () => {
    expect(COVER_FORMAT_HINT).toContain(COVER_SHAPE_RATIO_TEXT);
    expect(COVER_FORMAT_HINT).toContain(COVER_SHAPE_SIZE_TEXT);
    expect(COVER_FORMAT_HINT).toMatch(/JPG/i);
    expect(COVER_FORMAT_HINT).toMatch(/PNG/i);
    expect(COVER_FORMAT_HINT).toMatch(/WebP/i);
  });

  it("尺寸读数形如「1920 × 1080」，没有尺寸时为空串", () => {
    expect(formatCoverSize({ width: 1920, height: 1080 })).toBe("1920 × 1080");
    expect(formatCoverSize(null)).toBe("");
  });
});
