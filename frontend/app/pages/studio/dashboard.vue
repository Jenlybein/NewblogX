<script setup lang="ts">
/**
 * 个人数据概览（票 354，2026-11-12 重做版式）。
 *
 * 这一页要回答一个问题：**我的内容怎么样**。所以从上到下是
 * "我是谁 → 数字 → 创作节奏 → 最近在写什么 → 哪些最受欢迎 → 我的空间 / 待处理"。
 *
 * 三条版式决定：
 *  1. 顶部是一张**报头**：问候、一句话状态、两个入口，右下角是五个数字。
 *     上一版把状态卡、KPI 横条、热力图拆成三块，进来先看到三张"一样重"的白卡；
 *  2. 数字直接写在报头里，*不*再用 MetricCard 那种"标签在上、数字在下"的小卡 ——
 *     五个等重的小卡读不出主次，而这一页的主语就是"已发布多少、被看了多少"；
 *  3. 热力图占满一整行（它本身就是一张横向图表），下面才是两列内容卡。
 *
 * 数据来自一个端点（`GET /api/articles/studio-stats`）：状态数量、四个计数、
 * 最近 5 篇、阅读 Top 5、90 天活跃度一次取齐 —— 分六七次请求时任何一个慢都会
 * 让首屏一半是空的，且"共 N 篇"来自不同时刻的快照、彼此对不上。
 */
import { NAlert, NSkeleton } from "naive-ui";
import { computed } from "vue";
import ActivityHeatmap from "~/components/studio/ActivityHeatmap.vue";
import { getStudioStats } from "~/services/article";
import { getProfileCategories } from "~/services/category";
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
  if (!stats.value) return "概览数据这次没读到，下面的数字可能不完整。";
  const parts: string[] = [];
  if (draftCount.value) parts.push(`${draftCount.value} 篇草稿`);
  if (pendingCount.value) parts.push(`${pendingCount.value} 篇待审核`);
  if (rejectedCount.value) parts.push(`${rejectedCount.value} 篇被拒`);
  if (parts.length === 0) return `已发布 ${publishedCount.value} 篇，眼下没有待办`;
  return `已发布 ${publishedCount.value} 篇，还有 ${parts.join("、")} 要处理`;
});

/**
 * 报头里的五个数字。
 *
 * `value` 为 null 时显示破折号而不是 0 —— "还没读到"与"确实是 0"必须能分开。
 */
const kpis = computed(() => [
  { label: "已发布", value: stats.value ? publishedCount.value : null, suffix: "篇", hint: "全部状态里的已发布" },
  { label: "阅读", value: stats.value?.sum_view ?? null, suffix: "次", hint: "所有文章累计" },
  { label: "点赞", value: stats.value?.sum_digg ?? null, suffix: "", hint: "读者给的赞" },
  { label: "收藏", value: stats.value?.sum_favor ?? null, suffix: "", hint: "被收进收藏夹" },
  { label: "评论", value: stats.value?.sum_comment ?? null, suffix: "", hint: "全部文章评论" },
]);

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
 * 分类概览：读**分类表**里这位作者真实的分类（`GET /articles/category?user_id=`），
 * 不要再拿个人资料里的偏好标签（`like_tag_items`）冒充 —— 那是"喜欢的标签"，
 * 跟作者自己建的文章分类是两回事，所以这里以前永远显示"还没有分类"。
 *
 * 只显示前 5 个：概览是"扫一眼"的页面，把二十个分类全铺开会把下面的站点健康
 * 挤出首屏。完整清单在 /studio/categories。
 */
const categoriesLoadFailed = ref(false)
const { data: categories } = await useAsyncData("studio-dashboard-categories", async () => {
  if (!authStore.profileId) return null
  try {
    return await getProfileCategories(authStore.profileId)
  } catch {
    categoriesLoadFailed.value = true
    return null
  }
})

const categoryOverview = computed(() => (categories.value?.list ?? []).slice(0, 5))
const categoryCount = computed(() => categories.value?.list.length ?? 0)
const categoriesLoaded = computed(() => !categoriesLoadFailed.value);

const folderCount = computed(() => folders.value?.count ?? 0);
const followerCount = computed(() => baseInfo.value?.fans_count ?? 0);
const followingCount = computed(() => baseInfo.value?.follow_count ?? 0);

/** 站点健康：这里只显示"能不能读到数据"，真正的运维指标在后台。 */
const siteHealthy = computed(() => Boolean(stats.value));

/** 阅读 Top 5 的条形：按最高阅读量归一，条形让"第一与第五差多少"一眼睛看得出来。 */
const topMax = computed(() => Math.max(...(stats.value?.top ?? []).map(item => item.view_count ?? 0), 1));

function topBarWidth(viewCount: number) {
  return `${Math.max(4, Math.round((viewCount / topMax.value) * 100))}%`;
}

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
  <div class="page-stack overview">
    <!-- ① 报头：问候 / 状态 / 入口 / 五个数字 -->
    <section class="overview__masthead">
      <div class="overview__intro">
        <p class="overview__eyebrow">
          <span class="overview__mark" aria-hidden="true" />
          Studio · Overview
        </p>
        <h1 class="overview__title">你好，{{ displayName }}</h1>
        <p class="overview__status" data-role="studio-status-line">{{ statusLine }}</p>
      </div>

      <div class="overview__actions">
        <NuxtLink to="/studio/write" class="overview__action overview__action--primary">写一篇</NuxtLink>
        <NuxtLink to="/studio/profile" class="overview__action">我的文章</NuxtLink>
      </div>

      <dl class="overview__kpis" data-role="studio-metrics">
        <div v-for="kpi in kpis" :key="kpi.label" class="overview__kpi">
          <dt class="overview__kpi-label">{{ kpi.label }}</dt>
          <dd class="overview__kpi-value">
            <template v-if="kpi.value !== null">{{ formatCount(kpi.value) }}<small v-if="kpi.suffix">{{ kpi.suffix }}</small></template>
            <template v-else>—</template>
          </dd>
          <span class="overview__kpi-hint">{{ kpi.hint }}</span>
        </div>
      </dl>
    </section>

    <!-- ② 创作节奏：整行图表 -->
    <section class="overview__panel overview__panel--heatmap">
      <NSkeleton v-if="statsPending" text :repeat="4" />
      <ActivityHeatmap v-else :activity="activityMap" :days="stats?.activity_days ?? 91" />
    </section>

    <div class="overview__grid">
      <!-- ③ 最近在写什么 -->
      <section class="overview__panel">
        <header class="overview__panel-head">
          <h2 class="overview__panel-title">最近的文章</h2>
          <NuxtLink to="/studio/profile" class="overview__panel-link">全部文章</NuxtLink>
        </header>
        <p v-if="statsPending" class="overview__skeleton"><NSkeleton text :repeat="3" /></p>
        <ol v-else-if="stats?.recent.length" class="overview__list" data-role="studio-recent">
          <li v-for="item in stats.recent" :key="item.id" class="overview__row">
            <NuxtLink :to="`/article/${item.id}`" class="overview__row-title">{{ item.title }}</NuxtLink>
            <span class="overview__row-meta">{{ formatDay(item.created_at) }}</span>
          </li>
        </ol>
        <p v-else class="overview__empty">还没有文章。去「写一篇」写第一篇。</p>
      </section>

      <!-- ④ 哪些最受欢迎 -->
      <section class="overview__panel">
        <header class="overview__panel-head">
          <h2 class="overview__panel-title">阅读 Top 5</h2>
          <span class="overview__panel-note">按累计阅读</span>
        </header>
        <p v-if="statsPending" class="overview__skeleton"><NSkeleton text :repeat="3" /></p>
        <ol v-else-if="stats?.top.length" class="overview__top" data-role="studio-top">
          <li v-for="(item, index) in stats.top" :key="item.id" class="overview__top-row">
            <span class="overview__rank">{{ index + 1 }}</span>
            <div class="overview__top-body">
              <div class="overview__top-head">
                <NuxtLink :to="`/article/${item.id}`" class="overview__row-title">{{ item.title }}</NuxtLink>
                <span class="overview__row-meta">{{ formatCount(item.view_count) }} 阅读</span>
              </div>
              <span class="overview__bar" aria-hidden="true">
                <span class="overview__bar-fill" :style="{ width: topBarWidth(item.view_count) }" />
              </span>
            </div>
          </li>
        </ol>
        <p v-else class="overview__empty">还没有阅读数据。</p>
      </section>

      <!-- ⑤ 我的空间 -->
      <section class="overview__panel">
        <header class="overview__panel-head">
          <h2 class="overview__panel-title">我的空间</h2>
          <NuxtLink to="/studio/settings" class="overview__panel-link">设置</NuxtLink>
        </header>
        <dl class="overview__facts">
          <div class="overview__fact">
            <dt>收藏夹</dt>
            <dd><NuxtLink to="/studio/favorites">{{ folderCount }}<small>个</small></NuxtLink></dd>
          </div>
          <div class="overview__fact">
            <dt>关注</dt>
            <dd><NuxtLink to="/studio/relations">{{ followingCount }}<small>人</small></NuxtLink></dd>
          </div>
          <div class="overview__fact">
            <dt>粉丝</dt>
            <dd><NuxtLink to="/studio/relations">{{ followerCount }}<small>人</small></NuxtLink></dd>
          </div>
        </dl>

        <h3 class="overview__subhead">
          分类
          <span v-if="categoryCount" class="overview__subhead-count">{{ categoryCount }}</span>
        </h3>
        <div v-if="categoryOverview.length" class="overview__chips" data-role="studio-categories">
          <NuxtLink v-for="item in categoryOverview" :key="item.id" to="/studio/categories" class="overview__chip">
            {{ item.title }}
            <span v-if="item.article_count" class="overview__chip-count">{{ item.article_count }}</span>
          </NuxtLink>
          <NuxtLink
            v-if="categoryCount > categoryOverview.length"
            to="/studio/categories"
            class="overview__chip overview__chip--more">
            还有 {{ categoryCount - categoryOverview.length }} 个
          </NuxtLink>
        </div>
        <p v-else-if="categoriesLoaded" class="overview__empty">
          还没有分类。<NuxtLink to="/studio/categories" class="overview__inline-link">去建一个</NuxtLink>。
        </p>
        <p v-else class="overview__empty">分类这次没读到。<NuxtLink to="/studio/categories" class="overview__inline-link">去分类页看看</NuxtLink>。</p>
      </section>

      <!-- ⑥ 站点健康 + ⑦ 待处理（零值折叠） -->
      <section class="overview__panel">
        <header class="overview__panel-head">
          <h2 class="overview__panel-title">站点与待处理</h2>
          <span class="overview__panel-note" data-role="studio-health">
            {{ siteHealthy ? "数据正常" : "数据暂不可读" }}
          </span>
        </header>

        <p v-if="todoItems.length === 0" class="overview__empty" data-role="studio-todo-empty">无待处理。</p>
        <ul v-else class="overview__list overview__list--todo" data-role="studio-todo">
          <li v-for="item in todoItems" :key="item.label" class="overview__row">
            <NuxtLink :to="item.to" class="overview__row-title">{{ item.label }}</NuxtLink>
            <span class="overview__count-pill">{{ item.count }}</span>
          </li>
        </ul>

        <p class="overview__footnote">
          数字来自创作服务；公开读者看到的是其中已发布的部分。
        </p>
      </section>
    </div>

    <NAlert v-if="!statsPending && !stats" type="warning" :bordered="false" data-role="studio-stats-failed">
      概览数据这次没读到，其余部分不受影响；刷新页面可以重试。
    </NAlert>
  </div>
</template>

<style scoped>
/* 报头：一张纸面上的编辑部页头。全部尺寸走 token / rem，不写死颜色。 */
.overview__masthead {
  display: grid;
  gap: 1.5rem 2rem;
  padding: clamp(1.25rem, 3vw, 2rem);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  box-shadow: 0 24px 60px -48px var(--color-shadow);
}

@media (min-width: 900px) {
  .overview__masthead {
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
  }

  .overview__kpis {
    grid-column: 1 / -1;
  }
}

.overview__eyebrow {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0;
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.overview__mark {
  width: 0.5rem;
  height: 0.5rem;
  background: var(--color-ink);
}

.overview__title {
  margin: 0.75rem 0 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.75rem, 4vw, 2.5rem);
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.1;
}

.overview__status {
  margin: 0.5rem 0 0;
  color: var(--color-ink-muted);
  font-size: 0.9375rem;
  line-height: 1.6;
}

.overview__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.overview__action {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  padding: 0 1rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease),
    background-color var(--motion-fast) var(--motion-ease);
}

.overview__action:hover {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
  background: var(--color-bg-subtle);
}

.overview__action--primary {
  border-color: var(--color-ink);
  background: var(--color-ink);
  color: var(--color-surface);
}

.overview__action--primary:hover {
  border-color: var(--color-ink);
  background: var(--color-ink);
  color: var(--color-surface);
  opacity: 0.9;
}

/* 数字条：五格等宽，格与格之间一条发丝线。 */
.overview__kpis {
  display: grid;
  margin: 0;
  gap: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-top: 1px solid var(--color-line-faint);
  padding-top: 1rem;
}

@media (min-width: 720px) {
  .overview__kpis {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}

.overview__kpi {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.25rem 1rem;
  border-left: 1px solid var(--color-line-faint);
}

.overview__kpi:first-child {
  padding-left: 0;
  border-left: 0;
}

.overview__kpi-label {
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.overview__kpi-value {
  display: flex;
  align-items: baseline;
  gap: 0.25rem;
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.5rem, 2.4vw, 2rem);
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.1;
}

.overview__kpi-value small {
  color: var(--color-ink-faint);
  font-family: var(--font-sans);
  font-size: 0.75rem;
  font-weight: 400;
}

.overview__kpi-hint {
  overflow: hidden;
  color: var(--color-ink-faint);
  font-size: 0.6875rem;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 内容卡：纸面 + 发丝线，标题用版面语言而不是 naive 的卡头。 */
.overview__panel {
  min-width: 0;
  padding: 1.25rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
}

.overview__panel--heatmap {
  padding: 1.25rem clamp(1.25rem, 2.5vw, 1.75rem);
}

.overview__panel-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.875rem;
  padding-bottom: 0.625rem;
  border-bottom: 1px solid var(--color-line-faint);
}

.overview__panel-title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.overview__panel-link {
  color: var(--color-ink-muted);
  font-size: 0.75rem;
  text-decoration: underline;
  text-decoration-color: var(--color-line-strong);
  text-underline-offset: 3px;
}

.overview__panel-link:hover {
  color: var(--color-ink);
}

.overview__panel-note {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.06em;
}

.overview__grid {
  display: grid;
  gap: 1rem;
  grid-template-columns: minmax(0, 1fr);
}

@media (min-width: 900px) {
  .overview__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* 行列表：日期贴右、标题可截断；行间用发丝线分隔。 */
.overview__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.overview__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.875rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--color-line-faint);
}

.overview__row:last-child {
  border-bottom: 0;
}

.overview__row-title {
  min-width: 0;
  overflow: hidden;
  color: var(--color-ink);
  font-size: 0.875rem;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overview__row-title:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.overview__row-meta {
  flex: 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
}

/* Top 5：名次用等宽数字，条形只做相对比较。 */
.overview__top {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  gap: 0.625rem;
  list-style: none;
}

.overview__top-row {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.overview__rank {
  flex: 0 0 auto;
  width: 1.25rem;
  padding-top: 1px;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.overview__top-body {
  min-width: 0;
  flex: 1;
}

.overview__top-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.overview__bar {
  display: block;
  height: 3px;
  margin-top: 0.375rem;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-line-faint);
}

.overview__bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-ink);
}

/* 空间三数：一格一个数字，不画卡片套卡片。 */
.overview__facts {
  display: grid;
  margin: 0;
  gap: 0.5rem;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.overview__fact {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.overview__fact dt {
  color: var(--color-ink-muted);
  font-size: 0.6875rem;
}

.overview__fact dd {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.125rem;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  line-height: 1.2;
}

.overview__fact dd a:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.overview__fact small {
  margin-left: 0.125rem;
  color: var(--color-ink-faint);
  font-family: var(--font-sans);
  font-size: 0.6875rem;
  font-weight: 400;
}

.overview__subhead {
  margin: 1rem 0 0.5rem;
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-weight: 500;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

/* 小标题右侧的总数：分类多的时候先让读者知道"还有多少不在这一屏"。 */
.overview__subhead-count {
  margin-left: 0.375rem;
  color: var(--color-ink-faint);
  letter-spacing: 0.06em;
}

.overview__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
}

.overview__chip {
  display: inline-flex;
  min-height: 26px;
  align-items: center;
  gap: 0.375rem;
  padding: 0 0.625rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  color: var(--color-ink-muted);
  font-size: 0.75rem;
}

.overview__chip:hover {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
}

.overview__chip-count {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
}

/* 「还有 N 个」是出口，不是数据：虚线边、更淡，别和真分类混淆。 */
.overview__chip--more {
  border-style: dashed;
  border-color: var(--color-line-faint);
  color: var(--color-ink-faint);
}

.overview__chip--more:hover {
  border-style: dashed;
  border-color: var(--color-line);
  color: var(--color-ink-muted);
}

.overview__count-pill {
  flex: 0 0 auto;
  min-width: 1.5rem;
  padding: 0 0.375rem;
  border-radius: var(--radius-pill);
  background: var(--color-bg-subtle);
  color: var(--color-ink);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.overview__empty {
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.overview__inline-link {
  color: var(--color-ink);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.overview__footnote {
  margin: 0.875rem 0 0;
  color: var(--color-ink-faint);
  font-size: 0.6875rem;
  line-height: 1.5;
}

.overview__skeleton {
  margin: 0;
}

@media (prefers-reduced-motion: reduce) {
  .overview__action {
    transition: none;
  }
}
</style>
