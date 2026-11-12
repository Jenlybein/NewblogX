<script setup lang="ts">
import AppAvatar from "~/components/common/AppAvatar.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { RelationItem } from "~/types/api";
import { getRelationLabel } from "~/utils/relation";

/**
 * 公开主页的关注 / 粉丝列表。
 *
 * 只承担**展示**：公开主页不是"管理我的关系"的地方，所以这里没有取消关注 /
 * 回关按钮 —— 那样一不小心就会在别人的主页上改掉自己的关系，而且两个列表长得
 * 跟个人中心一模一样，分不清自己在哪一页。
 *
 * 布局用用户卡网格而不是一行一条：一屏能看的人更多，视觉上也与搜索页的
 * 「相关用户」一致（复用 search-user-list / search-user-card 那套样式）。
 */
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
</script>

<template>
  <ProfileEmptyState
    v-if="locked"
    size="md"
    :title="lockedTitle || `${title}暂未公开`"
    :description="lockedDescription || '这位作者关闭了该列表的公开，当前登录用户无权查看。'"
  />

  <div v-else-if="items.length" class="profile-relation-grid">
    <NuxtLink
      v-for="item in items"
      :key="item.user_id"
      :to="`/users/${item.user_id}`"
      class="search-user-card"
      :title="`${getRelationLabel(item.relation)} · ${item.nickname}`"
    >
      <AppAvatar class="search-user-card__avatar" :src="item.avatar" :name="item.nickname" fallback="友" />
      <span class="search-user-card__text">
        <span class="search-user-card__name">{{ item.nickname }}</span>
        <span class="search-user-card__abstract">{{ item.abstract || "这个用户还没有填写简介。" }}</span>
      </span>
      <span class="blueprint-chip flex-none">{{ getRelationLabel(item.relation) }}</span>
    </NuxtLink>
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
