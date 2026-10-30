<script setup lang="ts">
import AppFooter from "~/components/layout/AppFooter.vue";
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";

const siteStore = useSiteStore();

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
