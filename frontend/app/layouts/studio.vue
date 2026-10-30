<script setup lang="ts">
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
import StudioSidebarNav from "~/components/studio/StudioSidebarNav.vue";
import { useSectionReveal } from "~/composables/useSectionReveal";

const siteStore = useSiteStore();
const authStore = useAuthStore();
const messageStore = useMessageStore();

// 创作区也接区块级滚动进场（整页内容作为一个布局级容器，进入即淡入，只进一次）。
const { target: revealTarget, revealStyle } = useSectionReveal();

void siteStore.ensurePublicBootstrap();

if (authStore.isLoggedIn && !messageStore.fetched) {
  void messageStore.refreshSummary().catch(() => undefined);
}
</script>

<template>
  <div class="studio-layout-page min-h-screen">
    <AppHeader />
    <main class="page-shell">
      <div class="studio-layout">
        <StudioSidebarNav />
        <div ref="revealTarget" class="min-w-0 space-y-5 motion-reveal" :style="revealStyle">
          <slot />
        </div>
      </div>
    </main>
    <DeferredAuthModal />
  </div>
</template>

<style scoped>
/* 页头是吸顶玻璃导航：锚点跳转与页内滚动都要给页头留位（ADR-0012 约束 8）。 */
.studio-layout-page {
  scroll-padding-top: calc(var(--site-header-height) + 0.5rem);
}
</style>
