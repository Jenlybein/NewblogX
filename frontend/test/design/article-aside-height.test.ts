import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

import { blockAfter } from "../helpers/designFiles";

/**
 * 右栏按视口高度收敛（票 237 / 决定清单第 2 节决策 4）。
 *
 * 事实基线：`.article-aside` 在 ≥1024px 是 `sticky; top: 88px`，作者卡 ≈340px、
 * 目录卡 ≈490px（交互条卡夹在中间），合计 ≈850px+ —— 视口高度低于约 938px 时，
 * 吸顶右栏的下半截会被**直接裁掉**（不是滚动，是看不见）。
 *
 * 这一票不改模板（页面不许有 `<style>`，另有门禁），全部落在 `tailwind.css`，
 * 所以这里沿用 #236 / `scroll-chrome.test.ts` 的先例锁"源码里必须成立的关系"：
 *   1. 限高与整列滚动只出现在 ≥1024 的吸顶区间，窄屏照旧整页滚动；
 *   2. 目录体高度换成 `dvh` 档，目录仍然自己滚（不把滚动推给整页）；
 *   3. 作者签名两行截断；矮视口（高 <820px）作者卡瘦身（压 padding、折叠 stats）；
 *   4. 矮视口下互动条卡与目录卡的间距一起收紧，避免"整列滚动 + 目录滚动"两层滚动条叠加；
 *   5. 新写的每一段只用语义 token，不出现裸色值（全站门禁另有断言，这里提前锁住本票的改动）。
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

/** 取出所有指定 at-rule 的块内容（同一条媒体查询可能在文件里出现多次）。 */
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

/** ≥1024px 的吸顶右栏那一段（`position: sticky` 与限高必须待在同一块里）。 */
function stickyAsideBlock(source: string): string {
  const block = atRuleBlocks(source, "@media (min-width: 1024px)").find(
    (candidate) => candidate.includes(".article-aside") && candidate.includes("position: sticky"),
  );
  expect(block, "找不到 ≥1024px 的吸顶右栏规则").toBeDefined();
  return block!;
}

/** <1024px 里把右栏退回普通流的那一段。
 *  注意用带花括号的 `.article-aside {` 作锚点：<1024px 里还有一段
 *  `.article-aside .article-actions-card { display: none }`（互动条让位），
 *  只按类名前缀找会先命中它。 */
function narrowAsideBlock(source: string): string {
  const block = atRuleBlocks(source, "@media (max-width: 1023px)").find((candidate) =>
    candidate.includes(".article-aside {"),
  );
  expect(block, "找不到 <1024px 的右栏规则").toBeDefined();
  return block!;
}

/** 矮视口（高 <820px）里给作者卡瘦身的那一段（嵌在吸顶右栏里，别溢到窄屏去）。 */
function compactAsideBlock(source: string): string {
  const sticky = stickyAsideBlock(source);
  expect(sticky, "矮视口的瘦身规则必须嵌在 ≥1024px 的吸顶区间里").toContain("@media (max-height: 820px)");
  return blockAfter(sticky, "@media (max-height: 820px)");
}

describe("吸顶右栏：按视口高度收敛，不再把底部裁掉", () => {
  it("限高到视口高度减去页头偏移，并让整列可滚（裁掉 = 看不见，可滚 = 能看完）", () => {
    const aside = blockAfter(stickyAsideBlock(css()), ".article-aside {");

    // 原先的两条（吸顶位置）保持不动，本票只是加高度约束。
    expect(aside).toContain("position: sticky");
    expect(aside).toContain("top: 88px");
    expect(aside).toMatch(/max-height:\s*calc\(100dvh\s*-\s*88px\)/u);
    expect(aside).toMatch(/overflow-y:\s*auto/u);
    // 只给 overflow-y 会让 overflow-x 自动变 auto：焦点环 / hover 面底会在
    // 右栏边缘多出一条横向滚动条。显式关掉横向。
    expect(aside).toMatch(/overflow-x:\s*hidden/u);
  });

  it("限高不写进裸选择器：≥1024 才生效，窄屏右栏仍是 static（入口交给底部固定条）", () => {
    expect(blockAfter(css(), ".article-aside {")).not.toContain("max-height");

    const narrow = blockAfter(narrowAsideBlock(css()), ".article-aside {");
    expect(narrow).toContain("position: static");
    expect(narrow).not.toContain("max-height");
    expect(narrow).not.toContain("overflow");
  });

  it("整列滚动是兜底：滚到底不把页面一起带走（不滚链到整页）", () => {
    const aside = blockAfter(stickyAsideBlock(css()), ".article-aside {");

    expect(aside).toMatch(/overscroll-behavior:\s*contain/u);
  });

  it("卡片不被 flex 压扁：限高后只许整列滚，不出现卡与卡重叠（hover 遮挡）", () => {
    const sticky = stickyAsideBlock(css());
    const children = blockAfter(sticky, ".article-aside > * {");

    expect(children).toMatch(/flex-shrink:\s*0/u);
  });
});

describe("目录体：dvh 档高度，内部滚动保留", () => {
  it("max-height 由固定 380px 换成 clamp(160px, 32dvh, 380px)", () => {
    const body = blockAfter(css(), ".article-toc__body {");

    expect(body).toMatch(/max-height:\s*clamp\(160px,\s*32dvh,\s*380px\)/u);
    // 旧的固定档退役：矮视口下 380px 会把整列顶出可视区。
    expect(body).not.toContain("380px]");
  });

  it("目录还是目录自己滚，不改成整页 / 整列滚动", () => {
    const body = blockAfter(css(), ".article-toc__body {");

    // 以 `@apply overflow-y-auto` 或字面 `overflow-y: auto` 任一种形式成立都算（契约是"能滚"）。
    expect(body).toMatch(/overflow-y(?::\s*auto|-auto)/u);
  });

  it("clamp 的上下界守住可读区间：下限不低于 140px，上限不超过 380px", () => {
    // 320（右栏宽）与 68ch 正文行宽之外的第三个数字约束：目录至少要能看见 3-4 条，
    // 又不能超过原来撑爆右栏的 380px。数值来自票面验收（1280x720 / 1024x768）。
    const body = blockAfter(css(), ".article-toc__body {");
    const clamp = body.match(/clamp\(([0-9]+)px,\s*([0-9]+)dvh,\s*([0-9]+)px\)/u);

    expect(clamp, "目录体的 max-height 必须是像素/dvh 三元 clamp").toBeDefined();
    expect(Number(clamp![1])).toBeGreaterThanOrEqual(140);
    expect(Number(clamp![3])).toBeLessThanOrEqual(380);
    expect(Number(clamp![3])).toBeGreaterThanOrEqual(Number(clamp![1]));
  });
});

describe("作者签名：两行截断", () => {
  it("签名限两行，超出省略（作者卡高度不再随签名长度漂移）", () => {
    const abstract = blockAfter(css(), ".article-author__abstract {");

    expect(abstract).toMatch(/-webkit-line-clamp:\s*2/u);
    expect(abstract).toMatch(/-webkit-box-orient:\s*vertical/u);
    expect(abstract).toMatch(/overflow:\s*hidden/u);
  });
});

describe("矮视口（高 <820px）：作者卡自动瘦身", () => {
  it("瘦身规则嵌在 ≥1024 的吸顶区间：窄屏整页滚动时作者信息照旧（stats 不折叠）", () => {
    const source = css();
    const sticky = stickyAsideBlock(source);

    // 把吸顶那段从源码里摘掉后，文件里不该再有裸的 max-height 断点。
    expect(source.replace(sticky, "")).not.toContain("@media (max-height: 820px)");

    // 窄屏区间不许动作者卡的可见性。
    const narrow = narrowAsideBlock(source);
    expect(narrow).not.toContain(".article-author__stats");
  });

  it("压 padding 并折叠 stats 行", () => {
    const compact = compactAsideBlock(css());

    expect(blockAfter(compact, ".article-aside .article-author {")).toMatch(/padding:/u);
    expect(blockAfter(compact, ".article-author__stats {")).toContain("display: none");
  });

  it("互动条卡与目录卡的间距一并收紧（差的那点高度省在这，不靠叠两条滚动条）", () => {
    const compact = compactAsideBlock(css());

    // 整列间距：基础是 gap-5（1.25rem），矮视口收到 0.75rem。
    expect(blockAfter(compact, ".article-aside {")).toMatch(/gap:\s*0\.75rem/u);
    // 两张纸面卡自己的 padding 也收一档。
    expect(blockAfter(compact, ".article-aside .article-actions-card {")).toMatch(/padding:/u);
    expect(blockAfter(compact, ".article-aside .article-toc {")).toMatch(/padding:/u);
  });

  it("瘦身只收版面，不动交互条的分栏与断点互斥（<1024 隐藏/显示仍由 #236 那段管）", () => {
    const compact = compactAsideBlock(css());

    // 矮视口里不许再声明 grid-template-columns / display，免得把 4 等分或底部条的显隐改坏。
    expect(compact).not.toContain("grid-template-columns");
    expect(compact).not.toContain(".article-actions-bar--aside");
    expect(compact).not.toContain(".article-actions-bar--fixed");
  });
});

describe("本票改动只用语义 token", () => {
  it("新增 / 改写的四段里没有裸色值", () => {
    const source = css();
    const blocks = [
      stickyAsideBlock(source),
      blockAfter(source, ".article-toc__body {"),
      blockAfter(source, ".article-author__abstract {"),
    ];

    for (const block of blocks) {
      expect(findColorLiterals(block)).toEqual([]);
    }
  });
});
