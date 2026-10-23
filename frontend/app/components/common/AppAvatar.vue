<script setup lang="ts">
import { computed } from "vue";
import { NAvatar } from "naive-ui";
import { resolveAvatarInitial, resolveAvatarUrl } from "~/utils/avatar";

type AvatarSize = number | "small" | "medium" | "large";

const props = withDefaults(
  defineProps<{
    src?: unknown;
    name?: unknown;
    fallback?: string;
    size?: AvatarSize;
    round?: boolean;
  }>(),
  {
    fallback: "?",
    round: true,
  },
);

const avatarUrl = computed(() => resolveAvatarUrl(props.src));
const initial = computed(() => resolveAvatarInitial(props.name, props.fallback));
</script>

<template>
  <NAvatar v-if="avatarUrl" :round="round" :size="size" :src="avatarUrl">
    <template #fallback>
      {{ initial }}
    </template>
  </NAvatar>
  <NAvatar v-else :round="round" :size="size">
    {{ initial }}
  </NAvatar>
</template>
