<script setup lang="ts">
import {
  IconBookmarks,
  IconDashboard,
  IconFileText,
  IconHeartHandshake,
  IconLogin2,
  IconSettings,
  IconTags,
  IconUsers,
  IconUserSquareRounded,
} from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";

const route = useRoute();
const authStore = useAuthStore();

// 消息中心 / 私信已迁出个人中心（第 239 票 起是顶级页 /messages，入口在顶栏）。
// 侧栏不再保留任何消息入口，也不再消费未读 store。
type StudioNavItem = {
  label: string;
  to: string;
  icon: typeof IconDashboard;
};

const items: StudioNavItem[] = [
  { label: "数据概览", to: "/studio/dashboard", icon: IconDashboard },
  { label: "我的文章", to: "/studio/profile", icon: IconFileText },
  // 浏览历史已提升为顶级页 `/history`：创作区不再有入口（决策 7）。
  { label: "收藏夹", to: "/studio/favorites", icon: IconBookmarks },
  // 分类的管理属于个人中心（票 327）：公开主页的分类栏只保留展示与写入口，
  // 集中管理（新建 / 改名 / 删除）放这里，挨着内容相关的收藏夹与关注。
  { label: "分类", to: "/studio/categories", icon: IconTags },
  { label: "关注", to: "/studio/follows", icon: IconHeartHandshake },
  { label: "粉丝", to: "/studio/fans", icon: IconUsers },
  { label: "登录设备", to: "/studio/recent-logins", icon: IconLogin2 },
  { label: "账号设置", to: "/studio/settings", icon: IconSettings },
];

function isItemActive(item: StudioNavItem) {
  return route.path === item.to;
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
        :key="item.to"
        :to="item.to"
        class="studio-sidebar__link"
        :class="{ 'is-active': isItemActive(item) }"
      >
        <span class="studio-sidebar__link-main">
          <component :is="item.icon" :size="18" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </span>
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
