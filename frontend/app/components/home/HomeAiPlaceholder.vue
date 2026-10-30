<script setup lang="ts">
import { computed } from "vue";
import type { SiteAiInfo } from "~/types/api";
import { homeAiPlaceholder } from "~/utils/home";

const props = withDefaults(defineProps<{ aiInfo?: SiteAiInfo | null }>(), { aiInfo: null });
const emit = defineEmits<{ open: [] }>();

const copy = computed(() => homeAiPlaceholder(props.aiInfo));
</script>

<template>
  <section class="blueprint-panel" data-home-section="ai-placeholder">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="eyebrow">AI</div>
      <span class="blueprint-chip" data-role="ai-status">{{ copy.available ? "占位入口" : "未启用" }}</span>
    </div>

    <h3 class="mt-2 font-title text-lg text-ink">{{ copy.name }}</h3>
    <p class="mt-2 text-sm leading-6 text-ink-muted">{{ copy.description }}</p>
    <p class="mt-3 text-xs leading-6 text-ink-muted" data-role="ai-note">{{ copy.note }}</p>

    <div class="mt-3 flex flex-wrap gap-2">
      <span v-for="capability in copy.capabilities" :key="capability" class="blueprint-chip">{{ capability }}</span>
    </div>

    <div class="mt-4 blueprint-actions">
      <button
        type="button"
        class="blueprint-tab"
        :disabled="!copy.available"
        data-role="ai-open"
        @click="emit('open')"
      >
        {{ copy.actionLabel }}
      </button>
    </div>
  </section>
</template>
