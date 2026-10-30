import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { buildReaderThemeOverrides, parseReaderThemePalette } from "~/utils/readerTheme";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const fontsCss = readFileSync(join(projectDir, "app/assets/css/fonts.css"), "utf8");
const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");

const faces = [...fontsCss.matchAll(/@font-face\s*\{([^}]*)\}/gu)].map((match) => match[1].trim());

function facesOfFamily(family: string): string[] {
  const pattern = new RegExp(`font-family:\\s*["']${family}["']`, "u");
  return faces.filter((face) => pattern.test(face));
}

describe("中文自托管字体：声明真的指向可加载的文件", () => {
  it("Noto Sans SC 的 400 / 700 两档都有 @font-face（依赖早就在，之前只是从未加载）", () => {
    const weights = facesOfFamily("Noto Sans SC").map(
      (face) => /\bfont-weight:\s*(\d+)/u.exec(face)?.[1] ?? "?",
    );

    expect(weights.sort()).toEqual(["400", "700"]);
  });

  it("每一条 src 的 woff2 都真实存在于 node_modules（拼错路径 = 字体静默不加载）", () => {
    for (const face of facesOfFamily("Noto Sans SC")) {
      const urls = [...face.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)].map((match) => match[1]);

      expect(urls.length).toBeGreaterThan(0);
      for (const url of urls) {
        expect(url).toMatch(/^@fontsource\/noto-sans-sc\/files\//u);
        expect(existsSync(join(projectDir, "node_modules", url))).toBe(true);
      }
    }
  });

  it("用 font-display: swap（首屏先出字，不白屏）", () => {
    for (const face of facesOfFamily("Noto Sans SC")) {
      expect(face).toMatch(/font-display:\s*swap/u);
    }
  });

  it("配有度量匹配 fallback：本地中文族 + ascent/descent/size-adjust 覆盖（避免换字跳版）", () => {
    const fallback = facesOfFamily("Noto Sans SC Fallback")[0];

    expect(fallback).toBeDefined();
    expect(fallback).toMatch(/src:\s*local\(/u);
    expect(fallback).toMatch(/ascent-override:\s*[\d.]+%/u);
    expect(fallback).toMatch(/descent-override:\s*[\d.]+%/u);
    expect(fallback).toMatch(/line-gap-override:\s*0%/u);
    expect(fallback).toMatch(/size-adjust:\s*100%/u);
  });

  it("既有拉丁字体 Sora 没有被中文改造挤掉", () => {
    expect(facesOfFamily("Sora").length).toBeGreaterThanOrEqual(3);
    expect(facesOfFamily("Sora Fallback").length).toBe(1);
  });
});

describe("字体栈的三个消费点都指向 Noto Sans SC", () => {
  it("body 消费 --font-sans（tokens.css 里把中文组排在系统中文回退之前）", () => {
    expect(tailwindCss).toMatch(/font-family:\s*var\(--font-sans\)/u);
  });

  it("Naive UI 的 themeOverrides 字体栈把 Noto Sans SC 排在中文字体之前", () => {
    // 主题栈由 app/utils/readerTheme.ts 从 tokens.css 解析后组装（见 readerTheme.test.ts）。
    const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
    const stack = String(buildReaderThemeOverrides(parseReaderThemePalette(tokensCss)).common?.fontFamily);

    expect(stack).toContain('"Noto Sans SC"');
    expect(stack).toContain('"Noto Sans SC Fallback"');
    expect(stack.indexOf('"Noto Sans SC"')).toBeLessThan(stack.indexOf('"PingFang SC"'));
  });
});
