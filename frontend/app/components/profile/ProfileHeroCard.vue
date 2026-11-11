<script setup lang="ts">
import { computed } from "vue";
import { NButton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { UserBaseInfo } from "~/types/api";
import { formatCount } from "~/utils/format";

/**
 * 公开主页的头部卡。
 *
 * 2026-11-12 收紧：
 *  - 去掉简介下面的「阅读 N」——右栏「个人成就」已经有（且那是主页访问量），
 *    同一屏里出现两次同一数字只会让人以为哪个是错的；
 *  - 去掉「进入个人中心」——顶栏已经有入口，本人看主页时再放一个按钮是噪音；
 *  - 「文章」从三项指标里撤掉：文章数在右栏个人成就里，头部只留关注 / 粉丝。
 */
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

const metrics = computed(() => [
  { label: "关注", value: formatCount(props.profile.follow_count) },
  { label: "粉丝", value: formatCount(props.profile.fans_count) },
]);
</script>

<template>
  <div class="profile-hero">
    <div class="profile-hero-main">
      <AppAvatar class="profile-hero-avatar" :size="80" :src="profile.avatar" :name="profile.nickname" fallback="作" />

      <div class="min-w-0">
        <h1 class="profile-hero-name">{{ profile.nickname }}</h1>
        <p class="profile-hero-abstract">
          {{ props.abstractText || "这位作者暂未补充个人简介。" }}
        </p>
      </div>
    </div>

    <div class="profile-hero-side">
      <div class="profile-hero-metrics" aria-label="作者数据">
        <div v-for="metric in metrics" :key="metric.label" class="profile-hero-metric">
          <strong class="profile-hero-metric__value">{{ metric.value }}</strong>
          <span class="profile-hero-metric__label">{{ metric.label }}</span>
        </div>
      </div>

      <div v-if="!isSelf" class="profile-hero-actions blueprint-actions">
        <NButton secondary :disabled="actionDisabled" attr-type="button" @click="emit('message')">
          私信
        </NButton>
        <NButton
          :type="actionActive ? 'default' : 'primary'"
          :secondary="actionActive"
          attr-type="button"
          :disabled="actionDisabled"
          @click="emit('follow')"
        >
          {{ relationText || "关注作者" }}
        </NButton>
      </div>
    </div>
  </div>
</template>
