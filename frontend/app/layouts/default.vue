<script setup lang="ts">
import AppFooter from "~/components/layout/AppFooter.vue";
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
import JellyLauncher from "~/components/jelly/JellyLauncher.vue";
import JellyAiPanel from "~/components/jelly/JellyAiPanel.vue";
import { provideJellyAssistant } from "~/composables/useJellyAssistant";

const siteStore = useSiteStore();

// 捏捏的状态实例在页面级建一次并 provide：读者站的角色与它张开的 AI 面板（#10）
// 必须共享同一份状态（谁也别自己 new）。
const jellyAssistant = provideJellyAssistant();

void siteStore.ensurePublicBootstrap();

useHead(() => ({
  link: siteStore.seo?.icon
    ? [
        {
          rel: "icon",
          href: siteStore.seo.icon,
        },
      ]
    : [],
}));
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
 * 页脚压到底部（页脚是"滚动到底的结束锚点"）；页头不是 sticky，滚动不遮正文。
 * tabindex="-1" 让页头的 skip link 能把焦点真正移到正文。
 */
.site-layout {
  display: flex;
  min-height: 100vh;
  flex-direction: column;
}

.site-layout__main {
  flex: 1 1 auto;
}

.site-layout__main:focus {
  outline: none;
}
</style>
