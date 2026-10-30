import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  FONT_MONO,
  FONT_STACK,
  RAW_TOKEN_VARS,
  buildNaiveThemeOverrides,
  contrastRatio,
  flatten,
  parseRawPalette,
  shade,
  withAlpha,
} from "../../app/utils/theme";

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

describe("原始层解析：颜色只有一个来源（tokens.css）", () => {
  it("把 tokens.css 的 --jelly-* 解析成 Naive 能吃的真实色值", () => {
    const palette = parseRawPalette(tokensCss);

    // 五个锁定色相取到 spec 给定的字面量（独立于实现，来自设计规格）。
    expect(palette.blue).toBe("#002fa7");
    expect(palette.cream).toBe("#fdfbf6");
    expect(palette.yellow).toBe("#f5c518");
    expect(palette.coral).toBe("#ff6b4a");
    expect(palette.warm).toBe("#f2eee6");

    // Naive 的 theme self() 会拿颜色做 changeColor/composite 运算，var() 会直接抛错，
    // 所以这里必须拿到真色值而不是 CSS 变量。
    expect(Object.values(palette).every((value) => /^#[0-9a-f]{6}$/u.test(value))).toBe(true);
  });

  it("同色相派生明度也来自 token 层（hover / pressed 有档位可换）", () => {
    const palette = parseRawPalette(tokensCss);

    expect(palette.blue600).toBe("#3a55a8");
    expect(palette.blue800).toBe("#001f70");
    expect(palette.coral300).toBe("#ff9b85");
    expect(palette.coral700).toBe("#c2410c");
  });

  it("缺 token 时直接抛错（避免静默回落到 Naive 默认青绿）", () => {
    expect(() => parseRawPalette(":root { --jelly-blue: #002fa7; }")).toThrow(
      /缺少原始层 token/u,
    );
  });

  it("映射表覆盖全部要用的原始 token", () => {
    expect(Object.keys(RAW_TOKEN_VARS)).toContain("blue");
    expect(RAW_TOKEN_VARS.coral700).toBe("--jelly-coral-700");
  });
});

describe("同色相派生工具（纯函数）", () => {
  it("withAlpha 输出 rgba，不改变色相", () => {
    expect(withAlpha("#002fa7", 0.32)).toBe("rgba(0, 47, 167, 0.32)");
    expect(withAlpha("#abc", 0.5)).toBe("rgba(170, 187, 204, 0.5)");
  });

  it("withAlpha 把 alpha 钳制在 0..1", () => {
    expect(withAlpha("#000000", 2)).toBe("rgba(0, 0, 0, 1)");
    expect(withAlpha("#000000", -1)).toBe("rgba(0, 0, 0, 0)");
  });

  it("withAlpha 拒绝非法色值", () => {
    expect(() => withAlpha("blue", 0.5)).toThrow(/十六进制/u);
  });

  it("shade 正数提亮、负数压暗，并钳制在 +/-100%", () => {
    expect(shade("#000000", 0.5)).toBe("#808080");
    expect(shade("#ffffff", -0.5)).toBe("#808080");
    expect(shade("#ff0000", -0.2)).toBe("#cc0000");
    expect(shade("#002fa7", 2)).toBe("#ffffff");
    expect(shade("#002fa7", -2)).toBe("#000000");
  });

  it("flatten 把半透明 token 压到底色上（用于对比度校验）", () => {
    expect(flatten("#002fa7", 0.5, "#ffffff")).toBe("#8097d3");
    expect(flatten("#002fa7", 0.32, "#fdfbf6")).toBe("#acbadd");
    expect(flatten("#002fa7", 1, "#ffffff")).toBe("#002fa7");
    expect(flatten("#002fa7", 0, "#fdfbf6")).toBe("#fdfbf6");
  });
});

describe("对比度：关键文字/控件边界达到 WCAG AA", () => {
  const palette = parseRawPalette(tokensCss);

  it("对比度公式符合 WCAG（黑/白 = 21:1，同色 = 1:1）", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 1);
    expect(contrastRatio("#ffffff", "#000000")).toBeCloseTo(21, 1);
    expect(contrastRatio("#777777", "#777777")).toBeCloseTo(1, 5);
  });

  it("正文 / 次级墨色在奶油白上 ≥ 4.5:1", () => {
    expect(contrastRatio(palette.blue900, palette.cream)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.blue600, palette.cream)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.blue600, palette.cream050)).toBeGreaterThanOrEqual(4.5);
  });

  it("危险文字用派生暗橙：奶油白底与浅珊瑚底都 ≥ 4.5:1", () => {
    const paleCoral = flatten(palette.coral, 0.1, palette.cream050);

    expect(contrastRatio(palette.coral700, palette.cream050)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.coral700, paleCoral)).toBeGreaterThanOrEqual(4.5);
    // 珊瑚橙本身当字色达不到 AA（这正是派生暗橙存在的原因）。
    expect(contrastRatio(palette.coral, palette.cream050)).toBeLessThan(4.5);
  });

  it("filled 按钮：primary 用奶油白字、danger 用墨色字、danger 按下用奶油白字", () => {
    expect(contrastRatio(palette.cream, palette.blue)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.cream, palette.blue600)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.blue900, palette.coral)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.cream, palette.coral700)).toBeGreaterThanOrEqual(4.5);
    // 白字压在珊瑚橙上只有 ~2.8:1 —— 所以 Button 危险态必须改成墨色字。
    expect(contrastRatio(palette.cream, palette.coral)).toBeLessThan(4.5);
  });

  it("黄色只做底：墨色字压在黄底/浅黄底上 ≥ 4.5:1", () => {
    const paleYellow = flatten(palette.yellow, 0.15, palette.cream050);

    expect(contrastRatio(palette.blue900, palette.yellow)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(palette.blue900, paleYellow)).toBeGreaterThanOrEqual(4.5);
  });

  it("表单控件边界（--color-line-strong 档压到纸面）≥ 3:1", () => {
    const border = flatten(palette.blue, 0.62, palette.cream050);
    const weakBorder = flatten(palette.blue, 0.32, palette.cream050);

    expect(contrastRatio(border, palette.cream050)).toBeGreaterThanOrEqual(3);
    // 0.32 档只够画装饰性分栏，不满足控件边界的 3:1（所以主题里没把它用在输入框上）。
    expect(contrastRatio(weakBorder, palette.cream050)).toBeLessThan(3);
  });
});

describe("Naive themeOverrides：克莱因蓝主色 + 派生明度 + 圆角归零", () => {
  const palette = parseRawPalette(tokensCss);
  const overrides = buildNaiveThemeOverrides(palette);
  const common = overrides.common ?? {};
  const button = overrides.Button ?? {};
  const tag = overrides.Tag ?? {};
  const alert = overrides.Alert ?? {};

  it("主色是克莱因蓝，hover/按下用同色相派生明度而不是新色相", () => {
    expect(common.primaryColor).toBe(palette.blue);
    expect(common.primaryColorHover).toBe(palette.blue600);
    expect(common.primaryColorPressed).toBe(palette.blue800);
    expect(common.primaryColorSuppl).toBe(palette.blue600);
  });

  it("危险色是珊瑚橙，文字用派生暗橙（达 AA）", () => {
    expect(common.errorColor).toBe(palette.coral);
    expect(common.errorColorPressed).toBe(palette.coral700);
    // 珊瑚橙作字色时对比度不足 4.5:1，必须换成派生暗橙。
    expect(tag.textColorError).toBe(palette.coral700);
    expect(button.textColorTextError).toBe(palette.coral700);
  });

  it("黄色永不做字色：warning 系列的文字一律落到墨色", () => {
    expect(common.warningColor).toBe(palette.yellow);
    expect(tag.textColorWarning).toBe(palette.blue900);
    expect(alert.iconColorWarning).toBe(palette.blue900);
  });

  it("圆角归零（卡片/按钮/输入/弹窗/标签/表格）", () => {
    expect(common.borderRadius).toBe("0");
    expect(common.borderRadiusSmall).toBe("0");
    expect(button.borderRadiusMedium).toBe("0");
    expect(overrides.Card?.borderRadius).toBe("0");
    expect(overrides.Input?.borderRadius).toBe("0");
    expect(overrides.Select?.peers?.InternalSelection?.borderRadius).toBe("0");
    expect(tag.borderRadius).toBe("0");
    expect(alert.borderRadius).toBe("0");
    expect(overrides.Dialog?.borderRadius).toBe("0");
  });

  it("字体栈含中文 fallback（英文 Sora 在前，中文 Noto 在系统中文之前）", () => {
    const stack = String(common.fontFamily);

    expect(stack).toBe(FONT_STACK);
    expect(stack).toContain('"Sora"');
    expect(stack).toContain('"Noto Sans SC"');
    expect(stack).toContain('"Noto Sans SC Fallback"');
    expect(stack.indexOf('"Noto Sans SC"')).toBeLessThan(stack.indexOf('"PingFang SC"'));
    expect(String(common.fontFamilyMono)).toBe(FONT_MONO);
  });

  it("颜色值全部是可运算的真实色值（不能把 var() 喂给 Naive）", () => {
    const serialized = JSON.stringify(overrides);

    // Naive 的 TextColor / Tag / Alert 等主题会用 seemly.changeColor 解析传入的颜色，
    // var(--color-x) 会让它抛 "Invalid color value"，所以主题层必须是真色值。
    expect(serialized).not.toMatch(/var\(/u);
    expect(serialized).not.toMatch(
      /#18a058|#36ad6a|#2080f0|#4098fc|#d03050|#de576d|#f0a020|#fcb040/u,
    );
  });

  it("中性底色/墨色也来自 token（表格与弹窗不回到默认白灰）", () => {
    expect(common.bodyColor).toBe(palette.cream);
    expect(common.cardColor).toBe(palette.cream050);
    expect(common.modalColor).toBe(palette.cream050);
    expect(common.tableColor).toBe(palette.cream050);
    expect(common.textColor1).toBe(palette.blue900);
    expect(common.textColor2).toBe(palette.blue900);
    expect(common.textColor3).toBe(palette.blue600);
    expect(common.borderColor).toBe(withAlpha(palette.blue, 0.62));
  });

  it("filled 危险按钮在珊瑚橙上是墨色字（白字对比度只有 2.8:1）", () => {
    expect(button.textColorError).toBe(palette.blue900);
    expect(button.textColorHoverError).toBe(palette.blue900);
    // 按下用派生暗橙（coral-700），此时奶油白字对比度 5.2:1 达标。
    expect(button.textColorPressedError).toBe(palette.cream);
  });

  it("弹窗 / 全局消息的警示图标不落在黄或珊瑚橙上（非文本对比度 3:1）", () => {
    const dialog = overrides.Dialog ?? {};
    const message = overrides.Message ?? {};

    expect(dialog.iconColorWarning).toBe(palette.blue900);
    expect(dialog.iconColorError).toBe(palette.coral700);
    expect(message.iconColorWarning).toBe(palette.blue900);
    expect(message.iconColorError).toBe(palette.coral700);
  });
});
