<script setup lang="ts">
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
import StudioSidebarNav from "~/components/studio/StudioSidebarNav.vue";

const siteStore = useSiteStore();
const authStore = useAuthStore();
const messageStore = useMessageStore();

void siteStore.ensurePublicBootstrap();

if (authStore.isLoggedIn && !messageStore.fetched) {
  void messageStore.refreshSummary().catch(() => undefined);
}
</script>

<template>
  <div class="min-h-screen">
    <AppHeader />
    <main class="page-shell">
      <div class="studio-layout">
        <StudioSidebarNav />
        <div class="min-w-0 space-y-5">
          <slot />
        </div>
      </div>
    </main>
    <DeferredAuthModal />
  </div>
</template>
