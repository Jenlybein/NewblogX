import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { parseTokenBlocks, resolveTokenValue } from "~/utils/markdownTheme";

/**
 * 暗色取值本身：铺底是克莱因蓝族、墨色是奶油白、各项对比度达 WCAG AA。
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

describe("暗色铺底是克莱因蓝族，不是深灰/黑", () => {
  it("页面底色 / 纸面 / 抬升纸面都在蓝族里（蓝通道最大、亮度很低）", () => {
    for (const token of ["--color-bg-page", "--color-surface", "--color-surface-raised", "--color-bg-subtle"]) {
      const { r, g, b } = hexToRgb(dark(token));

      expect(b).toBeGreaterThan(g);
      expect(g).toBeGreaterThanOrEqual(r);
      expect(relativeLuminance(dark(token))).toBeLessThan(0.05);
      expect(dark(token)).not.toMatch(/#(?:fff|fdfbf6)/u);
    }
  });

  it("墨色是奶油白，不是纯白/纯黑", () => {
    expect(dark("--color-ink")).toBe(light("--color-surface"));
    expect(dark("--color-ink")).not.toBe("#ffffff");
    expect(dark("--color-ink-muted")).not.toBe("#ffffff");
  });

  it("面/线/墨/高亮/危险五类都有暗色取值，且与浅色不同", () => {
    for (const token of [
      "--color-bg-page",
      "--color-bg-subtle",
      "--color-surface",
      "--color-surface-raised",
      "--color-ink",
      "--color-ink-muted",
      "--color-line",
      "--color-line-strong",
      "--color-line-faint",
      "--color-highlight",
      "--color-danger",
      "--color-danger-soft",
      "--color-danger-line",
      "--color-danger-ink",
      "--color-focus",
      "--color-focus-soft",
      "--color-shadow",
    ]) {
      expect(dark(token)).toBeTruthy();
    }

    // 底色/纸面/墨色必须翻转（否则等于没做暗色）。
    expect(dark("--color-bg-page")).not.toBe(light("--color-bg-page"));
    expect(dark("--color-surface")).not.toBe(light("--color-surface"));
    expect(dark("--color-ink")).not.toBe(light("--color-ink"));
  });
});

describe("暗色对比度达 WCAG AA", () => {
  const surface = dark("--color-surface");
  const bgPage = dark("--color-bg-page");

  it("正文墨色 / 页面底色 ≥ 4.5:1", () => {
    expect(contrastRatio(dark("--color-ink"), bgPage)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-ink"), surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-ink"), dark("--color-bg-subtle"))).toBeGreaterThanOrEqual(4.5);
  });

  it("次级墨色 / 危险文字 / 纸面 ≥ 4.5:1", () => {
    expect(contrastRatio(dark("--color-ink-muted"), surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-danger-ink"), surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(dark("--color-danger-ink"), dbg())).toBeGreaterThanOrEqual(4.5);
  });

  it("焦点环（奶油白）在深蓝底上 ≥ 3:1（UI 组件边界）", () => {
    expect(contrastRatio(dark("--color-focus"), bgPage)).toBeGreaterThanOrEqual(3);
  });
});

function dbg(): string {
  return dark("--color-bg-page");
}
