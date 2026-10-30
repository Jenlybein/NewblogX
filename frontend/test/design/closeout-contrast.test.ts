import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 收口票（#25）：明暗两套主题的对比度断言。
 *
 * 为什么值得测：前面几票把墨色、纸面、链接、危险文字、反白面全部换成中性石墨阶，
 * 换完是否仍满足「正文 4.5:1 / 非文字与大字 3:1」只能靠眼睛，眼睛会漏。这里直接
 * 从 tokens.css 里读出**真实取值**（含 var() 引用也要解一次），按 WCAG 的相对
 * 亮度公式算出比值再断言；任何一票把某个色改暗/改亮，这里立刻红。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

/** 取出某个选择器块的整体文本。 */
function block(selectorPattern: RegExp): string {
  const found = selectorPattern.exec(tokensCss);
  if (!found) throw new Error(`tokens.css 里找不到 ${selectorPattern}`);
  return found[1] ?? "";
}

const lightBlock = block(/:root\s*\{([\s\S]*?)\n\}/u);
const darkBlock = block(/(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u);

/**
 * 原始层的十六进制取值（解 var() 引用时要用到）。
 *
 * 关键：**按主题各建一份**。`--jelly-interactive` 在亮色是 #0066cc、在暗色是 #2997ff，
 * 若两张表合并成一张，暗色会覆盖亮色，亮色链接就会拿暗色值去算对比度 —— 那正是
 * Q19 里特意避开的那条坑（苹果的 #2997ff 对白底只有 2.9:1，不能作亮色链接）。
 */
function hexMap(css: string): Map<string, string> {
  const map = new Map<string, string>();
  for (const match of css.matchAll(/--([a-z0-9-]+):\s*#([0-9a-f]{3,8})/giu)) {
    map.set(`--${match[1]!.toLowerCase()}`, `#${match[2]!.toLowerCase()}`);
  }
  return map;
}

const lightHex = hexMap(lightBlock);
const darkHex = hexMap(darkBlock);

/** 读某个语义 token 的取值；`var(--x)` 解一层引用，必须解到十六进制。 */
function valueOf(css: string, name: string): string {
  const match = new RegExp(`${name}:\\s*([^;]+);`, "u").exec(css);
  if (!match) throw new Error(`${name} 未在该块里声明`);
  const raw = match[1]!.trim();

  if (raw.startsWith("#")) return raw.toLowerCase();
  if (raw.startsWith("var(")) {
    const ref = raw.slice(4, raw.indexOf(")")).trim().toLowerCase();
    // 先在本主题的取值里找，找不到才回落到另一主题（例如暗色块没重声明的 token）。
    const resolved =
      (css === darkBlock ? darkHex.get(ref) ?? lightHex.get(ref) : lightHex.get(ref) ?? darkHex.get(ref));
    if (!resolved) throw new Error(`${name} 引用的 ${ref} 不是可解析的十六进制求值`);
    return resolved;
  }

  throw new Error(`${name} 的取值既不是十六进制也不是可解析的 var()：${raw}`);
}

/** 十六进制（支持 3/6 位）→ 线性化的相对亮度。 */
function luminance(hex: string): number {
  const value = hex.replace("#", "");
  const full =
    value.length === 3
      ? [...value].map((char) => char + char).join("")
      : value.slice(0, 6);
  const channels = [0, 2, 4].map((index) => parseInt(full.slice(index, index + 2), 16) / 255);
  const [r, g, b] = channels as [number, number, number];
  const linear = (channel: number) =>
    channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;

  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b);
}

/** WCAG 对比度。 */
function contrast(foreground: string, background: string): number {
  const [light, dark] = [luminance(foreground), luminance(background)].sort((a, b) => b - a) as [
    number,
    number,
  ];
  return (light + 0.05) / (dark + 0.05);
}

describe.each([
  ["亮色", lightBlock],
  ["暗色", darkBlock],
])("收口：%s主题下的关键配对对比度", (_scheme, css) => {
  const surface = valueOf(css, "--color-surface");

  it("正文墨色 ≥4.5:1", () => {
    expect(contrast(valueOf(css, "--color-ink"), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("次级墨色 ≥4.5:1", () => {
    expect(contrast(valueOf(css, "--color-ink-muted"), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("极淡墨色 ≥3:1（只允许用于大字与非文字）", () => {
    expect(contrast(valueOf(css, "--color-ink-faint"), surface)).toBeGreaterThanOrEqual(3);
  });

  it("文字链接（功能蓝）≥4.5:1", () => {
    expect(contrast(valueOf(css, "--color-link"), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("焦点环（功能蓝）对相邻纸面 ≥3:1（非文字）", () => {
    expect(contrast(valueOf(css, "--color-focus"), surface)).toBeGreaterThanOrEqual(3);
  });

  it("危险文字（派生暗橙）≥4.5:1", () => {
    expect(contrast(valueOf(css, "--color-danger-ink"), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("深墨面上的反白字 ≥4.5:1", () => {
    expect(
      contrast(valueOf(css, "--color-on-inverse"), valueOf(css, "--color-surface-inverse")),
    ).toBeGreaterThanOrEqual(4.5);
  });
});

describe("收口：对比度公式本身可信（防止测试自我安慰）", () => {
  it("黑白对比度约 21:1", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 1);
  });

  it("同色对比度为 1:1", () => {
    expect(contrast("#1c1b19", "#1c1b19")).toBeCloseTo(1, 5);
  });
});
