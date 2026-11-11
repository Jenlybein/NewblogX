<script setup lang="ts">
/**
 * 个人数据概览（票 354 重构）。
 *
 * 这一页要回答一个问题：**我的内容怎么样**。所以从上到下是"状态 → 数字 →
 * 最近在写什么 → 哪些最受欢迎 → 我的空间与社交 → 站点是否正常 → 有没有待处理"。
 *
 * 三条版式决定：
 *  1. 先一句话状态摘要，再进数字：进来第一眼要知道"有没有待办"，而不是先读 KPI；
 *  2. KPI 是一条紧凑横条（五个 MetricCard），不是一面等重的大卡墙；
 *  3. **零值折叠**：待处理全是 0 时只显示一行「无待处理」，不再排五个 0。
 *
 * 数据来自一个端点（`GET /api/articles/studio-stats`）：状态数量、四个计数、
 * 最近 5 篇、阅读 Top 5、90 天活跃度一次取齐 —— 分六七次请求时任何一个慢都会
 * 让首屏一半是空的，且"共 N 篇"来自不同时刻的快照、彼此对不上。
 */
import { NAlert, NCard, NSkeleton } from "naive-ui";
import { computed } from "vue";
import ActivityHeatmap from "~/components/studio/ActivityHeatmap.vue";
import MetricCard from "~/components/studio/MetricCard.vue";
import { getStudioStats } from "~/services/article";
import { getOwnFavoriteFolders } from "~/services/favorite";
import { getMessageSummary, getSelfUserDetail, getUserBaseInfo } from "~/services/user";
import { formatCount } from "~/utils/format";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const authStore = useAuthStore();

const { data: stats, pending: statsPending } = await useAsyncData("studio-stats", () => getStudioStats());

// 其余几块各自独立：任何一块失败都不该让整页变成错误态（概览是"看一眼"的页面）。
const { data: user } = await useAsyncData("studio-dashboard-user", () => getSelfUserDetail().catch(() => null));
const { data: baseInfo } = await useAsyncData("studio-dashboard-base", () =>
  authStore.profileId ? getUserBaseInfo(authStore.profileId).catch(() => null) : Promise.resolve(null),
);
const { data: summary } = await useAsyncData("studio-dashboard-summary", () => getMessageSummary().catch(() => null));
const { data: folders } = await useAsyncData("studio-dashboard-folders", () => getOwnFavoriteFolders().catch(() => null));

const displayName = computed(() => user.value?.nickname || authStore.profileName || "作者");

/**
 * 把稀疏的活动数组翻成 `{日期: 次数}`。
 *
 * 接口返回的是数组（契约里"稀疏"用数组表达），热力图要按日期查，所以在这里
 * 转一次；转换放在页面而不是组件里，组件就只认一种形状。
 */
const activityMap = computed<Record<string, number>>(() => {
  const out: Record<string, number> = {};
  const list = Array.isArray(stats.value?.activity) ? stats.value.activity : [];
  for (const item of list) {
    out[item.day] = item.count;
  }
  return out;
});

/** 按发布态取数量；没出现过的状态是 0。 */
function statusCount(status: number) {
  return stats.value?.status_counts.find(item => item.status === status)?.count ?? 0;
}

const draftCount = computed(() => statusCount(1));
const pendingCount = computed(() => statusCount(2));
const publishedCount = computed(() => statusCount(3));
const rejectedCount = computed(() => statusCount(4));

/** 一句话状态摘要：进来第一眼要知道"有没有待办"。 */
const statusLine = computed(() => {
  if (statsPending.value) return "正在读取你的数据…";
  const parts: string[] = [];
  if (draftCount.value) parts.push(`${draftCount.value} 篇草稿`);
  if (pendingCount.value) parts.push(`${pendingCount.value} 篇待审核`);
  if (rejectedCount.value) parts.push(`${rejectedCount.value} 篇被拒`);
  if (parts.length === 0) return `已发布 ${publishedCount.value} 篇，眼下没有待办`;
  return `已发布 ${publishedCount.value} 篇，还有 ${parts.join("、")} 要处理`;
});

/** 待处理项：全为 0 时折叠成一行。 */
const todoItems = computed(() => {
  const items: { label: string; count: number; to: string }[] = [];
  if (draftCount.value) items.push({ label: "草稿", count: draftCount.value, to: "/studio/profile?status=1" });
  if (pendingCount.value) items.push({ label: "待审核", count: pendingCount.value, to: "/studio/profile?status=2" });
  if (rejectedCount.value) items.push({ label: "被拒", count: rejectedCount.value, to: "/studio/profile?status=4" });
  const msg = summary.value;
  if (msg) {
    const unread = (msg.comment_msg_count ?? 0) + (msg.digg_favor_msg_count ?? 0) + (msg.private_msg_count ?? 0);
    if (unread) items.push({ label: "未读消息", count: unread, to: "/messages" });
  }
  return items;
});

/**
 * 分类概览：从个人资料里的偏好标签推不出来，所以这里读用户详情里已有的分类。
 *
 * 只显示前 5 个：概览是"扫一眼"的页面，把二十个分类全铺开会把下面的站点健康
 * 挤出首屏。完整清单在 /studio/categories。
 */
const categoryOverview = computed(() => (user.value?.like_tag_items ?? []).slice(0, 5));

const folderCount = computed(() => folders.value?.count ?? 0);
const followerCount = computed(() => baseInfo.value?.fans_count ?? 0);
const followingCount = computed(() => baseInfo.value?.follow_count ?? 0);

/** 站点健康：这里只显示"能不能读到数据"，真正的运维指标在后台。 */
const siteHealthy = computed(() => Boolean(stats.value));

function formatDay(iso?: string) {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}/${m}/${d}`;
}

useSeoMeta({ title: "个人中心 - 数据概览" });
useNoIndexPage();
</script>

<template>
  <div class="page-stack">
    <!-- ① 一句话状态 -->
    <section class="surface-card p-5 md:p-6" data-role="studio-status">
      <div class="eyebrow">Overview</div>
      <h1 class="section-title mt-2">你好，{{ displayName }}</h1>
      <p class="muted mt-1" data-role="studio-status-line">{{ statusLine }}</p>
    </section>

    <!-- ② 紧凑 KPI 横条 -->
    <section class="metric-strip" data-role="studio-metrics">
      <MetricCard label="已发布" :value="publishedCount" suffix="篇" />
      <MetricCard label="阅读" :value="stats?.sum_view ?? null" suffix="次" />
      <MetricCard label="点赞" :value="stats?.sum_digg ?? null" />
      <MetricCard label="收藏" :value="stats?.sum_favor ?? null" />
      <MetricCard label="评论" :value="stats?.sum_comment ?? null" />
    </section>

    <!-- ③ 创作热力图 -->
    <NCard class="studio-list-card" :bordered="false">
      <NSkeleton v-if="statsPending" text :repeat="3" />
      <ActivityHeatmap v-else :activity="activityMap" :days="stats?.activity_days ?? 91" />
    </NCard>

    <div class="grid gap-5 lg:grid-cols-2">
      <!-- ④ 最近在写什么 -->
      <NCard class="studio-list-card" :bordered="false">
        <h2 class="section-title">最近的文章</h2>
        <p v-if="statsPending" class="mt-3"><NSkeleton text :repeat="3" /></p>
        <ul v-else-if="stats?.recent.length" class="mt-3 space-y-2" data-role="studio-recent">
          <li v-for="item in stats.recent" :key="item.id" class="flex items-center justify-between gap-3">
            <NuxtLink :to="`/article/${item.id}`" class="min-w-0 truncate hover:underline">{{ item.title }}</NuxtLink>
            <span class="shrink-0 text-xs muted">{{ formatDay(item.created_at) }}</span>
          </li>
        </ul>
        <p v-else class="mt-3 text-sm muted">还没有文章。去「创作文章」写第一篇。</p>
      </NCard>

      <!-- ⑤ 哪些最受欢迎 -->
      <NCard class="studio-list-card" :bordered="false">
        <h2 class="section-title">阅读 Top 5</h2>
        <p v-if="statsPending" class="mt-3"><NSkeleton text :repeat="3" /></p>
        <ol v-else-if="stats?.top.length" class="mt-3 space-y-2" data-role="studio-top">
          <li v-for="(item, index) in stats.top" :key="item.id" class="flex items-center justify-between gap-3">
            <span class="min-w-0 truncate">
              <span class="mr-2 text-xs muted">{{ index + 1 }}</span>
              <NuxtLink :to="`/article/${item.id}`" class="hover:underline">{{ item.title }}</NuxtLink>
            </span>
            <span class="shrink-0 text-xs muted">{{ formatCount(item.view_count) }} 阅读</span>
          </li>
        </ol>
        <p v-else class="mt-3 text-sm muted">还没有阅读数据。</p>
      </NCard>

      <!-- ⑥ 我的空间与社交 -->
      <NCard class="studio-list-card" :bordered="false">
        <h2 class="section-title">我的空间</h2>
        <div class="mt-3 space-y-2 text-sm">
          <div class="flex items-center justify-between">
            <span class="muted">收藏夹</span>
            <NuxtLink to="/studio/favorites" class="hover:underline">{{ folderCount }} 个</NuxtLink>
          </div>
          <div class="flex items-center justify-between">
            <span class="muted">关注</span>
            <NuxtLink to="/studio/relations" class="hover:underline">{{ followingCount }} 人</NuxtLink>
          </div>
          <div class="flex items-center justify-between">
            <span class="muted">粉丝</span>
            <NuxtLink to="/studio/relations" class="hover:underline">{{ followerCount }} 人</NuxtLink>
          </div>
        </div>

        <h3 class="mt-4 text-sm font-medium">分类</h3>
        <div v-if="categoryOverview.length" class="mt-2 flex flex-wrap gap-2" data-role="studio-categories">
          <span v-for="item in categoryOverview" :key="item.id" class="glass-badge">
            {{ item.title }}
          </span>
        </div>
        <p v-else class="mt-2 text-xs muted">还没有分类。<NuxtLink to="/studio/categories" class="hover:underline">去建一个</NuxtLink>。</p>
      </NCard>

      <!-- ⑦ 站点健康 + ⑧ 待处理（零值折叠） -->
      <NCard class="studio-list-card" :bordered="false">
        <h2 class="section-title">站点与待处理</h2>

        <div class="mt-3 flex items-center justify-between text-sm">
          <span class="muted">数据读取</span>
          <span :class="siteHealthy ? '' : 'text-danger'" data-role="studio-health">
            {{ siteHealthy ? "正常" : "概览数据暂时读不到" }}
          </span>
        </div>

        <!-- 零值折叠：全是 0 时只留一行，不再排五个 0。 -->
        <p v-if="todoItems.length === 0" class="mt-3 text-sm muted" data-role="studio-todo-empty">无待处理。</p>
        <ul v-else class="mt-3 space-y-2 text-sm" data-role="studio-todo">
          <li v-for="item in todoItems" :key="item.label" class="flex items-center justify-between">
            <NuxtLink :to="item.to" class="hover:underline">{{ item.label }}</NuxtLink>
            <span class="text-xs muted">{{ item.count }}</span>
          </li>
        </ul>
      </NCard>
    </div>

    <NAlert v-if="!statsPending && !stats" type="warning" :bordered="false" data-role="studio-stats-failed">
      概览数据这次没读到，其余部分不受影响；刷新页面可以重试。
    </NAlert>
  </div>
</template>
