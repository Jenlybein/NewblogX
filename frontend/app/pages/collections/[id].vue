<script setup lang="ts">
import { computed } from "vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { getCollectionDetail } from "~/services/collection";
import { collectionArticleView } from "~/utils/collection";

/**
 * 合集详情（收录取录：按管理员排好的顺序读一个系列）。
 *
 * 渲染边界：不进 SSR 白名单（与列表页、搜索页同一处理，ADR-0019）。
 *
 * 未展示的合集后端按**不存在**返回错误：这里把错误渲染成明确的空态，
 * 而不是"0 篇文章"——后者会让读者以为合集是空的。
 * 收录里的失效行（文章变私有/被删）后端已在 SQL 里过滤掉，前端不补判。
 */
const route = useRoute();
const collectionId = computed(() => String(route.params.id ?? ""));

const { data, pending, error } = await useAsyncData(
  () => `reader-collection-${collectionId.value}`,
  () => getCollectionDetail(collectionId.value),
  { watch: [collectionId] },
);

useSeoMeta({
  title: computed(() => data.value?.title || "合集"),
  description: computed(() => data.value?.description || "按主题成组阅读的系列文章。"),
});

const articles = computed(() => (data.value?.articles ?? []).map(collectionArticleView));
const description = computed(() => (data.value?.description ?? "").trim());
const emptyTitle = computed(() => (error.value ? "合集不存在或未公开" : "这个合集还没有文章"));
const emptyDescription = computed(() =>
  error.value
    ? "它可能还没发布，也可能已经被管理员收起。合集由管理员编排，尚未公开的合集对读者不可见。"
    : "管理员还没有把文章收录进这个合集。",
);
</script>

<template>
  <main id="main-content" class="page-shell collection-detail">
    <header class="collection-detail__header">
      <NuxtLink to="/collections" class="collection-detail__back">← 返回合集</NuxtLink>

      <h1 class="collection-detail__title">{{ data?.title || "合集" }}</h1>
      <p v-if="description" class="collection-detail__lede">{{ description }}</p>
      <p v-if="articles.length" class="collection-detail__meta">共 {{ articles.length }} 篇</p>
    </header>

    <ol v-if="articles.length" class="collection-series" data-collection-series="true">
      <li
        v-for="article in articles"
        :key="article.id"
        class="collection-series__item"
        data-collection-item="true"
      >
        <span class="collection-series__position" aria-hidden="true">{{ article.positionLabel }}</span>

        <NuxtLink :to="article.href" class="collection-series__article">
          <span class="collection-series__cover">
            <ArticleCover :src="article.cover" :title="article.title" />
          </span>
          <span class="collection-series__body">
            <strong class="collection-series__title">{{ article.title }}</strong>
            <span v-if="article.abstract" class="collection-series__abstract">
              {{ article.abstract }}
            </span>
            <span v-if="article.byline" class="collection-series__byline">{{ article.byline }}</span>
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
.collection-detail {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.collection-detail__header {
  max-width: 44rem;
  margin-bottom: 2rem;
}

.collection-detail__back {
  display: inline-block;
  margin-bottom: 1rem;
  color: var(--color-link);
  font-size: 0.875rem;
  text-decoration: none;
}

.collection-detail__back:hover {
  text-decoration: underline;
}

.collection-detail__title {
  margin: 0 0 0.75rem;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.75rem, 4vw, 2.5rem);
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.collection-detail__lede {
  margin: 0 0 0.75rem;
  color: var(--color-ink-muted);
  font-size: 0.9375rem;
  line-height: 1.7;
}

.collection-detail__meta {
  margin: 0;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

.collection-series {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.collection-series__item {
  display: grid;
  grid-template-columns: 5rem minmax(0, 1fr);
  gap: 1rem;
  align-items: start;
}

/* 位置标记是"我在系列里的第几篇"：等宽字 + 发丝线，做一个可扫读的刻度列。 */
.collection-series__position {
  display: block;
  padding-top: 0.875rem;
  border-top: 1px solid var(--color-line);
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.06em;
  white-space: nowrap;
}

.collection-series__article {
  display: grid;
  grid-template-columns: minmax(0, 8rem) minmax(0, 1fr);
  gap: 1rem;
  padding-top: 0.875rem;
  border-top: 1px solid var(--color-line);
  color: inherit;
  text-decoration: none;
}

.collection-series__item:hover .collection-series__article {
  border-top-color: var(--color-line-strong);
}

.collection-series__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.collection-series__body {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  min-width: 0;
}

.collection-series__title {
  color: var(--color-ink);
  font-size: 1.0625rem;
  line-height: 1.4;
}

.collection-series__abstract {
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.6;
}

.collection-series__byline {
  color: var(--color-ink-faint);
  font-size: 0.75rem;
}

.collection-detail :is(a, button, [tabindex]):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (max-width: 640px) {
  .collection-series__item {
    grid-template-columns: 4rem minmax(0, 1fr);
  }

  .collection-series__article {
    grid-template-columns: minmax(0, 1fr);
  }

  .collection-series__cover {
    display: none;
  }
}
</style>
