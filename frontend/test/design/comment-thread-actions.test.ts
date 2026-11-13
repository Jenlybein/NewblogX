import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";
import { blockAfter } from "../helpers/designFiles";

/**
 * 评论区图标按钮 / 胶囊按钮的样式门禁（#238 范围 4/5）。
 *
 * 为什么单独一条：这两处是"行为对了但样子没跟上"的高发区 —— 图标按钮很常见地
 * 没有焦点环（键盘用户看不见焦点）、没有最小点击高度（手机上点不中）；胶囊按钮
 * 很常见地写成裸色值或复用卡片的圆角档（圆角阶梯里胶囊只有一档 `pill`）。
 * 颜色仍然只能来自语义 token（ADR-0011）。
 */

const projectDir = process.cwd();
const cssFile = join(projectDir, "app/assets/css/tailwind.css");
const css = readFileSync(cssFile, "utf8");

describe("评论区图标按钮", () => {
  it("图标按钮有最小点击高度与水平内边距（不是无尺寸的裸图标）", () => {
    const block = blockAfter(css, ".comment-action {");

    expect(block).toContain("inline-flex");
    expect(block).toContain("min-h-[32px]");
    expect(block).toContain("items-center");
    expect(block).toContain("gap-1");
  });

  it("键盘可达：焦点环沿用全站 --color-focus", () => {
    const block = blockAfter(css, ".comment-action:focus-visible {");

    expect(block).toContain("outline");
    expect(block).toContain("var(--color-focus)");
  });

  it("已赞态用实心图标 + 最深墨色字（功能蓝在读者站只做文字链接与焦点环）", () => {
    const block = blockAfter(css, ".comment-action--active {");

    expect(block).toContain("text-ink");
    expect(block).not.toContain("text-link");
  });

  it("数字用等宽数字，点赞数变化时不抖动", () => {
    expect(blockAfter(css, ".comment-action__count {")).toContain("tabular-nums");
  });
});

describe("「展开 N 条回复」胶囊按钮", () => {
  it("浅底 + 主题色文字 + 胶囊圆角（圆角只从 pill 档取）", () => {
    const block = blockAfter(css, ".comment-capsule {");

    expect(block).toContain("rounded-pill");
    expect(block).toContain("bg-subtle");
    expect(block).toContain("text-link");
  });

  it("chevron 随展开状态翻转（只动 transform）", () => {
    expect(blockAfter(css, ".comment-capsule--expanded .comment-capsule__chevron {")).toContain("rotate(180deg)");
  });

  it("焦点环同样来自 --color-focus", () => {
    expect(blockAfter(css, ".comment-capsule:focus-visible {")).toContain("var(--color-focus)");
  });
});

describe("颜色仍只有一个来源", () => {
  it("评论区样式里没有裸色值", () => {
    expect(findColorLiterals(css)).toEqual([]);
  });
});
