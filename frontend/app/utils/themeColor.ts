import type { ThemeMode } from "~/types/api";
import { parseTokenBlocks, resolveTokenValue } from "~/utils/markdownTheme";

/**
 * 移动端状态栏 / 浏览器界面颜色（`theme-color`）。
 *
 * 取值**不在这里定义**：亮暗两档都从 `tokens.css` 的 `--color-bg-page` 解析，
 * 与页面底是同一个来源（spec 票 423 第 3 批第 53 条）。所以改页面底时状态栏自动
 * 跟着变，不会留下第二份手抄的色值。
 *
 * 为什么 meta 带 `tagPriority: -1`：unhead 给内联同步脚本的权重是 50、普通 meta
 * 是 100 —— 默认顺序下首屏脚本（buildThemeInitScript）会跑在 theme-color 之前，
 * 那时 meta 还不存在，就没法在水合前把它改成"没有 cookie、系统是暗色"的最终值。
 * 负数优先级把它排到脚本之前，脚本原地改写即可，不制造第二个 meta。
 */

export const THEME_COLOR_META_NAME = "theme-color";

/** 与 `--color-bg-page` 同源：状态栏颜色的唯一 token 名。 */
export const THEME_COLOR_TOKEN = "--color-bg-page";

export interface ThemeColorPalette {
  light: string;
  dark: string;
}

export interface ThemeColorMeta {
  name: string;
  content: string;
  tagPriority: number;
}

/** 从 tokens.css 源码解析亮/暗两档页面底；缺 token 直接抛错（不静默回落）。 */
export function parseThemeColorPalette(cssText: string): ThemeColorPalette {
  const blocks = parseTokenBlocks(cssText);

  return {
    light: resolveTokenValue(blocks, "light", THEME_COLOR_TOKEN),
    dark: resolveTokenValue(blocks, "dark", THEME_COLOR_TOKEN),
  };
}

export function themeColorFor(theme: ThemeMode, palette: ThemeColorPalette): string {
  return theme === "dark" ? palette.dark : palette.light;
}

/** head 里的 theme-color 条目：随主题取值，并排在首屏同步脚本之前。 */
export function buildThemeColorMeta(theme: ThemeMode, palette: ThemeColorPalette): ThemeColorMeta {
  return {
    name: THEME_COLOR_META_NAME,
    content: themeColorFor(theme, palette),
    tagPriority: -1,
  };
}
