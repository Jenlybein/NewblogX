<script setup lang="ts">
import { computed } from "vue";

import { BRAND } from "~/utils/brand";

const props = withDefaults(
  defineProps<{
    size?: "sm" | "md" | "lg";
    layout?: "inline" | "stack";
    /**
     * `page`：放在页面/顶栏（浅底深字、暗底浅字，随主题翻转）。
     * `inverse`：放在**固定的深墨面**上（登录 hero、封面兜底），两态都要浅字。
     * 登录 hero 以前误用 `page`，结果亮色下深字压深底、暗色下浅字压浅底，字标几乎看不见。
     */
    tone?: "page" | "inverse";
  }>(),
  { size: "md", layout: "inline", tone: "page" },
);

const SIZE_CLASSES = {
  sm: { primary: "text-base", secondary: "text-[10px]" },
  md: { primary: "text-xl", secondary: "text-xs" },
  lg: { primary: "text-3xl", secondary: "text-sm" },
} as const;

const layoutClass = computed(() =>
  props.layout === "stack" ? "flex-col items-start gap-0.5" : "items-baseline gap-2",
);
// 两种 tone 都走 Tailwind 语义色类（同一层叠层），不写 !important、不靠层叠顺序赌博。
const toneClass = computed(() =>
  props.tone === "inverse"
    ? { primary: "text-inverse", secondary: "text-inverse-muted" }
    : { primary: "text-ink", secondary: "text-ink-muted" },
);
const primaryClass = computed(() => [
  SIZE_CLASSES[props.size].primary,
  "font-display leading-none tracking-title",
  toneClass.value.primary,
]);
const secondaryClass = computed(() => [
  SIZE_CLASSES[props.size].secondary,
  "font-cjk font-semibold leading-none",
  toneClass.value.secondary,
]);
</script>

<template>
  <span class="admin-brand-wordmark inline-flex" :class="layoutClass">
    <span :class="primaryClass">{{ BRAND.wordmark.primary }}</span>
    <span :class="secondaryClass">{{ BRAND.wordmark.secondary }}</span>
  </span>
</template>
