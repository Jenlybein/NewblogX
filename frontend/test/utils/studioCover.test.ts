import { describe, expect, it } from "vitest";

import {
  COVER_FORMAT_HINT,
  COVER_PRIMARY_ASPECT,
  describeCoverBadge,
  formatCoverSize,
  resolveCoverUploadRoute,
} from "~/utils/studioCover";

/**
 * 文章封面上传的"走哪条路"判定（纯函数）。
 *
 * 上传体验的核心只有一条分叉：图与 16:9 合规就直接上传；不合规就先进取景窗，
 * 确认后才上传。把这条分叉做成纯函数，页面与弹窗都不再各判一次比例，
 * 于是"要不要弹取景窗"永远只有一个答案。
 */

describe("resolveCoverUploadRoute：16:9 合规就直接上传", () => {
  it("合规尺寸走 direct", () => {
    expect(resolveCoverUploadRoute({ width: 1920, height: 1080 })).toBe("direct");
    expect(resolveCoverUploadRoute({ width: 1280, height: 720 })).toBe("direct");
    // 边界噪声（±1%）不打断上传。
    expect(resolveCoverUploadRoute({ width: 1919, height: 1079 })).toBe("direct");
  });

  it("超出 1% 的尺寸走 frame（先取景再上传）", () => {
    expect(resolveCoverUploadRoute({ width: 1600, height: 1200 })).toBe("frame");
    expect(resolveCoverUploadRoute({ width: 1080, height: 1920 })).toBe("frame");
  });

  it("尺寸非法也走 frame：这种图根本没法直接铺进 16:9 的框", () => {
    expect(resolveCoverUploadRoute(null)).toBe("frame");
    expect(resolveCoverUploadRoute(undefined)).toBe("frame");
    expect(resolveCoverUploadRoute({ width: 0, height: 0 })).toBe("frame");
  });
});

describe("describeCoverBadge：尺寸与合规徽标", () => {
  it("合规图标 success，不合规待取景标 warning", () => {
    expect(describeCoverBadge({ route: "direct", framed: false })).toEqual({
      tone: "success",
      text: `${COVER_PRIMARY_ASPECT} 合规`,
    });
    expect(describeCoverBadge({ route: "frame", framed: false })).toEqual({
      tone: "warning",
      text: "比例不合规，需取景",
    });
  });

  it("取景确认后徽标改成「已取景」，不再催用户去取景", () => {
    expect(describeCoverBadge({ route: "frame", framed: true })).toEqual({
      tone: "warning",
      text: "已取景",
    });
  });
});

describe("提示文案与尺寸读数", () => {
  it("推荐比例与支持的格式写在同一句里", () => {
    expect(COVER_FORMAT_HINT).toContain(COVER_PRIMARY_ASPECT);
    expect(COVER_FORMAT_HINT).toMatch(/JPG/i);
    expect(COVER_FORMAT_HINT).toMatch(/PNG/i);
    expect(COVER_FORMAT_HINT).toMatch(/WebP/i);
  });

  it("尺寸读数形如「1920 × 1080」，没有尺寸时为空串", () => {
    expect(formatCoverSize({ width: 1920, height: 1080 })).toBe("1920 × 1080");
    expect(formatCoverSize(null)).toBe("");
  });
});
