<script setup lang="ts">
import { computed } from "vue";
import {
  IconBellRinging,
  IconBookmarks,
  IconClockHour4,
  IconDashboard,
  IconFileText,
  IconHeartHandshake,
  IconLogin2,
  IconMessageCircle,
  IconSettings,
  IconUsers,
  IconUserSquareRounded,
} from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";

const route = useRoute();
const authStore = useAuthStore();
const messageStore = useMessageStore();

type StudioNavItem = {
  label: string;
  to: string | { path: string; query?: Record<string, string> };
  icon: typeof IconDashboard;
  badge?: number;
};

const items = computed<StudioNavItem[]>(() => [
  { label: "数据概览", to: "/studio/dashboard", icon: IconDashboard },
  { label: "我的文章", to: "/studio/profile", icon: IconFileText },
  { label: "浏览历史", to: "/studio/history", icon: IconClockHour4 },
  { label: "收藏夹", to: "/studio/favorites", icon: IconBookmarks },
  { label: "关注", to: "/studio/follows", icon: IconHeartHandshake },
  { label: "粉丝", to: "/studio/fans", icon: IconUsers },
  { label: "最近登录", to: "/studio/recent-logins", icon: IconLogin2 },
  { label: "账号设置", to: "/studio/settings", icon: IconSettings },
  { label: "消息中心", to: { path: "/studio/inbox", query: { tab: "site" } }, icon: IconBellRinging, badge: messageStore.totalUnread },
  { label: "私信", to: { path: "/studio/inbox", query: { tab: "chat" } }, icon: IconMessageCircle, badge: messageStore.summary.private_msg_count },
]);

function isItemActive(item: StudioNavItem) {
  if (typeof item.to === "string") {
    return route.path === item.to;
  }
  if (item.to.path !== route.path) {
    return false;
  }
  const targetTab = item.to.query?.tab;
  if (!targetTab) {
    return true;
  }
  return String(route.query.tab || "site") === targetTab;
}
</script>

<template>
  <aside class="surface-card studio-sidebar">
    <div class="studio-sidebar__profile">
      <div class="studio-sidebar__avatar">
        <AppAvatar
          :key="authStore.profileAvatar || authStore.profileName"
          :size="72"
          :src="authStore.profileAvatar"
          :name="authStore.profileName"
          fallback="我" />
      </div>
      <div class="min-w-0">
        <div class="eyebrow">Studio</div>
        <div class="studio-sidebar__name">{{ authStore.profileName }}</div>
        <div class="muted truncate">
          {{ authStore.currentUser?.abstract || "围绕创作、社交、消息与账号资料管理你的空间。" }}
        </div>
      </div>
    </div>

    <nav class="studio-sidebar__nav" aria-label="个人中心导航">
      <NuxtLink
        v-for="item in items"
        :key="`${typeof item.to === 'string' ? item.to : `${item.to.path}:${item.to.query?.tab || ''}`}`"
        :to="item.to"
        class="studio-sidebar__link"
        :class="{ 'is-active': isItemActive(item) }"
      >
        <span class="studio-sidebar__link-main">
          <component :is="item.icon" :size="18" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </span>
        <span v-if="item.badge" class="studio-sidebar__badge">{{ item.badge }}</span>
      </NuxtLink>
    </nav>

    <NuxtLink
      v-if="authStore.profileId"
      :to="`/users/${authStore.profileId}`"
      class="studio-sidebar__profile-link"
    >
      <IconUserSquareRounded :size="16" aria-hidden="true" />
      <span>查看公开主页</span>
    </NuxtLink>
  </aside>
</template>
