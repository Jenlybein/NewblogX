<script setup lang="ts">
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from "@tabler/icons-vue";
import { NButton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { UserBaseInfo } from "~/types/api";
import { formatCount } from "~/utils/format";

const props = defineProps<{
  profile: UserBaseInfo;
  abstractText?: string;
  isSelf?: boolean;
  relationText?: string;
  actionDisabled?: boolean;
  actionActive?: boolean;
}>();

const emit = defineEmits<{
  follow: [];
  message: [];
}>();
</script>

<template>
  <div class="profile-hero">
    <div class="profile-hero-main">
      <AppAvatar class="profile-hero-avatar" :size="80" :src="profile.avatar" :name="profile.nickname" fallback="作" />

      <div>
        <div class="text-4xl font-semibold tracking-[-0.04em]">{{ profile.nickname }}</div>
        <p class="mt-3 max-w-2xl text-[15px] leading-7 muted">
          {{ props.abstractText || "这位作者暂未补充个人简介。" }}
        </p>
        <div class="mt-4 flex flex-wrap items-center gap-4 text-sm muted">
          <span class="inline-flex items-center gap-1.5"><IconThumbUp :size="16" /> 点赞 {{ formatCount(profile.digg_count || 0) }}</span>
          <span class="inline-flex items-center gap-1.5"><IconEye :size="16" /> 阅读 {{ formatCount(profile.view_count) }}</span>
          <span class="inline-flex items-center gap-1.5"><IconHeart :size="16" /> 收藏 {{ formatCount(profile.favor_count || 0) }}</span>
          <span class="inline-flex items-center gap-1.5"><IconMessageCircle2 :size="16" /> 评论 {{ formatCount(profile.comment_count || 0) }}</span>
        </div>
      </div>
    </div>

    <div v-if="!isSelf" class="profile-hero-actions flex flex-wrap items-center justify-end gap-3">
      <NButton secondary round :disabled="actionDisabled" attr-type="button" @click="emit('message')">
        私信
      </NButton>
      <NButton
        :type="actionActive ? 'default' : 'primary'"
        :secondary="actionActive"
        round
        attr-type="button"
        :disabled="actionDisabled"
        @click="emit('follow')"
      >
        {{ relationText || "关注作者" }}
      </NButton>
    </div>
    <NButton v-else secondary round @click="navigateTo('/search')">查看站内内容</NButton>
  </div>
</template>
