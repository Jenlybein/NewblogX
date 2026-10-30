<script setup lang="ts">
import AppFooter from "~/components/layout/AppFooter.vue";
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
import JellyLauncher from "~/components/jelly/JellyLauncher.vue";
import JellyAiPanel from "~/components/jelly/JellyAiPanel.vue";
import { provideJellyAssistant } from "~/composables/useJellyAssistant";
import { isPlaceholderSiteIcon, siteIconType } from "~/utils/siteIcon";

const siteStore = useSiteStore();

// 捏捏的状态实例在页面级建一次并 provide：读者站的角色与它张开的 AI 面板（#10）
// 必须共享同一份状态（谁也别自己 new）。
const jellyAssistant = provideJellyAssistant();

void siteStore.ensurePublicBootstrap();

// 图标：品牌图标已由 `nuxt.config` 的 `app.head` 统一发出（品牌 SVG + .ico 兜底 +
// apple-touch-icon）。后端 `/site/seo` 的 `icon` 只是可覆盖入口，默认值是占位串
// `/xxx.png`——直接照发会让 favicon 404 并盖掉品牌图标（线上实测过），所以这里：
// 占位/空值一律不发（交给 app.head），真实值才作为管理员覆盖发出。
useHead(() => {
  const raw = siteStore.seo?.icon ?? "";
  if (isPlaceholderSiteIcon(raw)) return {};
  const href = raw.trim();
  const type = siteIconType(href);
  return type ? { link: [{ rel: "icon", type, href }] } : { link: [{ rel: "icon", href }] };
});
</script>

<template>
  <div class="site-layout">
    <AppHeader />
    <main id="main-content" class="site-layout__main page-shell" tabindex="-1">
      <slot />
    </main>
    <AppFooter />
    <DeferredAuthModal />
    <!-- 读者站全站常驻「捏捏」与它张开的 AI 面板。ClientOnly：位置/视口依赖 window，
         AnimatePresence 的退场也只在客户端有意义（SSR 不渲染，避免 initial 内联藏内容）；
         创作区用的是 studio / write 布局，本来就不带这一块，组件内还有 /studio 路由守卫兜底。 -->
    <ClientOnly>
      <JellyLauncher :assistant="jellyAssistant" />
      <JellyAiPanel :assistant="jellyAssistant" :ai-info="siteStore.aiInfo" />
    </ClientOnly>
  </div>
</template>

<style scoped>
/*
 * 布局 = 页头 / 正文 / 页脚三段的纵向流。`site-layout__main` flex:1 让短内容页也把
 * 页脚压到底部（页脚是"滚动到底的结束锚点"）；页头是吸顶玻璃导航（ADR-0012 决策 6），
 * 所以布局要补 scroll-padding-top：页内滚动容器跳锚点时不被页头盖住。
 * tabindex="-1" 让页头的 skip link 能把焦点真正移到正文。
 */
.site-layout {
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  /* 首屏封面的 100vw bleed 会在桌面多出 1px 级横向溢出；在视口级容器统一兜住（ADR-0013 约束 3）。
     用 clip 而不是 hidden：clip 不创建滚动容器，吸顶页头与锚点补偿都不受影响。 */
  overflow-x: clip;
  /* 整页滚动由 html 的 scroll-padding-top 兜底；这里给未来的页内滚动容器同一份安全区。 */
  scroll-padding-top: calc(var(--site-header-height) + 0.5rem);
}

.site-layout__main {
  flex: 1 1 auto;
}

.site-layout__main:focus {
  outline: none;
}
</style>
