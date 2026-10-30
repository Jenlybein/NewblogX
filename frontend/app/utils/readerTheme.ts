/**
 * 读者站 Naive 主题覆盖：取值来自 tokens.css（颜色只有一个来源）。
 *
 * 与后台那份（frontend_admin/app/utils/theme.ts）同思路但更小：
 *   1. 用 `markdownTheme` 的 token 解析器把 tokens.css 的语义层解析成真实色值 ——
 *      Naive 的 changeColor / composite 不接受 `var()`，所以主题层必须是真色值；
 *   2. 明暗两份都从 tokens.css 取（暗色是同一组语义 token 的第二份取值），
 *      由 app.vue 按当前主题选择，不再靠 Naive 的 darkTheme 兜底（否则会闪蓝）；
 *   3. 主色 = 墨色（CTA 是墨水实心胶囊，暗色下反转为浅墨填充），info 位 = 功能蓝；
 *   4. 圆角从 `--radius-sm/md/lg/xl/pill` 取值，不再归零。
 *
 * 纯函数，可单测；不 import Vue / Nuxt。
 */
import type { GlobalThemeOverrides } from "naive-ui";

import { parseTokenBlocks, resolveTokenValue } from "~/utils/markdownTheme";

/** 主题层需要的语义 token（其余中性色交给 Naive 自己的 light/dark 基础主题）。 */
export const READER_THEME_TOKEN_VARS = {
  ink: "--color-ink",
  inkMuted: "--color-ink-muted",
  surface: "--color-surface",
  page: "--color-bg-page",
  link: "--color-link",
  danger: "--color-danger",
  inverse: "--color-on-inverse",
} as const;

export type ReaderThemeTone = keyof typeof READER_THEME_TOKEN_VARS;
export type ReaderThemePalette = Record<ReaderThemeTone, string>;
export type ReaderThemeMode = "light" | "dark";

/** 圆角五档（与 tokens.css 同一份取值）。 */
export interface ReaderThemeScale {
  sm: string;
  md: string;
  lg: string;
  xl: string;
  pill: string;
}

const RADIUS_VARS: Record<keyof ReaderThemeScale, string> = {
  sm: "--radius-sm",
  md: "--radius-md",
  lg: "--radius-lg",
  xl: "--radius-xl",
  pill: "--radius-pill",
};

/** 英文正文字体 Inter + 中文自托管字体 + 系统中文 fallback（顺序有意义）。 */
export const READER_FONT_STACK =
  '"Inter", "Inter Fallback", "Noto Sans SC", "Noto Sans SC Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

/**
 * 从 tokens.css 源码解析主题层色值；缺 token 直接抛错（不静默回落 Naive 默认青绿）。
 *
 * @param mode 明/暗：暗色读 `.dark` 块里的第二份取值。
 */
export function parseReaderThemePalette(
  cssText: string,
  mode: ReaderThemeMode = "light",
): ReaderThemePalette {
  const blocks = parseTokenBlocks(cssText);
  const palette = {} as ReaderThemePalette;

  for (const [tone, tokenVar] of Object.entries(READER_THEME_TOKEN_VARS)) {
    try {
      palette[tone as ReaderThemeTone] = resolveTokenValue(blocks, mode, tokenVar);
    } catch {
      throw new Error(`tokens.css 缺 token ${tokenVar}（Naive 主题需要真实色值）`);
    }
  }

  return palette;
}

/** 从 `:root` 解析五档圆角；缺一档直接抛错（不允许静默归零）。 */
export function parseReaderThemeScale(cssText: string): ReaderThemeScale {
  const root = /:root\s*\{([\s\S]*?)\n\}/u.exec(cssText)?.[1] ?? "";
  const scale = {} as ReaderThemeScale;

  for (const [tier, tokenVar] of Object.entries(RADIUS_VARS)) {
    const match = new RegExp(`${tokenVar}\\s*:\\s*([^;\\n]+?)\\s*;`, "u").exec(root);
    if (!match) {
      throw new Error(`tokens.css 缺圆角 token ${tokenVar}（Naive 主题需要真实圆角值）`);
    }
    scale[tier as keyof ReaderThemeScale] = match[1]!.trim();
  }

  return scale;
}

/**
 * 生成读者站的 Naive 覆盖：主色墨色（CTA 实心胶囊）、info 功能蓝、
 * 圆角从阶梯取、字体栈含中文 fallback。
 */
export function buildReaderThemeOverrides(
  palette: ReaderThemePalette,
  scale: ReaderThemeScale,
): GlobalThemeOverrides {
  return {
    common: {
      primaryColor: palette.ink,
      primaryColorHover: palette.inkMuted,
      primaryColorPressed: palette.ink,
      primaryColorSuppl: palette.inkMuted,
      infoColor: palette.link,
      // info 的悬停/按下不能派生蓝色填充（功能蓝只允许出现在文字链接与焦点环）。
      infoColorHover: palette.ink,
      infoColorPressed: palette.inkMuted,
      errorColor: palette.danger,
      baseColor: palette.surface,
      borderRadius: scale.md,
      borderRadiusSmall: scale.sm,
      fontFamily: READER_FONT_STACK,
      fontFamilyMono: '"Cascadia Code", "JetBrains Mono", "Sofia Sans Mono", ui-monospace, monospace',
    },
    Card: { borderRadius: scale.lg },
    Button: {
      borderRadiusTiny: scale.sm,
      borderRadiusSmall: scale.sm,
      borderRadiusMedium: scale.md,
      borderRadiusLarge: scale.md,
    },
    Input: { borderRadius: scale.md },
    Select: { peers: { InternalSelection: { borderRadius: scale.md } } },
    Modal: { borderRadius: scale.xl },
    Dialog: { borderRadius: scale.xl },
    Drawer: { borderRadius: scale.xl },
    Popover: { borderRadius: scale.xl },
  };
}
