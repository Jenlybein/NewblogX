<script setup lang="ts">
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
</script>

<template>
  <div class="write-layout-page min-h-screen bg-page text-ink">
    <!-- 创作页**不**渲染站点页头：这里是"一屏专心写"的工作区，站点导航会分散注意力。
         代价是主题开关不能靠页头 —— 它落在创作页自己顶栏的最右端（StudioWriteHeader）。
         背景与字色走语义 token（不再写死亮色渐变），切暗色时不发白。 -->
    <slot />
    <DeferredAuthModal />
  </div>
</template>
