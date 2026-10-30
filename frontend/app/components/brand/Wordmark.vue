<script setup lang="ts">
import { computed } from "vue";

import { BRAND } from "~/utils/brand";

const props = withDefaults(
  defineProps<{
    size?: "sm" | "md" | "lg" | "xl";
    tone?: "default" | "invert";
    layout?: "inline" | "stack";
  }>(),
  { size: "md", tone: "default", layout: "inline" },
);

const SIZE_CLASSES = {
  sm: { primary: "text-base", secondary: "text-[10px]" },
  md: { primary: "text-xl", secondary: "text-xs" },
  lg: { primary: "text-2xl", secondary: "text-sm" },
  xl: { primary: "text-4xl", secondary: "text-base" },
} as const;

const layoutClass = computed(() =>
  props.layout === "stack" ? "flex-col items-start gap-0.5" : "items-baseline gap-2",
);
const primaryClass = computed(() => [
  SIZE_CLASSES[props.size].primary,
  "font-display leading-none tracking-title",
  props.tone === "invert" ? "text-jelly-cream" : "text-ink",
]);
const secondaryClass = computed(() => [
  SIZE_CLASSES[props.size].secondary,
  "font-cjk font-semibold leading-none",
  props.tone === "invert" ? "text-jelly-cream opacity-80" : "text-ink-muted",
]);
</script>

<template>
  <span class="brand-wordmark inline-flex" :class="layoutClass">
    <span :class="primaryClass">{{ BRAND.wordmark.primary }}</span>
    <span :class="secondaryClass">{{ BRAND.wordmark.secondary }}</span>
  </span>
</template>
