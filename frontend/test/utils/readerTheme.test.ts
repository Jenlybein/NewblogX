import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  READER_FONT_STACK,
  buildReaderThemeOverrides,
  parseReaderThemePalette,
  parseReaderThemeScale,
} from "~/utils/readerTheme";

// vitest 以 project root（frontend/）为 cwd。
const tokensCss = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

describe("读者站 Naive 主题（克莱因蓝退场：CTA 是墨水实心胶囊）", () => {
  const lightPalette = parseReaderThemePalette(tokensCss, "light");
  const darkPalette = parseReaderThemePalette(tokensCss, "dark");
  const scale = parseReaderThemeScale(tokensCss);

  it("从 tokens.css 取真实色值（不是 CSS 变量占位），明暗各一份", () => {
    expect(lightPalette.ink).toBe("#1c1b19");
    expect(lightPalette.inkMuted).toBe("#625c52");
    expect(lightPalette.surface).toBe("#fdfbf6");
    expect(lightPalette.page).toBe("#f5f3ee");
    expect(lightPalette.link).toBe("#0066cc");
    expect(lightPalette.danger).toBe("#ff6b4a");
    expect(lightPalette.inverse).toBe("#ffffff");

    expect(darkPalette.ink).toBe("#f5f5f7");
    expect(darkPalette.inkMuted).toBe("#a1a1a6");
    expect(darkPalette.surface).toBe("#1d1d1f");
    expect(darkPalette.page).toBe("#161617");
    expect(darkPalette.link).toBe("#2997ff");
    expect(darkPalette.inverse).toBe("#f5f5f7");
  });

  it("圆角从 token 阶梯取值（8 / 14 / 20 / 28 / pill）", () => {
    expect(scale).toEqual({ sm: "8px", md: "14px", lg: "20px", xl: "28px", pill: "999px" });
  });

  it("主色 = 墨色（CTA 是墨水实心胶囊），不是克莱因蓝、不是功能蓝", () => {
    const overrides = buildReaderThemeOverrides(lightPalette, scale);

    expect(overrides.common?.primaryColor).toBe(lightPalette.ink);
    expect(overrides.common?.primaryColorHover).toBe(lightPalette.inkMuted);
    expect(overrides.common?.primaryColorPressed).toBe(lightPalette.ink);
    expect(JSON.stringify(overrides)).not.toMatch(/#002fa7/u);
  });

  it("info 位用功能蓝（链接/信息态），危险态用珊瑚橙，且暗色映射同源", () => {
    const light = buildReaderThemeOverrides(lightPalette, scale);
    const dark = buildReaderThemeOverrides(darkPalette, scale);

    expect(light.common?.infoColor).toBe(lightPalette.link);
    expect(dark.common?.infoColor).toBe(darkPalette.link);
    expect(light.common?.errorColor).toBe("#ff6b4a");
    expect(dark.common?.primaryColor).toBe(darkPalette.ink);
    expect(light.common?.baseColor).toBe(lightPalette.surface);
    expect(dark.common?.baseColor).toBe(darkPalette.surface);
  });

  it("按钮 / 输入 / 弹窗圆角跟随 token，不再是 0", () => {
        const overrides = buildReaderThemeOverrides(lightPalette, scale);

    expect(overrides.common?.borderRadius).toBe(scale.md);
    expect(overrides.common?.borderRadiusSmall).toBe(scale.sm);
    expect(overrides.Card?.borderRadius).toBe(scale.lg);
    expect(overrides.Button?.borderRadiusTiny).toBe(scale.sm);
    expect(overrides.Button?.borderRadiusSmall).toBe(scale.sm);
    expect(overrides.Button?.borderRadiusMedium).toBe(scale.md);
    expect(overrides.Button?.borderRadiusLarge).toBe(scale.md);
    expect(overrides.Input?.borderRadius).toBe(scale.md);
    expect(overrides.Select?.peers?.InternalSelection?.borderRadius).toBe(scale.md);
    expect(overrides.Modal?.borderRadius).toBe(scale.xl);
    expect(overrides.Drawer?.borderRadius).toBe(scale.xl);
    expect(overrides.Popover?.borderRadius).toBe(scale.xl);

    // 覆盖层里不能残留圆角 0 或 CSS 变量占位（Naive 只吃真实色值）。
    expect(JSON.stringify(overrides)).not.toMatch(/"(?:borderRadius(?:Small|Tiny|Medium|Large)?|borderRadius)":"0"/u);
    expect(JSON.stringify(overrides)).not.toMatch(/var\(/u);
  });

  it("字体栈先 Inter、后系统中文（不再有自托管 Noto fallback）", () => {
    const overrides = buildReaderThemeOverrides(lightPalette, scale);
    const stack = String(overrides.common?.fontFamily);

    expect(stack).toBe(READER_FONT_STACK);
    expect(stack.startsWith('"Inter", "Inter Fallback"')).toBe(true);
    expect(stack).not.toContain('"Noto Sans SC Fallback"');
    expect(stack).toContain('"PingFang SC"');
    expect(stack.indexOf('"PingFang SC"')).toBeLessThan(stack.indexOf('"Microsoft YaHei"'));
    expect(stack.indexOf('"Microsoft YaHei"')).toBeLessThan(stack.indexOf('"Noto Sans SC"'));
  });

  it("缺 token 时抛错（避免静默回落到 Naive 默认青绿）", () => {
    expect(() => parseReaderThemePalette(":root { --color-ink: #1c1b19; }")).toThrow(/缺 token/u);
    expect(() => parseReaderThemeScale(":root { --radius-sm: 8px; }")).toThrow(/缺圆角 token/u);
  });
});
