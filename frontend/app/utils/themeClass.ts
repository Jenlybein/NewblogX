import type { ThemeMode } from "~/types/api";

/**
 * 站点根类：`.theme-dark` / `.theme-light`。
 *
 * `.theme-*` 是重构前就有的 CSS 覆盖层选择器（tokens.css 里语义 token 的第二份
 * 取值也挂在它上面）；Twind/Tailwind 不认它，所以它只负责"旧样式跟着变"。
 */
export function themeRootClass(theme: ThemeMode): "theme-dark" | "theme-light" {
  return theme === "dark" ? "theme-dark" : "theme-light";
}

/**
 * body 上的类。**必须同时带 `dark`**：Tailwind 配置里是 `darkMode: 'class'`，
 * `dark:` 工具类只认 `.dark` 祖先。历史上 body 只有 `theme-dark`、Tailwind 走默认
 * `media` 策略，于是站内明暗开关管不住任何 `dark:` 工具类（只有操作系统能）。
 */
export function themeBodyClass(theme: ThemeMode): string {
  const rootClass = themeRootClass(theme);
  return theme === "dark" ? `${rootClass} dark` : rootClass;
}
