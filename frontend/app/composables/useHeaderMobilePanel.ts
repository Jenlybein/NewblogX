import { onBeforeUnmount, onMounted, ref } from "vue";

/**
 * 手机断点（页头折叠面板）的视口判定（票 437）。
 *
 * 为什么需要 JS 而不是纯 CSS：面板收起后要挂 `inert`（容器必须常驻 DOM 才能做
 * 轨道过渡），而 `inert` 是属性、不能由媒体查询开关。它又**只能**在手机断点挂：
 * 桌面断点下面板一直可见，挂上会把整条导航锁死。
 *
 * SSR 默认 false（桌面口径）：服务端没有视口，宁可先渲染成"可聚焦"，也不要让
 * 桌面用户在水合前点不动导航；手机端在水合阶段立刻按真实视口补上 inert。
 *
 * 查询字符串与 `AppHeader.vue` 里 `.site-header__panel` 的折叠断点同源（640px）；
 * 改断点必须两处同改。
 */
export const MOBILE_PANEL_QUERY = "(max-width: 639px)";

function initialMatches(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  return Boolean(window.matchMedia(MOBILE_PANEL_QUERY).matches);
}

export function useHeaderMobilePanel() {
  const isMobile = ref(initialMatches());
  let mobileQuery: MediaQueryList | null = null;

  function sync(event?: MediaQueryListEvent) {
    isMobile.value = event ? event.matches : Boolean(mobileQuery?.matches);
  }

  onMounted(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    // setup 阶段已读过一次（水合期就用真实视口）；这里只负责后续的断点变化。
    mobileQuery = window.matchMedia(MOBILE_PANEL_QUERY);
    sync();
    mobileQuery.addEventListener("change", sync);
  });

  onBeforeUnmount(() => {
    mobileQuery?.removeEventListener("change", sync);
  });

  return { isMobile };
}
