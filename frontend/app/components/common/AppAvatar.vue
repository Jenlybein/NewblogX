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

<style scoped>
/*
 * 头像必须是**正圆**，任何来源的图片都不能被拉成椭圆。
 *
 * 两个坑都在这里收口（阅读页作者卡与个人资料页各踩过一次）：
 *   - naive-ui 的 <img> 只给了 width/height 100%，没有 object-fit，
 *     默认 fill 会把非 1:1 的旧头像与第三方（QQ 等）头像横向拉宽——看起来"扁了"。
 *     改成 cover：按短边裁切，人脸比例保持，圆框内完整填充。
 *   - 头像在 flex 行里默认 flex-shrink: 1：作者卡窄、昵称又长时宽度会被压，
 *     盒子的宽高不再相等，正圆就直接画成椭圆。钉死 flex 基准尺寸。
 */
.n-avatar {
  flex: 0 0 auto;
  overflow: hidden;
}

:deep(img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
</style>
