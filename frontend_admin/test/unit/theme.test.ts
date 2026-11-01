import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  ALPHA,
  FONT_MONO,
  FONT_STACK,
  THEME_TOKEN_VARS,
  THEME_TRIPLET_VARS,
  buildNaiveThemeOverrides,
  contrastRatio,
  parseThemePalette,
  parseThemeScale,
  parseThemeTriplets,
  withTripletAlpha,
} from "../../app/utils/theme";

/**
 * 后台 Naive 主题层（#22 重写）：颜色只有一个来源 —— `tokens.css`。
 *
 * 与旧版的区别（本次验收的实质）：
 *   1. 不再解析原始层 hex 再自己拼;直接从**语义层**解析（明暗各一份取值）；
 *   2. 带透明度的颜色全部由 `--*-rgb` 三元组派生，源码里没有写死的 rgb 通道数字；
 *   3. 圆角从 `--radius-*` 五档取，不再归零；
 *   4. 暗色是同一组语义 token 的第二份取值（palette(mode)），不是另一套颜色。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const themeSource = readFileSync(join(projectDir, "app/utils/theme.ts"), "utf8");

const lightPalette = parseThemePalette(tokensCss, "light");
const darkPalette = parseThemePalette(tokensCss, "dark");
const lightTriplets = parseThemeTriplets(tokensCss, "light");
const darkTriplets = parseThemeTriplets(tokensCss, "dark");
const scale = parseThemeScale(tokensCss);
const lightOverrides = buildNaiveThemeOverrides(lightPalette, lightTriplets, scale);
const darkOverrides = buildNaiveThemeOverrides(darkPalette, darkTriplets, scale);

/** 递归收集覆盖对象里的所有颜色字符串（hex 与 rgba）。 */
function collectColors(node: unknown, found: string[] = []): string[] {
  if (typeof node === "string") {
    if (/^(?:#[0-9a-f]{6}|rgba?\()/iu.test(node)) found.push(node);
    return found;
  }
  if (node && typeof node === "object") {
    for (const value of Object.values(node)) collectColors(value, found);
  }
  return found;
}

describe("语义层解析：Naive 拿到真实色值（不能是 var()）", () => {
  it("把 tokens.css 的语义层解析成 hex / rgba", () => {
    expect(lightPalette.ink).toBe("#1c1b19");
    expect(lightPalette.surface).toBe("#fdfbf6");
    expect(lightPalette.onStrong).toBe("#ffffff");
    // 三元组派生的语义色被展开成 rgba，Naive 的 changeColor/composite 才吃得下。
    expect(lightPalette.line).toBe("rgba(28, 27, 25, 0.16)");
    expect(lightPalette.focusSoft).toBe("rgba(0, 102, 204, 0.10)");
    expect(lightPalette.onStrongMuted).toBe("rgba(255, 255, 255, 0.78)");
  });

  it("暗色是同一组名字的第二份取值（不是另一套颜色）", () => {
    expect(darkPalette.ink).toBe("#f5f5f7");
    expect(darkPalette.surface).toBe("#1d1d1f");
    expect(darkPalette.bgPage).toBe("#161617");
    expect(darkPalette.line).toBe("rgba(245, 245, 247, 0.14)");
    expect(Object.keys(darkPalette).sort()).toEqual(Object.keys(lightPalette).sort());
  });

  it("缺 token 直接抛错（不静默回落 Naive 默认青绿）", () => {
    expect(() => parseThemePalette(":root { --color-ink: #1c1b19; }", "light")).toThrow(
      /缺 token/u,
    );
    expect(() => parseThemeTriplets(":root { --color-ink: #1c1b19; }", "light")).toThrow(
      /缺三元组/u,
    );
    expect(() => parseThemeScale(":root { --radius-sm: 8px; }")).toThrow(/缺圆角 token/u);
  });

  it("三元组基底在暗色换第二份取值（石墨阶与功能蓝），其余五色相不变", () => {
    expect(lightTriplets.graphite).toBe("28 27 25");
    expect(darkTriplets.graphite).toBe("245 245 247");
    expect(lightTriplets.interactive).toBe("0 102 204");
    expect(darkTriplets.interactive).toBe("41 151 255");
    expect(darkTriplets.cream).toBe(lightTriplets.cream);
    expect(darkTriplets.onStrong).not.toBe(lightTriplets.onStrong);
  });

  it("映射表覆盖 Naive 要用的语义 token（键名 → tokens.css 变量名）", () => {
    expect(Object.keys(THEME_TOKEN_VARS).length).toBeGreaterThanOrEqual(20);
    expect(THEME_TOKEN_VARS.inkMuted).toBe("--color-ink-muted");
    expect(THEME_TRIPLET_VARS.graphite).toBe("--graphite-rgb");
  });
});

describe("透明度派生：全部走三元组，源码里没有写死的 rgb 通道", () => {
  it("withTripletAlpha 通道来自三元组，alpha 钳制在 0..1", () => {
    expect(withTripletAlpha("28 27 25", 0.32)).toBe("rgba(28, 27, 25, 0.32)");
    expect(withTripletAlpha("245 245 247", 2)).toBe("rgba(245, 245, 247, 1)");
    expect(withTripletAlpha("0 102 204", -1)).toBe("rgba(0, 102, 204, 0)");
  });

  it("withTripletAlpha 拒绝非法三元组", () => {
    expect(() => withTripletAlpha("#1c1b19", 0.5)).toThrow(/三元组/u);
  });

  it("alpha 常量集中在一张命名表里（不在调用处散落魔法数字）", () => {
    const values = Object.values(ALPHA);

    expect(values.length).toBeGreaterThanOrEqual(6);
    for (const value of values) expect(value).toBeGreaterThan(0);
    for (const value of values) expect(value).toBeLessThanOrEqual(1);
  });

  it("theme.ts 源码里没有 rgba(数字…) 常量，也没有裸 hex", () => {
    // 注释里允许写"旧写法"，所以先去掉注释再判定。
    const code = themeSource.replace(/\/\*[\s\S]*?\*\//gu, "").replace(/\/\/[^\n]*/gu, "");

    expect(code).not.toMatch(/\brgba\(\s*\d/u);
    expect(code).not.toMatch(/\brgb\(\s*\d/u);
    expect(code).not.toMatch(/#[0-9a-f]{3,8}\b/iu);
  });
});

describe("同色相派生工具（纯函数）", () => {
  it("对比度公式符合 WCAG（黑/白 = 21:1，同色 = 1:1）", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 1);
    expect(contrastRatio("#ffffff", "#000000")).toBeCloseTo(21, 1);
    expect(contrastRatio("#777777", "#777777")).toBeCloseTo(1, 5);
  });
});

describe("圆角：从 token 阶梯取（不再归零）", () => {
  it("五档取自 tokens.css 的真实取值", () => {
    expect(scale).toEqual({ sm: "8px", md: "14px", lg: "20px", xl: "28px", pill: "999px" });
  });

  it("控件 8–14 / 卡片 20 / 弹窗与浮层 28 / 胶囊 pill", () => {
    expect(lightOverrides.common?.borderRadius).toBe("14px");
    expect(lightOverrides.common?.borderRadiusSmall).toBe("8px");
    expect(lightOverrides.Button?.borderRadiusMedium).toBe("14px");
    expect(lightOverrides.Button?.borderRadiusTiny).toBe("8px");
    expect(lightOverrides.Input?.borderRadius).toBe("14px");
    expect(lightOverrides.Select?.peers?.InternalSelection?.borderRadius).toBe("14px");
    expect(lightOverrides.Card?.borderRadius).toBe("20px");
    expect(lightOverrides.Dialog?.borderRadius).toBe("28px");
    expect(lightOverrides.Dropdown?.borderRadius).toBe("28px");
    expect(lightOverrides.Popover?.borderRadius).toBe("28px");
    expect(lightOverrides.Tag?.borderRadius).toBe("999px");
  });

  it("暗色与亮色用同一套圆角（圆角不随主题变）", () => {
    expect(darkOverrides.common?.borderRadius).toBe(lightOverrides.common?.borderRadius);
    expect(darkOverrides.Card?.borderRadius).toBe(lightOverrides.Card?.borderRadius);
  });
});

describe("暗色接入：同一份覆盖对象换第二份取值即可", () => {
  it("中性面/墨色跟随 token 的暗色取值", () => {
    expect(lightOverrides.common?.bodyColor).toBe(lightPalette.surface);
    expect(darkOverrides.common?.bodyColor).toBe(darkPalette.surface);
    expect(lightOverrides.common?.textColor1).toBe(lightPalette.ink);
    expect(darkOverrides.common?.textColor1).toBe(darkPalette.ink);
    expect(darkOverrides.common?.cardColor).toBe(darkPalette.surfaceRaised);
    expect(darkOverrides.common?.tableColor).toBe(darkPalette.surfaceRaised);
    expect(darkOverrides.common?.tableHeaderColor).toBe(darkPalette.bgSubtle);
  });

  it("暗色下正文 / 次级墨 / 危险字 / 反白字对各自底面 ≥ 4.5:1", () => {
    const surface = darkPalette.surface;
    const raised = darkPalette.surfaceRaised;

    for (const [label, foreground, background] of [
      ["正文墨 / 纸面", darkPalette.ink, surface],
      ["正文墨 / 抬升面", darkPalette.ink, raised],
      ["次级墨 / 纸面", darkPalette.inkMuted, surface],
      ["次级墨 / 抬升面", darkPalette.inkMuted, raised],
      ["危险字 / 抬升面", darkPalette.dangerInk, raised],
      ["墨水实心 CTA 反白字", darkPalette.onStrong, darkPalette.ink],
      ["珊瑚橙底上的字", darkPalette.onDanger, darkPalette.danger],
    ] as const) {
      expect(contrastRatio(foreground, background), label).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("亮色下同样达标（同一份判定，不是只补暗色）", () => {
    expect(contrastRatio(lightPalette.ink, lightPalette.surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(lightPalette.inkMuted, lightPalette.surface)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(lightPalette.dangerInk, lightPalette.surfaceRaised)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(lightPalette.onStrong, lightPalette.ink)).toBeGreaterThanOrEqual(4.5);
  });

  it("控件边界（表单/表格/弹窗）明暗两态都 ≥ 3:1（WCAG 1.4.11）", () => {
    expect(contrastRatio(lightPalette.inkFaint, lightPalette.surfaceRaised)).toBeGreaterThanOrEqual(3);
    expect(contrastRatio(darkPalette.inkFaint, darkPalette.surfaceRaised)).toBeGreaterThanOrEqual(3);
  });
});

describe("颜色只有一个来源：覆盖对象里没有任何第二套色值", () => {
  it("每个颜色都来自解析出的 token 值或三元组派生", () => {
    const allowedHex = new Set(
      [lightPalette, darkPalette].flatMap((palette) =>
        Object.values(palette).filter((value) => value.startsWith("#")),
      ),
    );
    const allowedTriplets = new Set(
      [...Object.values(lightTriplets), ...Object.values(darkTriplets)],
    );

    for (const overrides of [lightOverrides, darkOverrides]) {
      const colors = collectColors(overrides);

      expect(colors.length).toBeGreaterThan(10);
      for (const color of colors) {
        if (color.startsWith("#")) {
          expect(allowedHex, `未知 hex ${color}`).toContain(color.toLowerCase());
          continue;
        }
        // rgba(r, g, b, α)：通道必须来自某个已解析的三元组基底。
        const channels = /rgba\((\d+), (\d+), (\d+), /u.exec(color);
        expect(channels, `无法解析 ${color}`).not.toBeNull();
        expect(allowedTriplets, `未知 rgb 通道 ${color}`).toContain(
          `${channels![1]} ${channels![2]} ${channels![3]}`,
        );
      }
    }
  });

  it("不残留克莱因蓝（色值、三通道、旧蓝档全部不许出现）", () => {
    const serialized = JSON.stringify([lightOverrides, darkOverrides]);

    expect(serialized).not.toMatch(/002fa7|001b63|001f70|00227a|3a55a8|6e86cf/iu);
    expect(serialized).not.toMatch(/0,\s*47,\s*167/u);
    for (const legacy of ["blue400", "blue600", "blue800", "blue900", "blue"]) {
      expect(Object.keys(THEME_TOKEN_VARS)).not.toContain(legacy);
    }
  });

  it("颜色值全部是可运算的真实色值（不能把 var() 喂给 Naive）", () => {
    expect(JSON.stringify([lightOverrides, darkOverrides])).not.toMatch(/var\(/u);
  });
});

describe("字体与语义细节保持既有口径", () => {
  const common = lightOverrides.common ?? {};
  const button = lightOverrides.Button ?? {};
  const tag = lightOverrides.Tag ?? {};

  it("字体栈含中文（英文 Inter 在前，中文走系统字体栈，不再自托管）", () => {
    const stack = String(common.fontFamily);

    expect(stack).toBe(FONT_STACK);
    expect(stack).not.toContain('"Sora"');
    expect(stack).toContain('"PingFang SC"');
    expect(stack).not.toContain('"Noto Sans SC Fallback"');
    expect(stack.indexOf('"PingFang SC"')).toBeLessThan(stack.indexOf('"Noto Sans SC"'));
    expect(String(common.fontFamilyMono)).toBe(FONT_MONO);
  });

  it("黄色永不做字色：warning 填充来自三元组，文字一律落到墨色", () => {
    expect(common.warningColor).toBe("rgba(245, 197, 24, 1)");
    expect(tag.textColorWarning).toBe(lightPalette.ink);
    expect(lightOverrides.Alert?.iconColorWarning).toBe(lightPalette.ink);
    expect(lightOverrides.Dialog?.iconColorWarning).toBe(lightPalette.ink);
  });

  it("危险文字用派生暗橙（亮）/ 浅珊瑚（暗），均达 AA", () => {
    expect(tag.textColorError).toBe(lightPalette.dangerInk);
    expect(button.textColorTextError).toBe(lightPalette.dangerInk);
    expect(contrastRatio(darkPalette.dangerInk, darkPalette.surfaceRaised)).toBeGreaterThanOrEqual(4.5);
  });

  it("filled 危险按钮：珊瑚橙底用墨色字（明暗两态都达 6:1）", () => {
    expect(button.textColorError).toBe(lightPalette.onDanger);
    expect(button.textColorHoverError).toBe(lightPalette.onDanger);
    // 珊瑚橙填充在两态都足够深，所以按下不换填充、字仍是墨色。
    expect(button.textColorPressedError).toBe(lightPalette.onDanger);
    expect(contrastRatio(lightPalette.onDanger, lightPalette.danger)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(darkPalette.onDanger, darkPalette.danger)).toBeGreaterThanOrEqual(4.5);
  });

  it("主色是墨色（CTA 实心胶囊），info 位是功能蓝", () => {
    expect(common.primaryColor).toBe(lightPalette.ink);
    expect(common.primaryColorHover).toBe(lightPalette.inkMuted);
    expect(common.infoColor).toBe(lightPalette.link);
    // 功能蓝只出现在链接/焦点：info 的悬停/按下退回中性色，不派生蓝色填充。
    expect(common.infoColorHover).toBe(lightPalette.inkMuted);
    expect(common.infoColorPressed).toBe(lightPalette.ink);
  });
});
