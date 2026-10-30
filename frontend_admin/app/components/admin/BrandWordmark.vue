<script setup lang="ts">
import { computed } from "vue";

import { BRAND } from "~/utils/brand";

const props = withDefaults(
  defineProps<{
    size?: "sm" | "md" | "lg";
    layout?: "inline" | "stack";
  }>(),
  { size: "md", layout: "inline" },
);

const SIZE_CLASSES = {
  sm: { primary: "text-base", secondary: "text-[10px]" },
  md: { primary: "text-xl", secondary: "text-xs" },
  lg: { primary: "text-3xl", secondary: "text-sm" },
} as const;

const layoutClass = computed(() =>
  props.layout === "stack" ? "flex-col items-start gap-0.5" : "items-baseline gap-2",
);
const primaryClass = computed(() => [
  SIZE_CLASSES[props.size].primary,
  "font-display leading-none tracking-title text-ink",
]);
const secondaryClass = computed(() => [
  SIZE_CLASSES[props.size].secondary,
  "font-cjk font-semibold leading-none text-ink-muted",
]);
</script>

<template>
  <span class="admin-brand-wordmark inline-flex" :class="layoutClass">
    <span :class="primaryClass">{{ BRAND.wordmark.primary }}</span>
    <span :class="secondaryClass">{{ BRAND.wordmark.secondary }}</span>
  </span>
</template>
