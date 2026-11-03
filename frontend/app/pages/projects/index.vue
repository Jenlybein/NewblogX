<script setup lang="ts">
import { computed } from "vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
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

    <ul v-if="cards.length" class="project-grid" data-project-list="true">
      <li v-for="card in cards" :key="card.id" class="project-card">
        <NuxtLink :to="card.href" class="project-card__link">
          <span class="project-card__cover">
            <ArticleCover :src="card.cover" :title="card.title" label="项目" />
          </span>
          <span class="project-card__body">
            <strong class="project-card__title">{{ card.title }}</strong>
            <span class="project-card__description">{{ card.description }}</span>
            <span class="project-card__meta">{{ card.countLabel }}</span>
          </span>
        </NuxtLink>
      </li>
    </ul>

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
/* 卡片复用合集卡片的视觉语言（语义色 token + 圆角阶梯），不引入新的视觉类。 */
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

.project-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
  gap: 1.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.project-card {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  overflow: hidden;
  transition:
    transform var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease),
    box-shadow var(--motion-fast) var(--motion-ease);
}

.project-card:hover {
  border-color: var(--color-line-strong);
  box-shadow: 0 22px 52px -30px var(--color-shadow);
  transform: translateY(-2px);
}

@media (prefers-reduced-motion: reduce) {
  .project-card:hover {
    transform: none;
  }
}

.project-card__link {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: inherit;
  text-decoration: none;
}

.project-card__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: var(--color-bg-subtle);
}

.project-card__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 0.5rem;
  padding: 1rem 1.125rem 1.25rem;
}

.project-card__title {
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.25rem;
  letter-spacing: -0.015em;
  line-height: 1.3;
}

.project-card__description {
  display: -webkit-box;
  flex: 1 1 auto;
  overflow: hidden;
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.project-card__meta {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

.project-card :is(a):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  border-radius: var(--radius-lg);
}
</style>
