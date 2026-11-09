<script setup lang="ts">
import { NButton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { RelationItem } from "~/types/api";
import { formatDateTimeLabel } from "~/utils/format";
import { getRelationActionLabel, getRelationLabel } from "~/utils/relation";

withDefaults(
  defineProps<{
    title: string;
    /**
     * 关系条目。票 327 起关注 / 粉丝 / 互关共用一种形状（`RelationItem`），
     * 所以这里不再按字段名分支判断"这是关注还是粉丝"。
     */
    items: RelationItem[];
    pending?: boolean;
    locked?: boolean;
    lockedTitle?: string;
    lockedDescription?: string;
  }>(),
  { pending: false, locked: false, lockedTitle: "", lockedDescription: "" },
);

const emit = defineEmits<{
  toggleFollow: [id: string, relation: number];
}>();

/** 动作文案：公开主页只表达"你与这个人的关系"（互相关注 → 取消关注，对方关注了我 → 回关）。 */
function actionLabel(relation: number) {
  return getRelationActionLabel(relation);
}
</script>

<template>
  <ProfileEmptyState
    v-if="locked"
    size="md"
    :title="lockedTitle || `${title}暂未公开`"
    :description="lockedDescription || '这位作者关闭了该列表的公开，当前登录用户无权查看。'"
  />

  <div v-else-if="items.length" class="profile-relation-list">
    <article v-for="item in items" :key="item.user_id" class="profile-relation-card">
      <div class="profile-relation-card__main">
        <AppAvatar :src="item.avatar" :name="item.nickname" fallback="友" />
        <div class="min-w-0">
          <div class="profile-relation-card__title-row">
            <NuxtLink :to="`/users/${item.user_id}`" class="profile-relation-card__name">{{ item.nickname }}</NuxtLink>
            <span class="blueprint-chip">{{ getRelationLabel(item.relation) }}</span>
          </div>
          <p class="profile-relation-card__abstract">{{ item.abstract || "这个用户还没有填写简介。" }}</p>
          <p class="profile-relation-card__time">{{ formatDateTimeLabel(item.follow_time) }}</p>
        </div>
      </div>

      <div class="profile-relation-card__actions blueprint-actions">
        <NuxtLink :to="`/users/${item.user_id}`" class="blueprint-link">查看主页</NuxtLink>
        <NButton quaternary attr-type="button" @click="emit('toggleFollow', item.user_id, item.relation)">
          {{ actionLabel(item.relation) }}
        </NButton>
      </div>
    </article>
  </div>

  <ProfileEmptyState
    v-else
    size="md"
    :pending="pending"
    :title="pending ? `正在加载${title}` : `还没有${title}`"
    :description="pending ? '系统正在读取最新关系数据。' : '当前这个分栏还没有可展示的关系记录。'"
    :hints="pending ? [] : ['关注/粉丝列表属于登录态接口，先登录后才能查看完整记录。']"
  />
</template>
