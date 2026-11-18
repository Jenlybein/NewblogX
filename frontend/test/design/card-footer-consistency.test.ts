import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, blockAfterContaining, stripComments } from "../helpers/designFiles";

/**
 * 卡片与页脚一致性 + 列表卡 hover（票 429，spec 423 子票）。
 *
 * 三处细节回到同一套圆角 / 发丝线 / 悬停语言：
 *   1. 首页置顶卡此前是直角，压在 20px 圆角的 blueprint-panel 上，网格里一眼就是
 *      "卡没做完"；这里钉它取卡片圆角 token（--radius-lg / rounded-card）。
 *   2. 页脚版权行上方那条 1px 线此前借了次级字色（72% 反白），比正文还抢眼；
 *      这里钉它降到 12–16% 反白档，且由反白 token 经 color-mix 派生（不写裸 rgba）。
 *   3. 条目卡 hover 只给极轻回应：纸面差 ≤3%、1px 位移、不加重阴影；且只放在
 *      `(hover: hover)` 里 —— 触屏的 sticky hover 会让点过的卡片一直亮着。
 *
 * 与仓库既有口径一致：断言 token 名与关系（谁由谁派生、落在哪个档位），
 * 不断言像素，也不做截图比对。
 *
 * 注意：本文件注释里的"票 429"刻意不写成井号 + 数字 —— 设计 token 门禁把
 * `#429` 这种写法当 3 位十六进制颜色。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const pinnedVue = read("app/components/home/HomePinnedSection.vue");
const footerVue = read("app/components/layout/AppFooter.vue");
const tailwindCss = read("app/assets/css/tailwind.css");
const tokensCss = read("app/assets/css/tokens.css");

/** 取 SFC 里的 `<style scoped>` 块（没有就抛，避免悄悄扫到空字符串）。 */
function scopedStyle(source: string, label: string): string {
  const match = /<style[^>]*scoped[^>]*>([\s\S]*?)<\/style>/u.exec(source);
  if (!match) throw new Error(`${label} 里找不到 <style scoped>`);
  return match[1] ?? "";
}

/** 取 tokens.css 某个映射块里某条自定义属性的取值（去首尾空白）。 */
function declarationIn(block: string, name: string, label: string): string {
  const match = new RegExp(`${name}\\s*:\\s*([^;\\n]+)`, "u").exec(block);
  if (!match) throw new Error(`tokens.css 里找不到 ${name}（${label}）`);
  return match[1]!.trim();
}

function rootBlock(): string {
  const match = /:root\s*\{([\s\S]*?)\n\}/u.exec(tokensCss);
  if (!match) throw new Error("tokens.css 里找不到 :root 块");
  return match[1]!;
}

function darkBlock(): string {
  const match = /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\}/u.exec(tokensCss);
  if (!match) throw new Error("tokens.css 里找不到暗色映射块");
  return match[1]!;
}

/** 从 `rgb(R G B / A)` 里取 alpha；取不到返回 NaN（用于比较档位）。 */
function alphaOf(value: string): number {
  const match = /\/\s*([0-9.]+)\s*\)/u.exec(value);
  return match ? Number(match[1]) : Number.NaN;
}

describe("置顶卡：取卡片圆角阶梯（不再直角咬 20px 圆角面板）", () => {
  it("卡片圆角 token 是 20px，Tailwind 的 rounded-card 指向同一档", () => {
    expect(declarationIn(rootBlock(), "--radius-lg", ":root")).toBe("20px");
    expect(read("tailwind.config.ts")).toContain('card: "var(--radius-lg)"');
  });

  it("HomePinnedSection 的置顶卡用卡片圆角 token，不写像素、不另造一档", () => {
    const block = blockAfter(scopedStyle(pinnedVue, "HomePinnedSection.vue"), ".home-pinned-card {");

    expect(block).toContain("border-radius: var(--radius-lg)");
    // 直角卡是本票要修的形态：正数字面量圆角一律算逃逸。
    expect(block).not.toMatch(/border-radius:\s*[1-9]/u);
  });
});

describe("页脚版权行：1px 线降到 12–16% 反白档", () => {
  function footerMetaBlock(): string {
    return blockAfter(scopedStyle(footerVue, "AppFooter.vue"), ".site-footer__meta {");
  }

  it("线色由反白 token 经 color-mix 派生，落在 12–16% 档（不再是 72% 那档）", () => {
    const block = footerMetaBlock();
    const match =
      /border-top:\s*1px solid color-mix\(in srgb, var\(--color-on-inverse\) ([0-9.]+)%, transparent\)/u.exec(
        block,
      );

    expect(
      match,
      "版权行上方的线必须写成 color-mix(in srgb, var(--color-on-inverse) <12–16>%, transparent)",
    ).not.toBeNull();
    const percent = Number(match![1]);
    expect(percent).toBeGreaterThanOrEqual(12);
    expect(percent).toBeLessThanOrEqual(16);
  });

  it("次级反白字色不再兼任线色（72% 那档只留给文字）", () => {
    const block = footerMetaBlock();

    expect(block).not.toContain("border-top: 1px solid var(--color-on-inverse-muted)");
    // 降档只动线、不动字 —— 版权行的对比度仍是 4.5:1 那一档。
    expect(block).toContain("color: var(--color-on-inverse-muted)");
  });

  it("页脚仍没有裸色值：线色也只能从 token 派生", () => {
    const code = stripComments(footerVue);

    expect(code).not.toMatch(/(?:rgba?|hsla?)\(/u);
    expect(code).not.toMatch(/#[0-9a-fA-F]{3,8}(?![0-9a-zA-Z-])/u);
  });
});

/** 找出所有选择器里含 `.feed-entry:hover` 的声明块（含媒体查询里的那一条）。 */
function feedEntryHoverBlocks(): { selector: string; body: string }[] {
  const code = stripComments(tailwindCss);
  const found: { selector: string; body: string }[] = [];
  for (const match of code.matchAll(/([^{}]+)\{([^{}]*)\}/gu)) {
    const selector = match[1]!.trim();
    if (selector.includes(".feed-entry:hover")) found.push({ selector, body: match[2]! });
  }
  return found;
}

describe("条目卡 hover：极轻纸面回应，只在 hover-capable 设备", () => {
  it("底色与位移写在 @media (hover: hover) 里", () => {
    const block = blockAfterContaining(tailwindCss, "@media (hover: hover)", ".feed-entry:hover");

    expect(block).toContain(".feed-entry:hover");
    expect(block).toContain("background-color");
    expect(block).toMatch(/transform:\s*translateY\(-1px\)/u);
  });

  it("底色差 ≤ 3% 纸面：发丝线 token 与纸面混一档，明暗两态都不过线", () => {
    const block = blockAfterContaining(tailwindCss, "@media (hover: hover)", ".feed-entry:hover");
    const match =
      /background-color:\s*color-mix\(in srgb, var\(--color-line-faint\) ([0-9.]+)%, var\(--color-surface\)\)/u.exec(
        block,
      );

    expect(
      match,
      "hover 底色必须是由 var(--color-line-faint) 与 var(--color-surface) 混出来的极淡一档",
    ).not.toBeNull();
    const mix = Number(match![1]) / 100;
    // 发丝线本身就是墨色 10%（亮）/ 8%（暗）的透明度：再乘混入比例才是叠加到
    // 纸面上的实际墨量，两态都必须 ≤ 3%。
    expect(mix * alphaOf(declarationIn(rootBlock(), "--color-line-faint", "亮色"))).toBeLessThanOrEqual(0.03);
    expect(mix * alphaOf(declarationIn(darkBlock(), "--color-line-faint", "暗色"))).toBeLessThanOrEqual(0.03);
  });

  it("悬停不碰阴影：所有 .feed-entry:hover 规则块里都没有 box-shadow", () => {
    const blocks = feedEntryHoverBlocks();
    expect(blocks.length, "至少要有一条 .feed-entry:hover 规则（含减少动态的位移归零）").toBeGreaterThanOrEqual(2);

    for (const { selector, body } of blocks) {
      expect(body, `${selector} 不该靠阴影抬升（层级是票 428 定的）`).not.toContain("box-shadow");
    }
    // 基态条目卡同样不带影：material-tiers.test.ts 的同一口径，这里补在 hover 侧。
    expect(blockAfter(tailwindCss, ".feed-entry {")).not.toContain("box-shadow");
  });

  it("hover 不是无条件覆盖：基态 hover 只改描边，底色/位移只在 hover-capable 媒体里", () => {
    const base = blockAfter(tailwindCss, ".feed-entry:hover,\n  .feed-entry:focus-within {");

    expect(base).toContain("border-color: var(--color-line-strong)");
    expect(base).not.toContain("background");
    expect(base).not.toContain("transform");
  });

  it("减少动态：1px 位移归零，但纸面回应保留", () => {
    const reduced = blockAfterContaining(tailwindCss, "@media (prefers-reduced-motion: reduce)", ".feed-entry:hover");

    expect(reduced).toContain("transform: none");
    expect(reduced).not.toContain("background-color");
  });
});
