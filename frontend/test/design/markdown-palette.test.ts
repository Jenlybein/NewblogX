import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  MARKDOWN_THEME_ROLES,
  parseTokenBlocks,
  resolveMarkdownTheme,
  resolveTokenValue,
} from "~/utils/markdownTheme";

/**
 * 正文 markdown 色板（方向 A 暖调编辑）的取值契约（issue #244 / 决策清单 §8）。
 *
 * 为什么单独一份：正文渲染区**不受五族色相锁定**（ADR-0012 后续修订），它的颜色
 * 只由 `--md-*` 这一组 token 治理。这份测试把"色板取值 + 明暗两份 + AA 对比度"
 * 钉在 tokens.css 上，改色时必须同时改这份表 —— 色板漂移会直接红。
 *
 * 取值来源：spec 决策清单 §8 的色板表（独立于实现的第二份事实；不是从 tokens.css
 * 抄回来的，否则断言永远自我实现）。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const adminTokensCss = readFileSync(
  join(projectDir, "../frontend_admin/app/assets/css/tokens.css"),
  "utf8",
);
const blocks = parseTokenBlocks(tokensCss);

const light = resolveMarkdownTheme(tokensCss, "light");
const dark = resolveMarkdownTheme(tokensCss, "dark");

/** 六层标题 + 内联 + 列表标记的规格色值（方向 A 暖调编辑）。 */
const SPEC_LIGHT_PALETTE = {
  heading1: "#1a1815",
  heading2: "#b2442a",
  heading3: "#4e6b3c",
  heading4: "#35597d",
  heading5: "#6b4a78",
  heading6: "#8a6524",
  boldInk: "#0f0d0b",
  italicInk: "#5f5a52",
  strikeLine: "#b2442a",
  strikeInk: "#6e6961",
  underlineLine: "#35597d",
  listOrderedMarker: "#b2442a",
  listUnorderedMarker: "#4e6b3c",
  codeKeyword: "#b2442a",
  codeString: "#4e6b3c",
  codeNumber: "#35597d",
  codeFunction: "#6b4a78",
  codeType: "#8a6524",
  codeVariable: "#2a2724",
} as const;

/** 规格给的是 `#7A756C`；见下方「代码注释」用例说明为什么实际取更深一档。 */
const SPEC_COMMENT_HEX = "#7a756c";

function relativeLuminance(color: string): number {
  const hex = /^#([0-9a-f]{6})$/iu.exec(color.trim())?.[1];
  if (!hex) throw new Error(`对比度断言只接受十六进制色值，收到：${color}`);
  const channels = [0, 2, 4].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255);
  const linear = channels.map((channel) =>
    channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4,
  );
  return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
}

function contrastRatio(foreground: string, background: string): number {
  const [high, low] = [relativeLuminance(foreground), relativeLuminance(background)].sort(
    (left, right) => right - left,
  ) as [number, number];
  return (high + 0.05) / (low + 0.05);
}

/** 通道差：用来判断"同色相、只动明度"，而不是偷偷换了个色相。 */
function channelSpread(left: string, right: string): number {
  const channels = (value: string) =>
    [0, 1, 2].map((index) => Number.parseInt(value.replace("#", "").slice(index * 2, index * 2 + 2), 16));
  const a = channels(left);
  const b = channels(right);
  return Math.max(...a.map((value, index) => Math.abs(value - b[index]!)));
}

const TEXT_ROLES = [
  "heading1",
  "heading2",
  "heading3",
  "heading4",
  "heading5",
  "heading6",
  "boldInk",
  "italicInk",
  "strikeInk",
] as const;

const CODE_ROLES = [
  "codeInk",
  "codeKeyword",
  "codeString",
  "codeNumber",
  "codeFunction",
  "codeType",
  "codeComment",
  "codeVariable",
] as const;

describe("--md-* 语义 token 组：只供正文渲染区，明暗两份同组名字", () => {
  it("亮色取值与 spec 色板逐项一致（注释除外，见下一条）", () => {
    for (const [role, hex] of Object.entries(SPEC_LIGHT_PALETTE)) {
      expect(light[role as keyof typeof light], role).toBe(hex);
    }
  });

  it("代码注释为达 AA 加深一档：仍是同色相，只动明度", () => {
    // spec 写 #7A756C，但它对正文纸面只有 4.43:1、对降一档的代码纸面只有 4.17:1，
    // 达不到 issue 验收里"对比度达 AA"的硬要求。按"色板给形态、AA 给下限"的次序取
    // 同色相加深一档的取值，色相不变（通道差很小），可读性达标。
    expect(contrastRatio(light.codeComment, light.codePaper)).toBeGreaterThanOrEqual(4.5);
    expect(channelSpread(light.codeComment, SPEC_COMMENT_HEX)).toBeLessThanOrEqual(16);
    expect(relativeLuminance(light.codeComment)).toBeLessThan(relativeLuminance(SPEC_COMMENT_HEX));
  });

  it("六个标题色两两不同，七档代码色两两不同", () => {
    const headingColors = [
      light.heading1,
      light.heading2,
      light.heading3,
      light.heading4,
      light.heading5,
      light.heading6,
    ];
    expect(new Set(headingColors).size).toBe(6);

    const codeColors = [
      light.codeKeyword,
      light.codeString,
      light.codeNumber,
      light.codeFunction,
      light.codeType,
      light.codeComment,
      light.codeVariable,
    ];
    expect(new Set(codeColors).size).toBe(7);
  });

  it("暗色有同一组名字的第二份取值，且与亮色不同（不许只做一半）", () => {
    for (const role of Object.keys(MARKDOWN_THEME_ROLES) as Array<keyof typeof light>) {
      const name = MARKDOWN_THEME_ROLES[role];
      if (!name.startsWith("--md-")) continue;
      expect(dark[role], `暗色 ${name}`).toBeTruthy();
      expect(dark[role], `暗色 ${name} 与亮色相同`).not.toBe(light[role]);
    }
  });

  it("两端 tokens.css 都有 --md-* 组，且逐字节同源（由 token-parity 门禁兜底）", () => {
    for (const css of [tokensCss, adminTokensCss]) {
      expect(css).toContain("--md-heading-h1:");
      expect(css).toContain("--md-code-variable:");
    }
    expect(adminTokensCss).toBe(tokensCss);
  });

  it("几何 / 字重档也在 token 层，而不是散在样式字符串里", () => {
    expect(resolveTokenValue(blocks, "light", "--md-bold-weight")).toBe("650");
    expect(resolveTokenValue(blocks, "light", "--md-strike-width")).toBe("1.5px");
    expect(resolveTokenValue(blocks, "light", "--md-underline-offset")).toBe("3px");
  });
});

describe("正文对比度：明暗两态都 ≥ 4.5:1", () => {
  it.each(["light", "dark"] as const)("%s：标题 / 加粗 / 斜体 / 删除线字对纸面达 AA", (mode) => {
    const values = mode === "light" ? light : dark;

    for (const role of TEXT_ROLES) {
      expect(
        contrastRatio(values[role], values.paper),
        `${mode} ${MARKDOWN_THEME_ROLES[role]}`,
      ).toBeGreaterThanOrEqual(4.5);
    }
  });

  it.each(["light", "dark"] as const)("%s：七档代码色对代码纸面达 AA", (mode) => {
    const values = mode === "light" ? light : dark;

    for (const role of CODE_ROLES) {
      expect(
        contrastRatio(values[role], values.codePaper),
        `${mode} ${MARKDOWN_THEME_ROLES[role]}`,
      ).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("代码纸面比正文纸面降一档：亮色更暗、暗色更暗（不是同一张纸）", () => {
    expect(light.codePaper).not.toBe(light.paper);
    expect(dark.codePaper).not.toBe(dark.paper);

    // "降一档"= 亮度更低：亮色纸面与暗色纸面都朝自己的暗方向走一档。
    const lightDelta = relativeLuminance(light.paper) - relativeLuminance(light.codePaper);
    expect(lightDelta).toBeGreaterThan(0);
    expect(lightDelta).toBeLessThan(0.12);

    const darkDelta = relativeLuminance(dark.paper) - relativeLuminance(dark.codePaper);
    expect(darkDelta).toBeGreaterThan(0);
    expect(darkDelta).toBeLessThan(0.05);
  });

  it("标记色（有序赤陶 / 无序苔绿）对纸面达 AA，且两者不同", () => {
    for (const mode of ["light", "dark"] as const) {
      const values = mode === "light" ? light : dark;
      expect(contrastRatio(values.listOrderedMarker, values.paper), mode).toBeGreaterThanOrEqual(4.5);
      expect(contrastRatio(values.listUnorderedMarker, values.paper), mode).toBeGreaterThanOrEqual(4.5);
    }
    expect(light.listOrderedMarker).not.toBe(light.listUnorderedMarker);
  });
});
