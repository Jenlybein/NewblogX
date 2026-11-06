<script setup lang="ts">
import AppAvatar from "~/components/common/AppAvatar.vue";

/**
 * 一条评论（根评论或子评论）的作者块：头像 + 昵称。
 *
 * 为什么抽出来：根评论与子评论各写过一份逐字相同的结构（只有头像边长 42 / 32
 * 不同）——链接地址、静态退化、类名、无障碍语义全一样。两处各写一遍，改一处
 * 忘另一处的风险太高，而「作者区是进个人主页的入口」是评论区的对外契约（票 292）。
 *
 * 语义与两处调用完全一致：
 *   - 有 `user_id` → 头像与昵称包在**同一个** `NuxtLink` 里指向 `/users/{id}`
 *     （不是两个各自可点的入口；链接的可访问名就是昵称）；
 *   - 缺 `user_id` → 退化成静态 `<span>`：不可点、不可聚焦、读屏不当链接；
 *   - 类名沿用 `.comment__author-link(--static)` / `.comment__author`，样式仍只在
 *     `tailwind.css` 一处（hover 下划线、焦点环、墨色字），本组件不带样式。
 */
withDefaults(
  defineProps<{
    /** 作者 ID：空串表示后端没给（退化态）。 */
    userId?: string;
    avatar?: string;
    nickname?: string;
    /** 头像边长（px）：根评论 42、子评论 32。 */
    size?: number;
  }>(),
  { userId: "", avatar: "", nickname: "", size: 42 },
);
</script>

<template>
  <NuxtLink v-if="userId" :to="`/users/${userId}`" class="comment__author-link">
    <AppAvatar :size="size" :src="avatar" :name="nickname" fallback="评" />
    <span class="comment__author">{{ nickname }}</span>
  </NuxtLink>
  <span v-else class="comment__author-link comment__author-link--static">
    <AppAvatar :size="size" :src="avatar" :name="nickname" fallback="评" />
    <span class="comment__author">{{ nickname }}</span>
  </span>
</template>
