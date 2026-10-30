import { computed, onScopeDispose, ref, shallowRef } from "vue";
import type { ThemeMode } from "~/types/api";
import { THEME_COOKIE_NAME, THEME_MEDIA_QUERY, resolveThemeMode } from "~/utils/themePreference";

export const useUiStore = defineStore("ui", () => {
  /**
   * cookie 的默认值**必须是 null（"没选过"）**。
   *
   * 历史上默认写成 "light"，于是永远分不清"用户选了浅色"与"从没选过"，
   * 首次访问也就无法跟随系统偏好（issue #12 的第一条验收）。
   * 名字沿用 `blogx-theme`：改名会让老用户已保存的选择作废。
   */
  const themeCookie = useCookie<ThemeMode | null>(THEME_COOKIE_NAME, {
    default: () => null,
  });

  const authModalOpen = shallowRef(false);

  /** 系统偏好只在浏览器里可读；SSR 时保持 false，最终值由首屏脚本/水合后修正。 */
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
   * 只读：写入口是 `setTheme` / `toggleTheme`（保证"此后以用户选择为准"）。
   */
  const theme = computed<ThemeMode>(() =>
    resolveThemeMode({ cookie: themeCookie.value, systemPrefersDark: systemPrefersDark.value }),
  );

  function setTheme(next: ThemeMode) {
    themeCookie.value = next;
  }

  function openAuthModal() {
    authModalOpen.value = true;
  }

  function closeAuthModal() {
    authModalOpen.value = false;
  }

  function toggleTheme() {
    setTheme(theme.value === "dark" ? "light" : "dark");
  }

  return {
    authModalOpen,
    theme,
    setTheme,
    openAuthModal,
    closeAuthModal,
    toggleTheme,
  };
});
