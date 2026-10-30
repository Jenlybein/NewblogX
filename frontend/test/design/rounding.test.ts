import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");

const STRUCTURAL_RADIUS_SELECTORS = [".surface-card", ".surface-section", ".article-author-row"];

function ruleBody(selector: string): string {
  const start = tailwindCss.indexOf(`${selector} {`);
  if (start < 0) throw new Error(`tailwind.css 里找不到 ${selector}`);
  return tailwindCss.slice(start, tailwindCss.indexOf("}", start));
}

describe("圆角归零（spec：只允许头像 / 胶囊 / 圆形进度 / 捏捏）", () => {
  it("结构卡片 / 分节 / 作者行都声明 0 圆角", () => {
    for (const selector of STRUCTURAL_RADIUS_SELECTORS) {
      expect(ruleBody(selector)).toContain("rounded-none");
    }
  });

  it("tailwind.css 里不再有 >0 的半径工具类（28px / 24px / 2xl 等）", () => {
    // 允许 rounded-none 与 rounded-full（pill/tag/进度/头像/捏捏例外）。
    const positiveRadius = /rounded-(?!none\b|full\b)(?:\[[1-9][0-9]*px\]|sm\b|md\b|lg\b|xl\b|2xl\b|3xl\b)/gu;
    const offenders = [...tailwindCss.matchAll(positiveRadius)].map((match) => match[0]);

    expect(offenders).toEqual([]);
  });

  it("文章页/首页共用的结构件（feed-entry / profile-hero / surface-*）不是圆润卡片", () => {
    for (const selector of [".feed-entry", ".profile-hero", ".surface-card", ".surface-section"]) {
      // 要么显式 rounded-none，要么根本没有正半径（Tailwind 默认 0）。
      const body = ruleBody(selector);

      expect(body).not.toMatch(/rounded-(?!none\b|full\b)(?:\[[1-9][0-9]*px\]|sm\b|md\b|lg\b|xl\b|2xl\b|3xl\b)/u);
    }
  });
});
