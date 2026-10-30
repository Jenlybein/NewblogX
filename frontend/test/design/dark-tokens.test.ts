import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { parseTokenBlocks, resolveTokenValue } from "~/utils/markdownTheme";

/**
 * 暗色取值本身：铺底是**中性石墨阶**（不是克莱因蓝、也不是暖黑），墨色是浅石墨，
 * 各项对比度达 WCAG AA。
 *
 * 取值全部从 tokens.css **真实解析**（不手抄 hex）：改 token 时这里会一起动，
 * 抄写漂移不会被放过。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const blocks = parseTokenBlocks(tokensCss);

function dark(name: string): string {
  return resolveTokenValue(blocks, "dark", name);
}

function light(name: string): string {
  return resolveTokenValue(blocks, "light", name);
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const value = hex.replace("#", "");
  return {
    r: Number.parseInt(value.slice(0, 2), 16),
    g: Number.parseInt(value.slice(2, 4), 16),
    b: Number.parseInt(value.slice(4, 6), 16),
  };
}

function channelLuminance(channel: number): number {
  const value = channel / 255;
  return value <= 0.03928 ? value / 12.92 : ((value + 0.05) / 1.05) ** 2.4;
}

function relativeLuminance(hex: string): number {
  const { r, g, b } = hexToRgb(hex);
  return 0.2126 * channelLuminance(r) + 0.7152 * channelLuminance(g) + 0.0722 * channelLuminance(b);
}

/** `rgba(r, g, b, α)` 叠在给定底色上的合成色，用于"半透明线/底"上的文字对比度。 */
function composite(foreground: string, background: string): string {
  const rgba = /rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)/u.exec(foreground);
  if (!rgba) {
    return foreground;
  }
  const alpha = Number.parseFloat(rgba[4]!);
  const base = hexToRgb(background);
  const blend = [Number(rgba[1]), Number(rgba[2]), Number(rgba[3])].map(
    (value, index) => Math.round(value * alpha + [base.r, base.g, base.b][index]! * (1 - alpha)),
  );
  return `#${blend.map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

function contrastRatio(foreground: string, background: string): number {
  const fg = foreground.startsWith("rgba") ? composite(foreground, background) : foreground;
  const lighter = Math.max(relativeLuminance(fg), relativeLuminance(background));
  const darker = Math.min(relativeLuminance(fg), relativeLuminance(background));
  return (lighter + 0.05) / (darker + 0.05);
}

describe("暗色铺底：中性石墨阶（不是深蓝、不是暖黑）", () => {
  it("四档底色取到 §3.3 规定的中性暗阶", () => {
    expect(dark("--color-bg-page")).toBe("#161617");
    expect(dark("--color-bg-subtle")).toBe("#101011");
    expect(dark("--color-surface")).toBe("#1d1d1f");
    expect(dark("--color-surface-raised")).toBe("#2c2c2e");
  });

  it("四档暗面确实是中性灰：三通道差 ≤3、亮度很低、没有蓝色偏置", () => {
    for (const token of ["--color-bg-page", "--color-surface", "--color-surface-raised", "--color-bg-subtle"]) {
      const { r, g, b } = hexToRgb(dark(token));

      expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThanOrEqual(3);
      expect(b - r).toBeLessThanOrEqual(2);
      expect(relativeLuminance(dark(token))).toBeLessThan(0.06);
    }
  });

  it("墨色是浅石墨 #F5F5F7，次级 #A1A1A6，极淡 #86868B（不是奶油白）", () => {
    expect(dark("--color-ink")).toBe("#f5f5f7");
    expect(dark("--color-ink-muted")).toBe("#a1a1a6");
    expect(dark("--color-ink-faint")).toBe("#86868b");
  });

  it("面/线/墨/高亮/危险/链接/反白层都有暗色取值，且与浅色不同", () => {
    for (const token of [
      "--color-bg-page",
      "--color-bg-subtle",
      "--color-surface",
      "--color-surface-raised",
      "--color-ink",
      "--color-ink-muted",
      "--color-ink-faint",
      "--color-line",
      "--color-line-strong",
      "--color-line-faint",
      "--color-highlight",
      "--color-danger",
      "--color-danger-soft",
      "--color-danger-line",
      "--color-danger-ink",
      "--color-link",
      "--color-focus",
      "--color-focus-soft",
      "--color-on-inverse",
      "--color-on-inverse-muted",
      "--color-surface-inverse",
      "--color-shadow",
    ]) {
      expect(dark(token)).toBeTruthy();
    }

    // 底色/纸面/墨色必须翻转（否则等于没做暗色）。
    expect(dark("--color-bg-page")).not.toBe(light("--color-bg-page"));
    expect(dark("--color-surface")).not.toBe(light("--color-surface"));
    expect(dark("--color-ink")).not.toBe(light("--color-ink"));
  });

  it("功能蓝在暗色换成苹果暗色蓝 #2997FF，且只出现在链接与焦点环", () => {
    expect(dark("--color-link")).toBe("#2997ff");
    expect(dark("--color-focus")).toBe("#2997ff");
    // 焦点环的浅色底（交互蓝 16%）在暗色块里是第二份取值。
    expect(dark("--color-focus-soft")).toBe("rgba(41, 151, 255, 0.16)");
  });

  it("暗色不出现克莱因蓝的任何残留", () => {
    expect(tokensCss).not.toMatch(/#002fa7/iu);
    expect(dark("--color-line")).toBe("rgba(245, 245, 247, 0.14)");
    expect(dark("--color-line-strong")).toBe("rgba(245, 245, 247, 0.28)");
  });
});

describe("暗色对比度达 WCAG AA", () => {
  const surface = dark("--color-surface");
  const bgPage = dark("--color-bg-page");

  it("正文墨色 / 页面底色 / 纸面 / 次底 ≥ 4.5:1", () => {
    expect(contrastRatio(dark("--color-ink"), bgPage)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-ink"), surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-ink"), dark("--color-bg-subtle"))).toBeGreaterThanOrEqual(4.5);
  });

  it("次级墨色 / 危险文字 / 纸面 ≥ 4.5:1", () => {
    expect(contrastRatio(dark("--color-ink-muted"), surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-ink-muted"), bgPage)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-danger-ink"), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("文字链接与焦点环（功能蓝 #2997FF）在页底 ≥ 4.5:1 / 3:1", () => {
    expect(contrastRatio(dark("--color-link"), bgPage)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-focus"), bgPage)).toBeGreaterThanOrEqual(3);
  });

  it("反白层（页脚 / 封面）在深墨面上 ≥ 4.5:1（含次级字）", () => {
    const inverseSurface = dark("--color-surface-inverse");

    expect(contrastRatio(dark("--color-on-inverse"), inverseSurface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-on-inverse-muted"), inverseSurface)).toBeGreaterThanOrEqual(4.5);
  });
});
