<script setup lang="ts">
defineProps<{
  title: string;
  description: string;
}>();
</script>

<template>
  <div class="surface-section studio-empty-state">
    <div class="studio-empty-state__icon">BX</div>
    <h2 class="section-title">{{ title }}</h2>
    <p class="muted max-w-xl text-center leading-7">{{ description }}</p>
    <slot />
  </div>
</template>
