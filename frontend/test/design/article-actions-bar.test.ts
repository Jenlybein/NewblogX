import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 阅读页互动条的版式（票 236 / 决定清单第 2 节）。
 *
 * 页面模板不许有 `<style>`（`pages/articleOutlineColumn.test.ts` 有门禁），所以互动条的
 * 全部样式落在 `tailwind.css`。这里只锁与"版式决策"直接相关的几条：
 *   1. 右栏互动条与底部固定条是**同一个组件**的两种变体，只差容器规则；
 *   2. 断点是 1024px：窄屏右栏那份隐藏、底部固定条出现（不留两个入口，也不一个都没有）；
 *   3. 底部条固定在下沿，给**安全区**留 padding（iPhone 横条 / 手势条不压按钮）；
 *   4. 已赞 / 已藏只用语义 token（`var(--color-*)`）表达，不引入裸色值（另有全站门禁）。
 */

const css = () => readFileSync(join(process.cwd(), "app/assets/css/tailwind.css"), "utf8");

/** 取出从 `openIndex` 起配对的一个 `{...}` 块内容。 */
function braceBlock(source: string, openIndex: number): string {
  let depth = 0;
  for (let index = openIndex; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(openIndex + 1, index);
    }
  }
  throw new Error("样式块没有闭合");
}

/** 取出所有指定 at-rule 的块内容（同一句 media 可能在文件里出现多次）。 */
function atRuleBlocks(source: string, atRule: string): string[] {
  const blocks: string[] = [];
  let from = 0;
  for (;;) {
    const index = source.indexOf(atRule, from);
    if (index < 0) return blocks;
    blocks.push(braceBlock(source, source.indexOf("{", index)));
    from = index + atRule.length;
  }
}

/** 窄屏（<1024px）里接管互动条入口的那一段。 */
function narrowActionsBlock(source: string): string {
  const block = atRuleBlocks(source, "@media (max-width: 1023px)").find((candidate) =>
    candidate.includes(".article-actions-bar--hero"),
  );
  expect(block, "找不到 <1024px 的互动条断点规则").toBeDefined();
  return block!;
}

describe("互动条：右栏与 hero 是同一组件的两种变体（决定清单 §3）", () => {
  it("基础容器是六等分网格，六个动作铺满 320px 侧栏", () => {
    const base = blockAfter(css(), ".article-actions-bar {");
    const button = blockAfter(css(), ".article-actions-bar__button {");

    // 2026-11-13：第六个动作（到顶）落地后，等分格子必须跟着变成六列，
    // 否则第六个按钮会溢出 320px 侧栏、被 `overflow-x: hidden` 裁掉。
    expect(base).toContain("display: grid");
    expect(base).toContain("grid-template-columns: repeat(6");
    expect(base).toContain("gap");
    // 六个动作：点赞 / 收藏 / 评论 / 分享 / 到底 / 到顶。
    expect(button).toContain("min-width: 0");
  });

  it("按钮是紧凑图标档：仍满足 44px 最小点击高度", () => {
    const button = blockAfter(css(), ".article-actions-bar__button {");

    expect(button).toContain("min-height: 44px");
    expect(button).toContain("gap");
  });

  it("已赞 / 已藏用语义 token 抬升（不靠裸色值，也不只靠颜色区分）", () => {
    const active = blockAfter(css(), ".article-actions-bar__button.is-active {");

    expect(active).toContain("var(--color-ink)");
    expect(active).toContain("var(--color-highlight)");
  });
});

describe("断点 1024px：任何宽度只有一个入口，底部没有第二条", () => {
  it("hero 变体默认不显示（≥1024 由右栏那份接过）", () => {
    const hero = blockAfter(css(), ".article-actions-bar--hero,");

    expect(hero).toContain("display: none");
  });

  it("<1024px：右栏互动条隐藏、hero 那份出现（不会两处同时出现，也不会都没有）", () => {
    const narrow = narrowActionsBlock(css());

    expect(blockAfter(narrow, ".article-actions-bar--aside {")).toContain("display: none");
    expect(blockAfter(narrow, ".article-actions-bar--hero {")).toMatch(/display:\s*flex/u);
    expect(blockAfter(narrow, ".article-actions-card--hero {")).toMatch(/display:\s*block/u);
  });

  it("<1024px：右栏那张纸面卡片一起隐藏，不在右栏留一条空纸面带", () => {
    const narrow = narrowActionsBlock(css());

    expect(blockAfter(narrow, ".article-aside .article-actions-card {")).toContain("display: none");
  });

  it("底部固定条彻底退场：没有 --fixed 规则、没有 fixed 定位、页面也不再给它让高度", () => {
    const source = css();

    expect(source).not.toContain(".article-actions-bar--fixed");
    // 旧实现用 `.article-page { padding-bottom: 4.5rem }` 给固定条让位，这条也一并删掉。
    expect(source).not.toMatch(/\.article-page\s*\{[^}]*padding-bottom/u);
    // hero 那份在文档流里（不吸底）：只靠 flex 布局，不出现 position: fixed。
    const narrow = narrowActionsBlock(source);
    expect(narrow).not.toMatch(/\.article-actions-bar--hero\s*\{[^}]*position:\s*fixed/u);
  });
});
