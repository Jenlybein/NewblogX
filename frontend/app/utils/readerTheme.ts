/**
 * 读者站 Naive 主题覆盖：取值来自 tokens.css（颜色只有一个来源）。
 *
 * 与后台那份（frontend_admin/app/utils/theme.ts）同思路但更小：
 *   1. `?raw` 导入 tokens.css → 解析 `--jelly-*` 真实色值。Naive 的 changeColor /
 *      composite 不接受 `var()`，所以主题层必须是真色值；
 *   2. 只覆盖**主色族 + 圆角 + 字体**，不覆盖中性纸面/墨色 —— 那些交给 Naive 的
 *      darkTheme 切（#12 暗色）；否则静态浅色值会在暗色下变成白块；
 *   3. 圆角归零（spec：只允许头像 / 胶囊 / 圆形进度 / 捏捏）。
 *
 * 纯函数，可单测；不 import Vue / Nuxt。
 */
import type { GlobalThemeOverrides } from "naive-ui";

/** 主题层需要的原始层 token（其余中性色交给 darkTheme）。 */
export const READER_THEME_TOKEN_VARS = {
  blue: "--jelly-blue",
  blue600: "--jelly-blue-600",
  blue800: "--jelly-blue-800",
  cream: "--jelly-cream",
} as const;

export type ReaderThemeTone = keyof typeof READER_THEME_TOKEN_VARS;
export type ReaderThemePalette = Record<ReaderThemeTone, string>;

/** 英文主字体 + 中文自托管字体 + 系统中文 fallback（顺序有意义）。 */
export const READER_FONT_STACK =
  '"Sora", "Sora Fallback", "Noto Sans SC", "Noto Sans SC Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

/** 从 tokens.css 源码解析主题层色值；缺 token 直接抛错（不静默回落默认青绿）。 */
export function parseReaderThemePalette(cssText: string): ReaderThemePalette {
  const palette = {} as ReaderThemePalette;

  for (const [tone, tokenVar] of Object.entries(READER_THEME_TOKEN_VARS)) {
    const match = new RegExp(`${tokenVar}\\s*:\\s*([^;]+);`, "u").exec(cssText);
    if (!match) {
      throw new Error(`tokens.css 缺少原始层 token ${tokenVar}（Naive 主题需要真实色值）`);
    }
    palette[tone as ReaderThemeTone] = match[1]!.trim();
  }

  return palette;
}

/**
 * 生成读者站的 Naive 覆盖：主色克莱因蓝、hover/pressed 用同色相派生明度、
 * 圆角 0、字体栈含中文 fallback。
 */
export function buildReaderThemeOverrides(palette: ReaderThemePalette): GlobalThemeOverrides {
  return {
    common: {
      primaryColor: palette.blue,
      primaryColorHover: palette.blue600,
      primaryColorPressed: palette.blue800,
      primaryColorSuppl: palette.blue600,
      baseColor: palette.cream,
      borderRadius: "0",
      borderRadiusSmall: "0",
      fontFamily: READER_FONT_STACK,
      fontFamilyMono: '"Cascadia Code", "JetBrains Mono", "Sofia Sans Mono", ui-monospace, monospace',
    },
    Card: { borderRadius: "0" },
    Button: {
      borderRadiusTiny: "0",
      borderRadiusSmall: "0",
      borderRadiusMedium: "0",
      borderRadiusLarge: "0",
    },
    Input: { borderRadius: "0" },
    Select: { peers: { InternalSelection: { borderRadius: "0" } } },
  };
}
