import { onBeforeUnmount, onMounted, ref } from "vue";

import { MOBILE_PANEL_QUERY } from "~/utils/breakpoints";

/**
 * 手机断点（页头折叠面板）的视口判定（票 437）。
 *
 * 为什么需要 JS 而不是纯 CSS：面板收起后要挂 `inert`（容器必须常驻 DOM 才能做
 * 轨道过渡），而 `inert` 是属性、不能由媒体查询开关。它又**只能**在手机断点挂：
 * 桌面断点下面板一直可见，挂上会把整条导航锁死。
 *
 * **水合安全**（票 437 评审）：`isMobile` 在 setup 阶段恒为 `false`（服务端按桌面
 * 口径渲染，水合首帧与 SSR 完全一致），真实视口值在 `onMounted` 之后才补上。
 * 以前在 setup 里读 `matchMedia`，客户端首次渲染就是 `true`，SSR 却是 `false`，
 * `inert` 属性在挂载瞬间跳变 —— 正是 Vue 水合不一致的典型来源。
 * 查询串与 `AppHeader.vue` 里 `.site-header__panel` 的折叠断点同源
 * （`app/utils/breakpoints.ts` 的 `TABLET_BREAKPOINT_PX`），由
 * `test/design/header-breakpoints.test.ts` 断言两处一致。
 */
export function useHeaderMobilePanel() {
  // 恒 false 起步：服务端 / 水合首帧按桌面口径（面板可见、不挂 inert）。
  const isMobile = ref(false);
  let mobileQuery: MediaQueryList | null = null;

  function sync(event?: MediaQueryListEvent) {
    isMobile.value = event ? event.matches : Boolean(mobileQuery?.matches);
  }

  onMounted(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    // 挂载后才按真实视口补：这一帧是客户端自己的更新，不会参与水合比对。
    mobileQuery = window.matchMedia(MOBILE_PANEL_QUERY);
    sync();
    mobileQuery.addEventListener("change", sync);
  });

  onBeforeUnmount(() => {
    mobileQuery?.removeEventListener("change", sync);
  });

  return { isMobile };
}
