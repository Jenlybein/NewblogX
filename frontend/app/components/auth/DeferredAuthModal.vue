<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";

const uiStore = useUiStore();
const AuthModal = defineAsyncComponent(() => import("~/components/auth/AuthModal.vue"));
const shouldRender = computed(() => uiStore.authModalOpen);
</script>

<template>
  <ClientOnly>
    <AuthModal v-if="shouldRender" />
  </ClientOnly>
</template>
