<script setup lang="ts">
import { NAlert, NCard } from "naive-ui";
import { getOwnFavoriteFolders } from "~/services/favorite";
import { searchArticles } from "~/services/search";
import { getHistoryArticles } from "~/services/studio";
import { getSelfUserDetail, getMessageSummary, getUserBaseInfo } from "~/services/user";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const ARTICLE_PUBLISH_STATUS_LIST = [1, 2, 3] as const;

const emptySummary = {
  comment_msg_count: 0,
  digg_favor_msg_count: 0,
  private_msg_count: 0,
  system_msg_count: 0,
  global_msg_count: 0,
};

function getSearchTotal(payload: Awaited<ReturnType<typeof searchArticles>>) {
  return Math.max(payload.pagination.total ?? payload.list.length, 0);
}

async function getOwnArticleCountByPublishStatus(status?: number) {
  const payload = await searchArticles({
    type: 4,
    status,
    page: 1,
    limit: 1,
    page_mode: "count",
    sort: 1,
    auth: true,
  });

  return getSearchTotal(payload);
}

async function loadDashboardPart<T>(label: string, loader: Promise<T>, fallback: T, errors: string[]) {
  try {
    return await loader;
  } catch (error) {
    errors.push(label);
    console.warn(`[studio-dashboard] ${label} request failed`, error);
    return fallback;
  }
}

async function loadCurrentUserBase(errors: string[]) {
  const authStore = useAuthStore();
  let userId = authStore.profileId;

  if (!userId) {
    const user = await loadDashboardPart("用户资料", getSelfUserDetail(), null, errors);
    userId = user?.id ?? null;
  }

  if (!userId) {
    errors.push("用户冗余统计");
    return null;
  }

  return loadDashboardPart("用户冗余统计", getUserBaseInfo(userId), null, errors);
}

const { data } = await useAsyncData("studio-dashboard", async () => {
  const sourceErrors: string[] = [];

  const [userBase, summary, articleStatusEntries, favorites, history] = await Promise.all([
    loadCurrentUserBase(sourceErrors),
    loadDashboardPart("消息摘要", getMessageSummary(), emptySummary, sourceErrors),
    Promise.all(
      ARTICLE_PUBLISH_STATUS_LIST.map(async (status) => [
        status,
        await loadDashboardPart(`文章发布态 ${status}`, getOwnArticleCountByPublishStatus(status), 0, sourceErrors),
      ] as const),
    ),
    loadDashboardPart("收藏夹", getOwnFavoriteFolders(), { count: 0, list: [] }, sourceErrors),
    loadDashboardPart("浏览历史", getHistoryArticles({ type: 1 }), { list: [], has_more: false }, sourceErrors),
  ]);

  const articleByStatus = articleStatusEntries.reduce<Record<number, number>>((acc, [status, count]) => {
    acc[status] = count;
    return acc;
  }, {});

  return {
    userBase,
    summary,
    articleCount: userBase?.article_count ?? 0,
    articleByStatus,
    favoritesCount: favorites.count,
    historyCount: history.list.length,
    sourceErrors,
  };
});

const metrics = computed(() => [
  { label: "文章总量", value: data.value?.articleCount ?? 0, note: "来自 users/base.article_count" },
  { label: "草稿", value: data.value?.articleByStatus[1] ?? 0, note: "继续编辑或补资料后再发布" },
  { label: "待审核", value: data.value?.articleByStatus[2] ?? 0, note: "等待审核通过后进入公开流" },
  { label: "已发布", value: data.value?.articleByStatus[3] ?? 0, note: "会出现在公开页与搜索结果里" },
  { label: "收藏夹", value: data.value?.favoritesCount ?? 0, note: "我的分组总数" },
  { label: "浏览历史", value: data.value?.historyCount ?? 0, note: "最近阅读痕迹数量（当前页）" },
  {
    label: "未读消息",
    value:
      (data.value?.summary.comment_msg_count ?? 0) +
      (data.value?.summary.digg_favor_msg_count ?? 0) +
      (data.value?.summary.private_msg_count ?? 0) +
      (data.value?.summary.system_msg_count ?? 0) +
      (data.value?.summary.global_msg_count ?? 0),
    note: "评论、互动、私信、系统与全局通知的聚合值",
  },
  { label: "站龄", value: data.value?.userBase?.code_age ?? 0, note: "来自 users/base.code_age" },
]);

const dashboardWarning = computed(() => {
  const errors = data.value?.sourceErrors ?? [];
  if (!errors.length) return "";
  return `部分概览数据暂时加载失败：${errors.join("、")}。其余模块已按可用数据展示。`;
});

useSeoMeta({
  title: "个人中心 - 数据概览",
});
</script>

<template>
  <div class="page-stack">
    <NAlert v-if="dashboardWarning" type="warning" :bordered="false">
      {{ dashboardWarning }}
    </NAlert>

    <section class="studio-metric-grid">
      <NCard v-for="item in metrics" :key="item.label" class="studio-metric-card" :bordered="false">
        <div class="muted text-sm">{{ item.label }}</div>
        <div class="mt-3 text-3xl font-semibold tracking-[-0.04em]">{{ item.value }}</div>
        <p class="mt-2 text-sm leading-6 muted">{{ item.note }}</p>
      </NCard>
    </section>

    <div class="grid gap-5">
      <section class="studio-list-card max-w-[420px] xl:justify-self-end xl:w-full">
        <div class="studio-toolbar">
          <div>
            <div class="eyebrow">Message</div>
            <h2 class="section-title mt-2">消息摘要</h2>
          </div>
          <NuxtLink to="/studio/inbox" class="glass-badge">打开消息中心</NuxtLink>
        </div>

        <div class="mt-5 space-y-4">
          <div class="sidebar-list-row">
            <span class="muted">评论与回复</span>
            <strong>{{ data?.summary.comment_msg_count ?? 0 }}</strong>
          </div>
          <div class="sidebar-list-row line-divider">
            <span class="muted">点赞与收藏</span>
            <strong>{{ data?.summary.digg_favor_msg_count ?? 0 }}</strong>
          </div>
          <div class="sidebar-list-row line-divider">
            <span class="muted">私信消息</span>
            <strong>{{ data?.summary.private_msg_count ?? 0 }}</strong>
          </div>
          <div class="sidebar-list-row line-divider">
            <span class="muted">系统通知</span>
            <strong>{{ data?.summary.system_msg_count ?? 0 }}</strong>
          </div>
          <div class="sidebar-list-row line-divider">
            <span class="muted">全局通知</span>
            <strong>{{ data?.summary.global_msg_count ?? 0 }}</strong>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
