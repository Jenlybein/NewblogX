import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  READER_FONT_STACK,
  buildReaderThemeOverrides,
  parseReaderThemePalette,
} from "~/utils/readerTheme";

// vitest 以 project root（frontend/）为 cwd。
const tokensCss = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

describe("读者站 Naive 主题（旧青绿退场）", () => {
  const palette = parseReaderThemePalette(tokensCss);
  const overrides = buildReaderThemeOverrides(palette);

  it("从 tokens.css 取真实色值（不是 CSS 变量占位）", () => {
    expect(palette.blue).toBe("#002fa7");
    expect(palette.blue600).toBe("#3a55a8");
    expect(palette.blue800).toBe("#001f70");
    expect(palette.cream).toBe("#fdfbf6");
  });

  it("主色是克莱因蓝，hover / pressed 用同色相派生明度", () => {
    expect(overrides.common?.primaryColor).toBe(palette.blue);
    expect(overrides.common?.primaryColorHover).toBe(palette.blue600);
    expect(overrides.common?.primaryColorPressed).toBe(palette.blue800);
    expect(overrides.common?.primaryColorSuppl).toBe(palette.blue600);
    // 不能再用旧青绿（#0f766e 一族）。
    expect(JSON.stringify(overrides)).not.toMatch(/var\(/u);
  });

  it("圆角归零（卡片 / 按钮 / 输入 / 弹层选择器）", () => {
    expect(overrides.common?.borderRadius).toBe("0");
    expect(overrides.Card?.borderRadius).toBe("0");
    expect(overrides.Button?.borderRadiusMedium).toBe("0");
    expect(overrides.Input?.borderRadius).toBe("0");
    expect(overrides.Select?.peers?.InternalSelection?.borderRadius).toBe("0");
  });

  it("字体栈含中文 fallback（Noto 在系统中文之前）", () => {
    const stack = String(overrides.common?.fontFamily);

    expect(stack).toBe(READER_FONT_STACK);
    expect(stack.indexOf('"Noto Sans SC"')).toBeLessThan(stack.indexOf('"PingFang SC"'));
  });

  it("缺 token 时抛错（避免静默回落到 Naive 默认青绿）", () => {
    expect(() => parseReaderThemePalette(":root { --jelly-blue: #002fa7; }")).toThrow(/缺少原始层 token/u);
  });
});
