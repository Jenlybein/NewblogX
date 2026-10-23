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
.studio-selection-ai-toolbar {
  position: fixed;
  z-index: 34;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  transform: translateX(-50%);
  padding: 8px;
  border-radius: 18px;
  border: 1px solid rgba(204, 215, 230, 0.95);
  background: rgba(255, 252, 247, 0.96);
  backdrop-filter: blur(18px);
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.18);
}

.studio-selection-ai-toolbar__button {
  border: 0;
  border-radius: 12px;
  padding: 9px 12px;
  background: rgba(241, 247, 246, 0.9);
  color: #365466;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition:
    background 0.18s ease,
    color 0.18s ease,
    transform 0.18s ease;
}

.studio-selection-ai-toolbar__button:hover {
  background: rgba(15, 118, 110, 0.14);
  color: #0f766e;
  transform: translateY(-1px);
}

@media (max-width: 820px) {
  .studio-selection-ai-toolbar {
    max-width: calc(100vw - 20px);
    flex-wrap: wrap;
    justify-content: center;
  }
}
</style>
