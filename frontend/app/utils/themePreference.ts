import type { ThemeMode } from "~/types/api";

/**
 * 明暗主题的"初始偏好"：系统偏好 vs 用户选择（cookie）的解析。
 *
 * 规则（issue #12）：
 *   1. 用户显式切过 → cookie 说了算（`blogx-theme`，**保留历史名**，改名会让老用户的选择丢失）；
 *   2. 从没切过 → 跟随系统 `prefers-color-scheme`；
 *   3. 系统也读不到（SSR / 老浏览器）→ 兜底浅色。
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
  if (userChoice) {
    return { theme: userChoice, source: "user" };
  }

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
 * 首屏前置脚本：同步执行在 `<head>` 里、早于 body 绘制。
 *
 * 为什么需要它：SSR 只能看到 cookie，看不到系统偏好；没有 cookie 的新访客如果系统是
 * 暗色，服务端会先渲染浅色，客户端水合后才变暗 —— 也就是"闪白"。这段脚本在水合之前
 * 直接把根元素类钉成最终值，颜色由 CSS 变量瞬间生效，不产生闪烁、也不改字号/间距。
 *
 * 只做 class 增删（连同 `dark`，Tailwind `dark:` 工具类才跟着走），不写样式、不抛错。
 */
export function buildThemeInitScript(): string {
  return [
    "(function () {",
    "  try {",
    "    var root = document.documentElement;",
    "    var match = document.cookie.match(/(?:^|;\\s*)" + THEME_COOKIE_NAME + "=(dark|light)/);",
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
