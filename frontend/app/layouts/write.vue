<script setup lang="ts">
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
</script>

<template>
  <div class="min-h-screen bg-page text-ink">
    <slot />
    <DeferredAuthModal />
  </div>
</template>
