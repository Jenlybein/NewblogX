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
    candidate.includes(".article-actions-bar--fixed"),
  );
  expect(block, "找不到 <1024px 的互动条断点规则").toBeDefined();
  return block!;
}

describe("互动条：右栏与底部条是同一组件的两种变体", () => {
  it("基础容器是横向排布的按钮条，右栏变体四等分铺满 320px 侧栏", () => {
    const base = blockAfter(css(), ".article-actions-bar {");
    const aside = blockAfter(css(), ".article-actions-bar--aside {");

    expect(base).toContain("display: flex");
    expect(base).toContain("gap");
    expect(aside).toContain("grid-template-columns: repeat(4");
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

describe("断点 1024px：窄屏只有底部固定条一个入口", () => {
  it("底部固定条默认不显示（≥1024 由右栏那份接过）", () => {
    const fixed = blockAfter(css(), ".article-actions-bar--fixed {");

    expect(fixed).toContain("display: none");
  });

  it("<1024px：右栏互动条隐藏、底部条出现（不会两处同时出现，也不会都没有）", () => {
    const narrow = narrowActionsBlock(css());

    expect(blockAfter(narrow, ".article-actions-bar--aside {")).toContain("display: none");
    expect(blockAfter(narrow, ".article-actions-bar--fixed {")).toMatch(/display:\s*flex/u);
  });

  it("底部条固定在下沿，四个按钮等分，并盖住页面底部（正文给它让出高度）", () => {
    const narrow = narrowActionsBlock(css());
    const fixed = blockAfter(narrow, ".article-actions-bar--fixed {");

    expect(fixed).toMatch(/position:\s*fixed/u);
    expect(fixed).toMatch(/bottom:\s*0/u);
    expect(fixed).toMatch(/left:\s*0/u);
    expect(fixed).toMatch(/right:\s*0/u);
    expect(fixed).toContain("z-index");
    // 纸面底色 + 结构线：底部条压在正文上时必须不透出内容。
    expect(fixed).toContain("var(--color-surface)");
    expect(fixed).toContain("var(--color-line");
    // 正文最后一段不被固定条盖住。
    expect(blockAfter(narrow, ".article-page {")).toContain("padding-bottom");
  });

  it("安全区：底部 / 左右都让出 env(safe-area-inset-*)（iPhone 横条不压按钮）", () => {
    const narrow = narrowActionsBlock(css());
    const fixed = blockAfter(narrow, ".article-actions-bar--fixed {");

    expect(fixed).toContain("env(safe-area-inset-bottom)");
    expect(fixed).toContain("env(safe-area-inset-left)");
    expect(fixed).toContain("env(safe-area-inset-right)");
    expect(fixed).toMatch(/padding-bottom/u);
  });
});
