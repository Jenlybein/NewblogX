<script setup lang="ts">
import {
  IconBookmarks,
  IconDashboard,
  IconFileText,
  IconHeartHandshake,
  IconLogin2,
  IconSettings,
  IconTags,
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
  /**
   * 账号设置的子项（票 355）。
   *
   * 设置页用 `?section=` 切换分栏，所以子项是"同一路由 + 不同查询"。列在侧栏
   * 里而不是页内 tab：用户找"改密码"时会在侧栏找，页内 tab 只有进到设置页
   * 才看得见。子项只在设置页展开 —— 别的页面展开它只是噪音。
   */
  children?: { label: string; to: string }[];
};

/** 账号设置的五个分栏：顺序即页面上的顺序，也即子项顺序。 */
const settingsSections = [
  { label: "个人资料", to: "/studio/settings?section=profile" },
  { label: "账号安全", to: "/studio/settings?section=security" },
  { label: "消息偏好", to: "/studio/settings?section=notify" },
  { label: "可见性", to: "/studio/settings?section=visibility" },
  { label: "隐私", to: "/studio/settings?section=privacy" },
] as const;

const items: StudioNavItem[] = [
  { label: "数据概览", to: "/studio/dashboard", icon: IconDashboard },
  { label: "我的文章", to: "/studio/profile", icon: IconFileText },
  // 浏览历史已提升为顶级页 `/history`：创作区不再有入口（决策 7）。
  { label: "收藏夹", to: "/studio/favorites", icon: IconBookmarks },
  // 分类的管理属于个人中心（票 327）：公开主页的分类栏只保留展示，
  // 集中管理（新建 / 改名 / 删除）放这里，挨着内容相关的收藏夹与关注。
  { label: "分类", to: "/studio/categories", icon: IconTags },
  // 关注与粉丝合成一页（票 327）：二者是同一件事的两面，页内顶栏切换关注/粉丝/互关。
  { label: "关注与粉丝", to: "/studio/relations", icon: IconHeartHandshake },
  { label: "登录设备", to: "/studio/recent-logins", icon: IconLogin2 },
  { label: "账号设置", to: "/studio/settings", icon: IconSettings, children: [...settingsSections] },
];

function isItemActive(item: StudioNavItem) {
  return route.path === item.to;
}

/** 子项是否展开：只在设置页展开，且保持父项处于激活态。 */
function showChildren(item: StudioNavItem) {
  return Boolean(item.children?.length) && route.path === item.to;
}

/** 子项激活判定要连查询一起比：五个子项路径相同、只有 section 不同。 */
function isChildActive(child: { to: string }) {
  const [path, query] = child.to.split("?");
  if (route.path !== path) return false;
  const params = new URLSearchParams(query ?? "");
  const want = params.get("section");
  const current = typeof route.query.section === "string" ? route.query.section : "profile";
  return (want ?? "profile") === current;
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
      <!-- 每一项连同它的子项一起渲染：`v-for` 挂在 template 上，
           否则子项那段不在循环里（只渲染一次、且拿不到 item）。 -->
      <template v-for="item in items" :key="item.to">
        <NuxtLink
          :to="item.to"
          class="studio-sidebar__link"
          :class="{ 'is-active': isItemActive(item) }"
        >
          <span class="studio-sidebar__link-main">
            <component :is="item.icon" :size="18" aria-hidden="true" />
            <span>{{ item.label }}</span>
          </span>
        </NuxtLink>

        <!-- 账号设置的子项（票 355）：只在设置页展开。 -->
        <template v-if="showChildren(item)">
          <NuxtLink
            v-for="child in item.children ?? []"
            :key="child.to"
            :to="child.to"
            class="studio-sidebar__link studio-sidebar__link--child"
            :class="{ 'is-active': isChildActive(child) }"
            :data-role="`settings-section-${child.label}`">
            <span class="studio-sidebar__link-main">{{ child.label }}</span>
          </NuxtLink>
        </template>
      </template>
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
