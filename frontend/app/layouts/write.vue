<script setup lang="ts">
import DeferredAuthModal from "~/components/auth/DeferredAuthModal.vue";
</script>

<template>
  <div class="min-h-screen bg-[linear-gradient(180deg,#fefcf8_0%,#f7fbff_100%)] text-slate-900">
    <slot />
    <DeferredAuthModal />
  </div>
</template>
