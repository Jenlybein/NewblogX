<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    hints?: string[];
    size?: "sm" | "md" | "lg";
    pending?: boolean;
  }>(),
  {
    description: "",
    hints: () => [],
    size: "md",
    pending: false,
  },
);

// 空态/加载态的**结构**必须固定占位：收藏架的空态如果只渲染一行字，右侧栏会
// 塌成一条，整页分区跟着错位。所以尺寸档位写成显式的 min-height，不靠内容撑高。
const SIZE_CLASS = {
  sm: "min-h-[160px]",
  md: "min-h-[240px]",
  lg: "min-h-[320px]",
} as const;

const sizeClass = computed(() => SIZE_CLASS[props.size]);
</script>

<template>
  <section
    class="empty-state"
    :class="[sizeClass, pending ? 'is-pending' : '']"
    data-empty-state="true"
    :data-state="pending ? 'pending' : 'empty'"
  >
    <div class="empty-state__mark" aria-hidden="true">
      <svg viewBox="0 0 24 24" class="h-5 w-5" focusable="false">
        <polygon points="12,3 21,20 3,20" class="fill-jelly-yellow" />
      </svg>
      <svg viewBox="0 0 24 24" class="h-5 w-5" focusable="false">
        <rect x="5" y="5" width="14" height="14" class="fill-jelly-coral" />
      </svg>
      <svg viewBox="0 0 24 24" class="h-5 w-5" focusable="false">
        <circle cx="12" cy="12" r="7" class="fill-jelly-blue" />
      </svg>
    </div>

    <h3 class="empty-state__title">{{ title }}</h3>
    <p v-if="description" class="empty-state__body">{{ description }}</p>
    <ul v-if="hints.length" class="empty-state__hints">
      <li v-for="hint in hints" :key="hint">{{ hint }}</li>
    </ul>
    <div v-if="$slots.default" class="empty-state__actions blueprint-actions">
      <slot />
    </div>
  </section>
</template>
