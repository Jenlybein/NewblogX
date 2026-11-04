import { computed, h, onBeforeUnmount, onMounted, ref } from "vue";
import {
  IconClockHour4,
  IconHome2,
  IconLogin2,
  IconUserCircle,
} from "@tabler/icons-vue";
import { navigateTo } from "#imports";
import { useAuthStore } from "~/stores/auth";

/**
 * 页头账号菜单（含「浏览历史」的三档落位）。
 *
 * 为什么是 matchMedia 而不是 CSS：菜单项是数据驱动的（`NDropdown` 的 `options`），
 * 服务端没有视口、CSS 也管不到「往 options 里追加一项」。所以三段断点里
 * **只有 640–1279 这一档需要 JS**：
 *   ≥1280 动作区普通链接常显（CSS）；
 *   640–1279 收进这张菜单（本文件）；
 *   < 640 收进折叠面板（CSS）。
 *
 * 抽成 composable 而不是留在 `AppHeader.vue`：页头已连续两张票加入口，
 * 单文件撞上 800 行上限，外移比调高上限更合适。
 */
export function useHeaderUserMenu() {
  const authStore = useAuthStore();

  const HISTORY_MENU_QUERY = "(min-width: 640px) and (max-width: 1279px)";
  const historyMenuVisible = ref(false);
  let historyMenuQuery: MediaQueryList | null = null;

  function syncHistoryMenu(event?: MediaQueryListEvent) {
    historyMenuVisible.value = event ? event.matches : Boolean(historyMenuQuery?.matches);
  }

  onMounted(() => {
    // 视口只在客户端有意义（服务端渲染时不注册监听）。
    historyMenuQuery = window.matchMedia(HISTORY_MENU_QUERY);
    syncHistoryMenu();
    historyMenuQuery.addEventListener("change", syncHistoryMenu);
  });

  onBeforeUnmount(() => {
    historyMenuQuery?.removeEventListener("change", syncHistoryMenu);
  });

  const userMenuOptions = computed(() => {
    const options = [
      {
        label: "个人中心",
        key: "studio",
        icon: () => h(IconHome2, { size: 16 }),
      },
      {
        label: "我的主页",
        key: "profile",
        icon: () => h(IconUserCircle, { size: 16 }),
      },
    ];
    // 640–1279px 时「历史」入口收在这里（票 241 的三档落位）。
    if (historyMenuVisible.value) {
      options.push({
        label: "浏览历史",
        key: "history",
        icon: () => h(IconClockHour4, { size: 16 }),
      });
    }
    options.push({
      label: "退出登录",
      key: "logout",
      icon: () => h(IconLogin2, { size: 16 }),
    });
    return options;
  });

  async function handleUserMenuSelect(key: string | number) {
    if (key === "studio") {
      await navigateTo("/studio/dashboard");
      return;
    }

    if (key === "profile" && authStore.profileId) {
      await navigateTo(`/users/${authStore.profileId}`);
      return;
    }

    if (key === "history") {
      await navigateTo("/history");
      return;
    }

    if (key === "logout") {
      await authStore.logout();
    }
  }

  return { userMenuOptions, handleUserMenuSelect };
}
