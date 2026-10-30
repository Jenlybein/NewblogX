<script setup lang="ts">
import AppFooter from "~/components/layout/AppFooter.vue";
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
import JellyLauncher from "~/components/jelly/JellyLauncher.vue";

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
  <div class="min-h-screen">
    <AppHeader />
    <main class="page-shell">
      <slot />
    </main>
    <AppFooter />
    <DeferredAuthModal />
    <!-- 读者站全站常驻「捏捏」。ClientOnly：位置依赖 window（SSR 不渲染，避免 ES 端算错位置）；
         创作区用的是 studio / write 布局，本来就不带这一块，组件内还有 /studio 路由守卫兜底。 -->
    <ClientOnly>
      <JellyLauncher />
    </ClientOnly>
  </div>
</template>
