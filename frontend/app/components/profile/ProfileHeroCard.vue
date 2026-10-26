<script setup lang="ts">
import { IconEye } from "@tabler/icons-vue";
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
          <!--
            「点赞 / 收藏 / 评论」三项曾经也在这里展示，但后端 user_stat 里根本没有这三个
            累计值（BaseInfoResponse 只返回 view_count / fans_count / follow_count 等），
            于是它们恒显示 0。零 DDL 约束下 user_stat 也加不了列，为三个虚荣指标做跨模块
            聚合同样不划算 —— 所以撤掉这三项，只保留有真实数据的「阅读」。
            若要恢复，先在后端 user_stat 建模（或由 article 模块提供聚合）。
          -->
          <span class="inline-flex items-center gap-1.5"><IconEye :size="16" /> 阅读 {{ formatCount(profile.view_count) }}</span>
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
