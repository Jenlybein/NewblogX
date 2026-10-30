import type { ThemeMode } from "~/utils/theme";

/**
 * 后台明暗主题的"初始偏好"与根类（#22 起后台正式启用暗色）。
 *
 * 规则与读者站 **同一套**（issue #12 定的口径），只是后台此前没有这套机制：
 *   1. 用户显式切过 → cookie 说了算（`blogx-theme`，**沿用读者站的 cookie 名**：
 *      同一台机器上两端的选择应当一致，换名会让"我在前台选了暗色、后台还是亮的"）；
 *   2. 从没切过 → 跟随系统 `prefers-color-scheme`；
 *   3. 系统也读不到（首屏脚本被拦 / 老浏览器）→ 兜底浅色。
 *
 * 这里是纯函数：store / 首屏脚本 / 测试共用同一份判定，避免"SSR 浅色、客户端又自己猜一次"。
 */

export const THEME_COOKIE_NAME = "blogx-theme";
export const THEME_MEDIA_QUERY = "(prefers-color-scheme: dark)";
export const DEFAULT_THEME_MODE: ThemeMode = "light";

export function isThemeMode(value: unknown): value is ThemeMode {
  return value === "light" || value === "dark";
}

/** 解析 cookie：只认 `light` / `dark`，其余（含未设置、被篡改的值）一律当"没有选择"。 */
export function parseThemeCookie(value: unknown): ThemeMode | null {
  return isThemeMode(value) ? value : null;
}

export interface ThemePreferenceInput {
  cookie?: unknown;
  systemPrefersDark?: boolean;
}

export interface ResolvedThemePreference {
  theme: ThemeMode;
  /** 这次取值是哪来的：用户选择 / 系统偏好 / 兜底。 */
  source: "user" | "system" | "default";
}

export function resolveThemePreference(input: ThemePreferenceInput = {}): ResolvedThemePreference {
  const userChoice = parseThemeCookie(input.cookie);
  if (userChoice) return { theme: userChoice, source: "user" };

  if (typeof input.systemPrefersDark === "boolean") {
    return { theme: input.systemPrefersDark ? "dark" : "light", source: "system" };
  }

  return { theme: DEFAULT_THEME_MODE, source: "default" };
}

/** 取最终主题（store 与首屏脚本都只需要这个）。 */
export function resolveThemeMode(input: ThemePreferenceInput = {}): ThemeMode {
  return resolveThemePreference(input).theme;
}

/**
 * 站点根类：`.theme-dark` / `.theme-light`。
 *
 * `.theme-*` 是 `tokens.css` 里语义 token 第二份取值的选择器；`dark` 是 Tailwind
 * `darkMode: "class"` 认的类。两者必须成对出现，否则"站点开关"与"`dark:` 工具类"
 * 会各自为政（读者站历史上踩过这个坑）。
 */
export function themeRootClass(theme: ThemeMode): "theme-dark" | "theme-light" {
  return theme === "dark" ? "theme-dark" : "theme-light";
}

export function themeBodyClass(theme: ThemeMode): string {
  const rootClass = themeRootClass(theme);

  return theme === "dark" ? `${rootClass} dark` : rootClass;
}

/** html 上的类：与 body 同一组（首屏脚本在水合前先钉一次）。 */
export function themeHtmlClass(theme: ThemeMode): string {
  return themeBodyClass(theme);
}

/**
 * 首屏前置脚本：同步执行在 `<head>` 里、早于 body 绘制。
 *
 * 为什么需要它：SSR 只能看到 cookie，看不到系统偏好；没 cookie 的新访客如果系统是
 * 暗色，服务端会先渲染浅色、客户端水合后才变暗 —— 也就是"闪白"。这段脚本在水合
 * 之前直接把根元素类钉成最终值，颜色由 CSS 变量瞬间生效，不产生闪烁、也不改字号/间距。
 *
 * 只做 class 增删（连同 `dark`），不写样式、不抛错。
 */
export function buildThemeInitScript(): string {
  return [
    "(function () {",
    "  try {",
    "    var root = document.documentElement;",
    '    var match = document.cookie.match(/(?:^|;\\s*)' + THEME_COOKIE_NAME + "=(dark|light)/);",
    '    var theme = match ? match[1] : (window.matchMedia && window.matchMedia("' +
      THEME_MEDIA_QUERY +
      '").matches ? "dark" : "light");',
    '    root.classList.remove("theme-light", "theme-dark", "dark");',
    '    root.classList.add(theme === "dark" ? "theme-dark" : "theme-light");',
    '    if (theme === "dark") { root.classList.add("dark"); }',
    "  } catch (error) {",
    "    /* 首屏脚本失败也不拦渲染，退化为默认浅色 */",
    "  }",
    "})();",
  ].join("");
}
