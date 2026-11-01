<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    /** 骨架条数（正文行或卡片块） */
    rows?: number;
    /** text = 正文行；card = 列表 / 侧栏卡片块 */
    variant?: "text" | "card";
    /** 可选的读屏加载说明（骨架条本身只是装饰，对读屏隐藏） */
    label?: string;
  }>(),
  { rows: 3, variant: "text", label: "" },
);

const barClass = computed(() => `skeleton-bar--${props.variant}`);
</script>

<template>
  <div class="skeleton" data-role="skeleton">
    <!--
      骨架 = "数据还没到"那一瞬间的最小占位（ADR-0019 决策 3）。
      只吃语义 token（发丝线色），**不做动画**：它不是装饰动效，数据到了就该立刻被真内容替换。
    -->
    <p v-if="label" class="sr-only">{{ label }}</p>
    <div class="skeleton__bars" aria-hidden="true">
      <span
        v-for="row in rows"
        :key="row"
        class="skeleton-bar"
        :class="barClass"
        data-role="skeleton-bar"
      />
    </div>
  </div>
</template>

<style scoped>
.skeleton__bars {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.skeleton-bar {
  display: block;
  border-radius: var(--radius-sm);
  background: var(--color-line-faint);
}

.skeleton-bar--text {
  height: 0.75rem;
}

/* 最后一行短一点：读起来像"一段文字被截断"，而不是一排等长色块。 */
.skeleton-bar--text:last-child {
  width: 60%;
}

.skeleton-bar--card {
  height: 4.5rem;
  border-radius: var(--radius-lg);
}
</style>
