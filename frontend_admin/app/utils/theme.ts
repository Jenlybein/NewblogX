/**
 * 后台主题层：把 `tokens.css` 的原始层色值翻译成 Naive UI 的 themeOverrides。
 *
 * 为什么需要这个模块（而不是在 app.vue 里写一串十六进制）：
 *
 *   1. **颜色只有一个来源**。spec 决策 1/8 要求颜色只在语义 token 层定义；这里
 *      从 `tokens.css?raw` 里解析 `--jelly-*`，不在 TS 里重复任何色值。
 *   2. **Naive 不接受 CSS 变量当颜色**。它的 `self()` 会用 seemly 的
 *      `changeColor()` / `composite()` 对传入颜色做运算（Tag、Alert、DataTable、
 *      Menu 等都会），传 `var(--color-x)` 会抛 "Invalid color value"。所以这里必须
 *      给出真实色值，且派生明度（hover / pressed）也用 token 层的 -600 / -800 档。
 *   3. **可测**。主题取值是纯函数（解析 + 色相派生 + 覆盖对象组装），单测可以直接
 *      断言"主色是蓝、hover 更亮、危险字用派生暗橙、圆角为 0、字体栈含中文"。
 *
 * 约束（spec 决策 7）：后台不做暗色、不放捏捏、动效只到 hover/focus。
 */
import type { GlobalThemeOverrides } from "naive-ui";

/** 原始层 token：色相 + 同色相明度/透明度派生。键名 → `tokens.css` 里的变量名。 */
export const RAW_TOKEN_VARS = {
  blue: "--jelly-blue",
  blue400: "--jelly-blue-400",
  blue600: "--jelly-blue-600",
  blue800: "--jelly-blue-800",
  blue900: "--jelly-blue-900",
  cream: "--jelly-cream",
  cream050: "--jelly-cream-050",
  cream700: "--jelly-cream-700",
  warm: "--jelly-warm",
  warm800: "--jelly-warm-800",
  yellow: "--jelly-yellow",
  coral: "--jelly-coral",
  coral300: "--jelly-coral-300",
  coral700: "--jelly-coral-700",
} as const;

export type RawTone = keyof typeof RAW_TOKEN_VARS;
export type RawPalette = Record<RawTone, string>;

/** 英文主字体 + 中文自托管字体 + 系统中文 fallback（顺序有意义）。 */
export const FONT_STACK =
  '"Sora", "Sora Fallback", "Noto Sans SC", "Noto Sans SC Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

export const FONT_MONO =
  '"Cascadia Code", "JetBrains Mono", "Sofia Sans Mono", ui-monospace, monospace';

/**
 * 从 `tokens.css` 源码解析出 Naive 要用的真实色值。
 *
 * 缺任何一个 token 都直接抛错：静默回落到 Naive 默认色（青绿 #18a058）会让
 * "后台看起来还是旧主题"，而这是最难在 CI 里发现的一类回归。
 */
export function parseRawPalette(cssText: string): RawPalette {
  const palette = {} as RawPalette;

  for (const [tone, tokenVar] of Object.entries(RAW_TOKEN_VARS)) {
    const match = new RegExp(`${tokenVar}\\s*:\\s*([^;]+);`, "u").exec(cssText);
    if (!match) {
      throw new Error(
        `tokens.css 缺少原始层 token ${tokenVar}（Naive 主题需要真实色值，不能使用 CSS 变量占位）`,
      );
    }
    palette[tone as RawTone] = match[1]!.trim();
  }

  return palette;
}

function hexToRgb(hex: string): [number, number, number] {
  const raw = hex.trim().replace(/^#/u, "");
  const expanded =
    raw.length === 3
      ? raw
          .split("")
          .map((channel) => channel + channel)
          .join("")
      : raw;

  if (!/^[0-9a-f]{6}$/iu.test(expanded)) {
    throw new Error(`不是合法的十六进制色值：${hex}`);
  }

  return [
    Number.parseInt(expanded.slice(0, 2), 16),
    Number.parseInt(expanded.slice(2, 4), 16),
    Number.parseInt(expanded.slice(4, 6), 16),
  ];
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/** 同色相透明度派生：`rgba()` 属于 token 层语义（结构线、危险底等）。 */
export function withAlpha(hex: string, alpha: number): string {
  const [red, green, blue] = hexToRgb(hex);

  return `rgba(${red}, ${green}, ${blue}, ${clamp(alpha, 0, 1)})`;
}

/**
 * 同色相明度派生：正数向白提亮、负数向黑压暗，按 sRGB 通道线性的混色。
 *
 * 只用来补 token 层没有列出的中间档（例如 warning 的 hover / pressed）；
 * 已有的 -600 / -800 档优先直接用 token，不要在这里重复造值。
 */
export function shade(hex: string, amount: number): string {
  const [red, green, blue] = hexToRgb(hex);
  const ratio = clamp(amount, -1, 1);
  const channel = (value: number) =>
    Math.round(ratio >= 0 ? value + (255 - value) * ratio : value * (1 + ratio));
  const toHex = (value: number) => value.toString(16).padStart(2, "0");

  return `#${toHex(channel(red))}${toHex(channel(green))}${toHex(channel(blue))}`;
}

/** 把半透明色压到不透明底色上（语义 token 里的 rgba 结构线/危险底需要它做对比度校验）。 */
export function flatten(hex: string, alpha: number, backgroundHex: string): string {
  const [red, green, blue] = hexToRgb(hex);
  const [bgRed, bgGreen, bgBlue] = hexToRgb(backgroundHex);
  const ratio = clamp(alpha, 0, 1);
  const channel = (value: number, background: number) =>
    Math.round(value * ratio + background * (1 - ratio));
  const toHex = (value: number) => value.toString(16).padStart(2, "0");

  return `#${toHex(channel(red, bgRed))}${toHex(channel(green, bgGreen))}${toHex(channel(blue, bgBlue))}`;
}

function channelLuminance(value: number): number {
  const normalized = value / 255;

  return normalized <= 0.04045
    ? normalized / 12.92
    : ((normalized + 0.055) / 1.055) ** 2.4;
}

/** WCAG 2.x 相对亮度。 */
export function relativeLuminance(hex: string): number {
  const [red, green, blue] = hexToRgb(hex);

  return (
    0.2126 * channelLuminance(red) +
    0.7152 * channelLuminance(green) +
    0.0722 * channelLuminance(blue)
  );
}

/** WCAG 2.x 对比度（1..21）。AA 正文门槛 4.5，UI 边界门槛 3。 */
export function contrastRatio(first: string, second: string): number {
  const lighter = Math.max(relativeLuminance(first), relativeLuminance(second));
  const darker = Math.min(relativeLuminance(first), relativeLuminance(second));

  return (lighter + 0.05) / (darker + 0.05);
}

/**
 * 生成后台的 Naive 主题覆盖。
 *
 * 颜色策略（全部来自原始层 + 同色相派生，不出现第六个色相）：
 *   - primary   = 克莱因蓝；hover = -600（更亮）、pressed = -800（更暗）
 *   - info/success = 蓝的深浅档（五色锁死不引入绿/天蓝）
 *   - warning   = 包豪斯黄，且**只做底**：文字/图标一律换墨色（黄永不做字色）
 *   - error     = 珊瑚橙；按钮里珊瑚橙底用墨色字（白字只有 2.8:1），按下改暗橙 + 奶油白字
 *   - 中性色    = 奶油白纸面 / 暖灰抬升 / 克莱因蓝墨色，结构线 1px 蓝
 *   - 圆角 0（头像/胶囊/捏捏除外，后台不涉及）
 */
export function buildNaiveThemeOverrides(palette: RawPalette): GlobalThemeOverrides {
  const ink = palette.blue900;
  const inkMuted = palette.blue600;
  const divider = withAlpha(palette.blue, 0.18);

  const overrides: GlobalThemeOverrides = {
    common: {
      // 主色 + 派生明度
      primaryColor: palette.blue,
      primaryColorHover: palette.blue600,
      primaryColorPressed: palette.blue800,
      primaryColorSuppl: palette.blue600,
      // 五色锁死：info / success 用蓝的深浅档，不引入绿 / 天蓝
      infoColor: palette.blue800,
      infoColorHover: palette.blue600,
      infoColorPressed: palette.blue900,
      infoColorSuppl: palette.blue600,
      successColor: palette.blue800,
      successColorHover: palette.blue600,
      successColorPressed: palette.blue900,
      successColorSuppl: palette.blue600,
      // 黄只做填充/底：hover 与按下是同色相明度派生
      warningColor: palette.yellow,
      warningColorHover: shade(palette.yellow, 0.12),
      warningColorPressed: shade(palette.yellow, -0.14),
      warningColorSuppl: shade(palette.yellow, 0.12),
      // 危险 = 珊瑚橙；按下用派生暗橙（coral-700，达 AA）
      errorColor: palette.coral,
      errorColorHover: palette.coral300,
      errorColorPressed: palette.coral700,
      errorColorSuppl: palette.coral,
      // 圆角归零
      borderRadius: "0",
      borderRadiusSmall: "0",
      // 字体栈含中文 fallback
      fontFamily: FONT_STACK,
      fontFamilyMono: FONT_MONO,
      // 中性色：纸面 / 边界 / 墨色
      baseColor: palette.cream,
      textColor1: ink,
      textColor2: ink,
      textColor3: inkMuted,
      placeholderColor: withAlpha(ink, 0.7),
      // 表单边界属于「识别控件所必需的信息」，WCAG 1.4.11 要求 3:1；
      // --color-line（0.32）压到奶油白上只有 ~2:1，所以控件边界用 --color-line-strong 档。
      borderColor: withAlpha(palette.blue, 0.62),
      dividerColor: divider,
      bodyColor: palette.cream,
      cardColor: palette.cream050,
      modalColor: palette.cream050,
      popoverColor: palette.cream050,
      tableColor: palette.cream050,
      tableHeaderColor: palette.warm,
      tableColorHover: withAlpha(palette.blue, 0.04),
      tableColorStriped: withAlpha(palette.blue, 0.02),
      inputColor: palette.cream050,
      inputColorDisabled: palette.warm,
      actionColor: palette.warm,
      tagColor: palette.warm,
      tabColor: palette.warm,
      hoverColor: withAlpha(palette.blue, 0.06),
      pressedColor: withAlpha(palette.blue, 0.12),
      avatarColor: withAlpha(palette.blue, 0.12),
      codeColor: withAlpha(palette.blue, 0.06),
      buttonColor2: withAlpha(palette.blue, 0.06),
      buttonColor2Hover: withAlpha(palette.blue, 0.1),
      buttonColor2Pressed: withAlpha(palette.blue, 0.15),
      scrollbarColor: withAlpha(palette.blue, 0.25),
      scrollbarColorHover: withAlpha(palette.blue, 0.4),
      progressRailColor: withAlpha(palette.blue, 0.08),
      railColor: palette.warm800,
    },
    Button: {
      borderRadiusTiny: "0",
      borderRadiusSmall: "0",
      borderRadiusMedium: "0",
      borderRadiusLarge: "0",
      // filled 危险按钮：珊瑚橙底 + 墨色字（奶油白字只有 2.8:1）
      textColorError: ink,
      textColorHoverError: ink,
      textColorFocusError: ink,
      textColorPressedError: palette.cream,
      textColorDisabledError: inkMuted,
      // tertiary / ghost / text 形态：暗橙字，奶油白底上 5.0:1
      textColorTextError: palette.coral700,
      textColorTextHoverError: palette.coral700,
      textColorTextPressedError: palette.coral700,
      textColorTextFocusError: palette.coral700,
      textColorGhostError: palette.coral700,
      textColorGhostHoverError: palette.coral700,
      textColorGhostPressedError: palette.coral700,
      textColorGhostFocusError: palette.coral700,
      // 万一出现 warning 实心按钮：黄底必须配墨色字
      textColorWarning: ink,
      textColorHoverWarning: ink,
      textColorFocusWarning: ink,
      textColorPressedWarning: ink,
    },
    Tag: {
      borderRadius: "0",
      // 默认标签：暖灰底 + 墨色字
      textColor: ink,
      textColorPrimary: palette.blue,
      textColorSuccess: palette.blue800,
      // 黄永不做字色：warning 标签文字用墨色
      textColorWarning: ink,
      // 珊瑚橙作字色不可达 AA，改用派生暗橙
      textColorError: palette.coral700,
      colorWarning: withAlpha(palette.yellow, 0.15),
      colorError: withAlpha(palette.coral, 0.1),
      colorSuccess: withAlpha(palette.blue, 0.1),
      colorPrimary: withAlpha(palette.blue, 0.1),
      // 结构线一律 1px 蓝（warning 标签的黄边在奶油白上不足 3:1）
      borderWarning: `1px solid ${withAlpha(ink, 0.3)}`,
      borderError: `1px solid ${withAlpha(palette.coral, 0.36)}`,
      borderSuccess: `1px solid ${withAlpha(palette.blue, 0.3)}`,
      borderPrimary: `1px solid ${withAlpha(palette.blue, 0.3)}`,
    },
    Alert: {
      borderRadius: "0",
      // 黄底警示：图标/标题/正文都用墨色（黄图标在浅黄底上读不出来）
      iconColorWarning: ink,
      titleTextColorWarning: ink,
      contentTextColorWarning: inkMuted,
      iconColorInfo: palette.blue800,
      titleTextColorInfo: ink,
      contentTextColorInfo: inkMuted,
      iconColorError: palette.coral700,
      titleTextColorError: ink,
      contentTextColorError: inkMuted,
    },
    Card: { borderRadius: "0" },
    Input: { borderRadius: "0" },
    Select: { peers: { InternalSelection: { borderRadius: "0" } } },
    Dropdown: { borderRadius: "0" },
    Dialog: {
      borderRadius: "0",
      // dialog.warning/error 的图标颜色默认直接取 warningColor/errorColor：
      // 黄图标在奶油白上读不出来，珊瑚橙图标也不足 3:1，统一换墨色/暗橙。
      iconColor: palette.blue,
      iconColorInfo: palette.blue800,
      iconColorSuccess: palette.blue800,
      iconColorWarning: ink,
      iconColorError: palette.coral700,
    },
    Message: {
      borderRadius: "0",
      iconColorWarning: ink,
      iconColorError: palette.coral700,
      iconColorInfo: palette.blue800,
      iconColorSuccess: palette.blue800,
    },
  };

  return overrides;
}
