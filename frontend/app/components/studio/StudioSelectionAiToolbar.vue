<script setup lang="ts">
import type { StudioAiAction } from "~/composables/useStudioAiSelection";

defineProps<{
  show: boolean;
  top: number;
  left: number;
}>();

const emit = defineEmits<{
  action: [value: StudioAiAction];
}>();

const actions: Array<{ key: StudioAiAction; label: string }> = [
  { key: "polish", label: "润色改写" },
  { key: "grammar_fix", label: "语法纠错" },
  { key: "style_transform", label: "风格转换" },
  { key: "diagnose", label: "内容诊断" },
];
</script>

<template>
  <div
    v-show="show"
    class="studio-selection-ai-toolbar"
    :style="{ top: `${top}px`, left: `${left}px` }"
    @mousedown.prevent>
    <button
      v-for="item in actions"
      :key="item.key"
      type="button"
      class="studio-selection-ai-toolbar__button"
      @click="emit('action', item.key)">
      {{ item.label }}
    </button>
  </div>
</template>

<style scoped>
/* 颜色一律消费语义 token（票 #9 把创作区从旧青绿色板收敛到新体系）。 */
.studio-selection-ai-toolbar {
  position: fixed;
  z-index: 34;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  transform: translateX(-50%);
  padding: 8px;
  border-radius: 0;
  border: 1px solid var(--color-line);
  background: var(--color-surface);
  backdrop-filter: blur(18px);
  box-shadow: 0 18px 42px var(--color-shadow);
}

.studio-selection-ai-toolbar__button {
  border: 0;
  border-radius: 0;
  padding: 9px 12px;
  background: var(--color-surface-raised);
  color: var(--color-ink-muted);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition:
    background var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.studio-selection-ai-toolbar__button:hover {
  background: var(--color-line-faint);
  color: var(--color-focus);
  transform: translateY(-1px);
}

.studio-selection-ai-toolbar__button:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 1px;
}

@media (max-width: 820px) {
  .studio-selection-ai-toolbar {
    max-width: calc(100vw - 20px);
    flex-wrap: wrap;
    justify-content: center;
  }
}
</style>
