<script setup lang="ts">
import { computed } from "vue";
import FolderCardList from "~/components/common/FolderCardList.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { getProjects } from "~/services/project";
import { projectCardView } from "~/utils/project";

/**
 * 项目列表（读者站「项目」入口的落地页，票 193）。
 *
 * 渲染边界：本页与详情页都**不进 SSR 白名单**（与合集、搜索页一致，ADR-0019）,
 * 由 shared/readerRenderingBoundary.ts 的 CLIENT_ONLY_PATTERNS 声明。
 *
 * 可见性完全由后端决定：只返回"展示中"的项目，篇数只算仍公开的文章，
 * 封面为空时后端已回退到首篇公开文章的封面。前端不重复判断——两处判据一旦
 * 分叉，就会出现"列表里有卡片、点进去 404"。
 *
 * 与「合集」并列但**不共用数据**；本页内部文案一律写「项目」，不混用「合集」。
 * 卡片与网格由共用组件 `FolderCardList` 提供（票 293），本页不再自带卡片样式。
 */
const { data, pending, error } = await useAsyncData(
  "reader-projects",
  () => getProjects(1, 24),
  { default: () => ({ list: [], count: 0 }) },
);

useSeoMeta({
  title: "项目",
  description: "按项目成组阅读的文章。",
});

const cards = computed(() => (data.value?.list ?? []).map(projectCardView));
const emptyTitle = computed(() => (error.value ? "项目暂时打不开" : "还没有项目"));
const emptyDescription = computed(() =>
  error.value
    ? "请稍后重试。项目由管理员编排，暂不可读不影响其它页面。"
    : "管理员还没有公开任何项目。项目会把同一主题的文章按顺序串起来。",
);
</script>

<template>
  <main id="main-content" class="page-shell project-page">
    <header class="project-page__header">
      <h1 class="project-page__title">项目</h1>
      <span class="project-page__rule" aria-hidden="true" />
      <p v-if="cards.length" class="project-page__count">{{ cards.length }} 个项目</p>
    </header>

    <FolderCardList v-if="cards.length" :cards="cards" data-project-list="true" />

    <ProfileEmptyState
      v-else
      :title="emptyTitle"
      :description="emptyDescription"
      :pending="pending"
      size="lg"
    />
  </main>
</template>

<style scoped>
.project-page {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.project-page__header {
  display: flex;
  align-items: baseline;
  gap: 1rem;
  margin-bottom: 2.25rem;
}

.project-page__title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(2.75rem, 7vw, 4.5rem);
  font-weight: 700;
  letter-spacing: -0.04em;
  line-height: 0.95;
}

/* 标题右侧一道短色条：纯装饰，用墨色而不是首屏那三支色相。 */
.project-page__rule {
  display: block;
  width: clamp(2rem, 6vw, 4.5rem);
  height: 4px;
  background: var(--color-ink);
  transform: translateY(-0.75rem);
}

.project-page__count {
  margin: 0 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

/* 卡片与网格由共用组件 FolderCardList 提供（票 293）：两页不再各写一份卡片样式。 */
</style>
