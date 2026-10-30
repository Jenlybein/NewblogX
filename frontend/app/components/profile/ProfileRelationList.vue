<script setup lang="ts">
import { computed } from "vue";
import { NButton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { FanUserItem, FollowUserItem } from "~/types/api";
import { formatDateTimeLabel } from "~/utils/format";
import { getRelationActionLabel, getRelationLabel } from "~/utils/relation";

type RelationRow = {
  id: string;
  nickname: string;
  avatar: string;
  abstract: string;
  time: string;
  relation: number;
};

const props = withDefaults(
  defineProps<{
    title: string;
    items: Array<FollowUserItem | FanUserItem>;
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

const normalizedItems = computed<RelationRow[]>(() =>
  props.items.map((item) =>
    "followed_user_id" in item
      ? {
          id: item.followed_user_id,
          nickname: item.followed_nickname,
          avatar: item.followed_avatar,
          abstract: item.followed_abstract,
          time: item.follow_time,
          relation: item.relation,
        }
      : {
          id: item.fans_user_id,
          nickname: item.fans_nickname,
          avatar: item.fans_avatar,
          abstract: item.fans_abstract,
          time: item.follow_time,
          relation: item.relation,
        },
  ),
);
</script>

<template>
  <ProfileEmptyState
    v-if="locked"
    size="md"
    :title="lockedTitle || `${title}暂未公开`"
    :description="lockedDescription || '这位作者关闭了该列表的公开，当前登录用户无权查看。'"
  />

  <div v-else-if="normalizedItems.length" class="profile-relation-list">
    <article v-for="item in normalizedItems" :key="item.id" class="profile-relation-card">
      <div class="profile-relation-card__main">
        <AppAvatar :src="item.avatar" :name="item.nickname" fallback="友" />
        <div class="min-w-0">
          <div class="profile-relation-card__title-row">
            <NuxtLink :to="`/users/${item.id}`" class="profile-relation-card__name">{{ item.nickname }}</NuxtLink>
            <span class="blueprint-chip">{{ getRelationLabel(item.relation) }}</span>
          </div>
          <p class="profile-relation-card__abstract">{{ item.abstract || "这个用户还没有填写简介。" }}</p>
          <p class="profile-relation-card__time">{{ formatDateTimeLabel(item.time) }}</p>
        </div>
      </div>

      <div class="profile-relation-card__actions blueprint-actions">
        <NuxtLink :to="`/users/${item.id}`" class="blueprint-link">查看主页</NuxtLink>
        <NButton quaternary attr-type="button" @click="emit('toggleFollow', item.id, item.relation)">
          {{ getRelationActionLabel(item.relation) }}
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
