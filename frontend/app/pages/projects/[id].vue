<script setup lang="ts">
import { computed } from "vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { getProjectDetail } from "~/services/project";
import { projectArticleView } from "~/utils/project";

/**
 * 项目详情（按管理员排好的顺序读一个项目，票 193）。
 *
 * 渲染边界：不进 SSR 白名单（与列表页、合集、搜索页同一处理，ADR-0019）。
 *
 * 未展示的项目后端按**不存在**返回错误：这里把错误渲染成中性空态，
 * 而不是"0 篇文章"——后者会让读者以为项目是空的。空态文案不得出现
 * "未发布 / 未公开 / 不可见"这类内部状态（spec story 12：对读者按不存在处理）。
 * 收录里的失效行（文章变私有/被删）后端已在 SQL 里过滤掉，前端不补判。
 */
const route = useRoute();
const projectId = computed(() => String(route.params.id ?? ""));

const { data, pending, error } = await useAsyncData(
  () => `reader-project-${projectId.value}`,
  () => getProjectDetail(projectId.value),
  { watch: [projectId] },
);

useSeoMeta({
  title: computed(() => data.value?.title || "项目"),
  description: computed(() => data.value?.description || "按项目成组阅读的文章。"),
});

const articles = computed(() => (data.value?.articles ?? []).map(projectArticleView));
const description = computed(() => (data.value?.description ?? "").trim());
const emptyTitle = computed(() => (error.value ? "项目不存在" : "这个项目还没有文章"));
const emptyDescription = computed(() =>
  error.value ? "这个项目暂时无法访问。" : "管理员还没有把文章收录进这个项目。",
);
</script>

<template>
  <main id="main-content" class="page-shell project-detail">
    <header class="project-detail__header">
      <NuxtLink to="/projects" class="project-detail__back">← 返回项目</NuxtLink>

      <h1 class="project-detail__title">{{ data?.title || "项目" }}</h1>
      <p v-if="description" class="project-detail__lede">{{ description }}</p>
      <p v-if="articles.length" class="project-detail__meta">共 {{ articles.length }} 篇</p>
    </header>

    <ol v-if="articles.length" class="project-series" data-project-series="true">
      <li
        v-for="article in articles"
        :key="article.id"
        class="project-series__item"
        data-project-item="true"
      >
        <span class="project-series__position" aria-hidden="true">{{ article.positionLabel }}</span>

        <NuxtLink :to="article.href" class="project-series__article">
          <span class="project-series__cover">
            <ArticleCover :src="article.cover" :title="article.title" />
          </span>
          <span class="project-series__body">
            <strong class="project-series__title">{{ article.title }}</strong>
            <span v-if="article.abstract" class="project-series__abstract">
              {{ article.abstract }}
            </span>
            <span v-if="article.byline" class="project-series__byline">{{ article.byline }}</span>
          </span>
        </NuxtLink>
      </li>
    </ol>

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
.project-detail {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.project-detail__header {
  max-width: 44rem;
  margin-bottom: 2rem;
}

/* 显式带上 `a` 前缀：链接色只允许用在文字链接上（设计门禁按选择器判定）。 */
a.project-detail__back {
  display: inline-block;
  margin-bottom: 1rem;
  color: var(--color-link);
  font-size: 0.875rem;
  text-decoration: none;
}

a.project-detail__back:hover {
  text-decoration: underline;
}

.project-detail__title {
  margin: 0 0 0.75rem;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.75rem, 4vw, 2.5rem);
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.project-detail__lede {
  margin: 0 0 0.75rem;
  color: var(--color-ink-muted);
  font-size: 0.9375rem;
  line-height: 1.7;
}

.project-detail__meta {
  margin: 0;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

.project-series {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.project-series__item {
  display: grid;
  grid-template-columns: 5rem minmax(0, 1fr);
  gap: 1rem;
  align-items: start;
}

/* 位置标记是"我在项目里的第几篇"：等宽字 + 发丝线，做一个可扫读的刻度列。 */
.project-series__position {
  display: block;
  padding-top: 0.875rem;
  border-top: 1px solid var(--color-line);
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.06em;
  white-space: nowrap;
}

.project-series__article {
  display: grid;
  grid-template-columns: minmax(0, 8rem) minmax(0, 1fr);
  gap: 1rem;
  padding-top: 0.875rem;
  border-top: 1px solid var(--color-line);
  color: inherit;
  text-decoration: none;
}

.project-series__item:hover .project-series__article {
  border-top-color: var(--color-line-strong);
}

.project-series__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.project-series__body {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  min-width: 0;
}

.project-series__title {
  color: var(--color-ink);
  font-size: 1.0625rem;
  line-height: 1.4;
}

.project-series__abstract {
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.6;
}

.project-series__byline {
  color: var(--color-ink-faint);
  font-size: 0.75rem;
}

.project-detail :is(a, button, [tabindex]):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (max-width: 640px) {
  .project-series__item {
    grid-template-columns: 4rem minmax(0, 1fr);
  }

  .project-series__article {
    grid-template-columns: minmax(0, 1fr);
  }

  .project-series__cover {
    display: none;
  }
}
</style>
