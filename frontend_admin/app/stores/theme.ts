import { computed, onScopeDispose, ref } from "vue";

import type { ThemeMode } from "~/utils/theme";
import { THEME_COOKIE_NAME, THEME_MEDIA_QUERY, resolveThemeMode } from "~/utils/themePreference";

/**
 * 后台明暗主题状态（#22 起后台正式启用暗色）。
 *
 * 与读者站 `stores/ui.ts` 同一套判定（用户 cookie > 系统偏好 > 兜底浅色），
 * 只是后台此前完全没有这个 store：暗色映射写好了，却没有任何入口能把它打开。
 *
 * cookie 的默认值**必须是 null（"没选过"）**：写成 "light" 就永远分不清
 * "用户选了浅色"与"从没选过"，首次访问也就无法跟随系统偏好。
 */
export const useThemeStore = defineStore("admin-theme", () => {
  const themeCookie = useCookie<ThemeMode | null>(THEME_COOKIE_NAME, {
    default: () => null,
  });

  /** 系统偏好只在浏览器里可读；SSR / 首屏脚本被拦时保持 false，最终值由首屏脚本修正。 */
  const systemPrefersDark = ref(false);

  if (import.meta.client && typeof window !== "undefined" && typeof window.matchMedia === "function") {
    const query = window.matchMedia(THEME_MEDIA_QUERY);
    systemPrefersDark.value = query.matches;
    const onSystemChange = (event: MediaQueryListEvent) => {
      systemPrefersDark.value = event.matches;
    };
    query.addEventListener("change", onSystemChange);
    onScopeDispose(() => query.removeEventListener("change", onSystemChange));
  }

  /**
   * 最终主题：用户选择（cookie）优先；没有选择时跟随系统；两者都没有才兜底浅色。
   * 只读：写入口是 `setTheme` / `toggleTheme`。
   */
  const theme = computed<ThemeMode>(() =>
    resolveThemeMode({ cookie: themeCookie.value, systemPrefersDark: systemPrefersDark.value }),
  );

  function setTheme(next: ThemeMode) {
    themeCookie.value = next;
  }

  function toggleTheme() {
    setTheme(theme.value === "dark" ? "light" : "dark");
  }

  return { theme, setTheme, toggleTheme };
});
