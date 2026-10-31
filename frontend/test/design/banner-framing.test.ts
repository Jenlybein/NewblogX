import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 轮播取景的固定比例（ADR-0014 决策 3）：16:9 只在两端各定义一处，跨端锁定一致。
 *
 * 为什么是静态断言而不是运行时配置：比例一旦可调，历史图的裁切就会随配置漂移。
 * 这里沿用 tokens.css 同源门禁与 foundation-tokens.test.ts 的形式 —— 读两份源码、
 * 取同一段声明、逐字比较（不做语义/空白宽松，免得"看起来一样但实现已经漂了"）。
 *
 * 阅读站：首屏轮播面板（所有断点同一比例）。
 * 后台：上传面板里那块 16:9 的预览 / 取景框（#45 的取景窗长在同一个框里）。
 */
const projectDir = process.cwd();
const readerIndex = readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");
const adminCss = readFileSync(join(projectDir, "../frontend_admin/app/assets/css/admin.css"), "utf8");

/** 取块里的比例声明，并把空白压成单空格（只消除排版差异，不放过取值差异）。 */
function aspectRatioOf(block: string): string {
  return (/aspect-ratio:\s*([^;]+);/u.exec(block)?.[1] ?? "").replace(/\s+/gu, " ").trim();
}

const SPEC_RATIO = "16 / 9";

describe("轮播面板固定 16:9：跨端只有一份定义、两端取值一致", () => {
  it("阅读站面板用 aspect-ratio: 16 / 9 + width: 100%，不再有高度档", () => {
    const panel = blockAfter(readerIndex, ".home-cover__panel {");

    expect(panel).toContain("width: 100%");
    expect(aspectRatioOf(panel)).toBe(SPEC_RATIO);
    expect(panel).not.toContain("height:");
  });

  it("窄屏断点不另换比例（面板比例任何断点都相同）", () => {
    const narrow = blockAfter(readerIndex, "@media (max-width: 767px)");
    const narrowPanel = blockAfter(narrow, ".home-cover__panel {");

    expect(narrowPanel).not.toContain("aspect-ratio");
    expect(narrowPanel).not.toContain("height:");
    // 阅读站首屏里只允许这一处比例定义 —— 加一条覆盖规则就等于留了两套。
    expect(readerIndex.match(/aspect-ratio\s*:/gu) ?? []).toHaveLength(1);
  });

  it("后台取景 / 预览框与阅读站面板是同一个比例（跨端常量一致性）", () => {
    const adminFrame = blockAfter(adminCss, ".admin-upload-panel__picker {");

    expect(aspectRatioOf(adminFrame)).toBe(SPEC_RATIO);
    expect(aspectRatioOf(adminFrame)).toBe(aspectRatioOf(blockAfter(readerIndex, ".home-cover__panel {")));
  });
});
