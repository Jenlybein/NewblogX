<script setup lang="ts">
import { computed } from "vue";
import { useRuntimeConfig } from "#imports";

import { brandAssetUrl, brandFullName } from "~/utils/brand";

const props = withDefaults(
  defineProps<{
    size?: number;
    variant?: "render" | "flat";
    decorative?: boolean;
  }>(),
  { size: 40, variant: "render", decorative: false },
);

const config = useRuntimeConfig();
const baseUrl = computed(() => String(config.app.baseURL || "/"));
const dimension = computed(() => Math.max(8, Math.round(props.size)));
const source = computed(() => brandAssetUrl(baseUrl.value, props.variant === "flat" ? "flat" : "logo"));
const altText = computed(() => (props.decorative ? "" : `${brandFullName()} logo`));
</script>

<template>
  <img
    class="brand-logo block shrink-0 select-none object-contain"
    :src="source"
    :alt="altText"
    :width="dimension"
    :height="dimension"
    :aria-hidden="decorative ? 'true' : undefined"
    decoding="async"
  >
</template>
