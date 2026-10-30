<script setup lang="ts">
import AppHeader from "~/components/layout/AppHeader.vue";
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
</script>

<template>
  <div class="min-h-screen bg-page text-ink">
    <!-- 写作页同样渲染 AppHeader：主题开关的唯一落点是它，暗色/亮色都与站内其他页一致。
         背景与字色走语义 token（不再写死亮色渐变），切暗色时不发白。 -->
    <AppHeader />
    <slot />
    <DeferredAuthModal />
  </div>
</template>
