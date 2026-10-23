<script setup lang="ts">
import { NButton, NTag } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
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

const props = defineProps<{
  title: string;
  items: Array<FollowUserItem | FanUserItem>;
  pending?: boolean;
  locked?: boolean;
  lockedTitle?: string;
  lockedDescription?: string;
}>();

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
  <div v-if="locked" class="surface-section flex min-h-[280px] flex-col items-center justify-center p-6 text-center">
    <h3 class="section-title">{{ lockedTitle }}</h3>
    <p class="mt-3 max-w-xl text-sm leading-7 muted">{{ lockedDescription }}</p>
  </div>

  <div v-else-if="normalizedItems.length" class="profile-relation-list">
    <article v-for="item in normalizedItems" :key="item.id" class="profile-relation-card">
      <div class="profile-relation-card__main">
        <AppAvatar :src="item.avatar" :name="item.nickname" fallback="友" />
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <NuxtLink :to="`/users/${item.id}`" class="text-lg font-semibold">
              {{ item.nickname }}
            </NuxtLink>
            <NTag size="small">{{ getRelationLabel(item.relation) }}</NTag>
          </div>
          <p class="mt-2 break-words text-sm leading-7 muted">{{ item.abstract || "这个用户还没有填写简介。" }}</p>
          <p class="mt-2 text-xs muted">{{ formatDateTimeLabel(item.time) }}</p>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <NuxtLink :to="`/users/${item.id}`" class="glass-badge">查看主页</NuxtLink>
        <NButton quaternary size="small" @click="emit('toggleFollow', item.id, item.relation)">
          {{ getRelationActionLabel(item.relation) }}
        </NButton>
      </div>
    </article>
  </div>

  <div v-else class="surface-section flex min-h-[280px] flex-col items-center justify-center p-6 text-center">
    <h3 class="section-title">{{ pending ? `正在加载${title}…` : `还没有${title}` }}</h3>
    <p class="mt-3 max-w-xl text-sm leading-7 muted">
      {{ pending ? "系统正在读取最新关系数据。" : "当前这个分栏还没有可展示的关系记录。" }}
    </p>
  </div>
</template>
