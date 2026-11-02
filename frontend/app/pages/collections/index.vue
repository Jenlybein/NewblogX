<script setup lang="ts">
import { computed } from "vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { getCollections } from "~/services/collection";
import { collectionCardView } from "~/utils/collection";

/**
 * 合集列表（读者站顶栏第四项的落地页）。
 *
 * 渲染边界：本页与详情页都**不进 SSR 白名单**（与搜索页一致，ADR-0019），
 * 由 shared/readerRenderingBoundary.ts 的 CLIENT_ONLY_PATTERNS 声明。
 *
 * 可见性完全由后端决定：只返回"展示中"的合集，且已过滤掉没有公开文章的合集。
 * 前端不重复判断——两处判据一旦分叉，就会出现"列表里有卡片、点进去 404"。
 */
const { data, pending, error } = await useAsyncData(
  "reader-collections",
  () => getCollections(1, 24),
  { default: () => ({ list: [], count: 0 }) },
);

useSeoMeta({
  title: "合集",
  description: "按主题成组阅读的系列文章。",
});

const cards = computed(() => (data.value?.list ?? []).map(collectionCardView));
const emptyTitle = computed(() => (error.value ? "合集暂时打不开" : "还没有合集"));
const emptyDescription = computed(() =>
  error.value
    ? "请稍后重试。合集由管理员编排，暂不可读不影响其它页面。"
    : "管理员还没有发布任何合集。合集会把同一主题的文章按顺序串起来。",
);
</script>

<template>
  <main id="main-content" class="page-shell collection-page">
    <header class="collection-page__header">
      <p class="collection-page__kicker">按主题成组阅读</p>
      <h1 class="collection-page__title">合集</h1>
      <p class="collection-page__lede">
        每个合集是一组有顺序的文章：从一个主题的第一篇读到最后一篇，不必靠关键词碰运气。
      </p>
    </header>

    <ul v-if="cards.length" class="collection-grid" data-collection-list="true">
      <li v-for="card in cards" :key="card.id" class="collection-card">
        <NuxtLink :to="card.href" class="collection-card__link">
          <span class="collection-card__cover">
            <ArticleCover :src="card.cover" :title="card.title" label="合集" />
          </span>
          <span class="collection-card__body">
            <strong class="collection-card__title">{{ card.title }}</strong>
            <span class="collection-card__description">{{ card.description }}</span>
            <span class="collection-card__meta">{{ card.countLabel }}</span>
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
/* 卡片复用既有内容卡的视觉语言（语义色 token + 圆角阶梯），不引入新的视觉类。 */
.collection-page {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.collection-page__header {
  max-width: 44rem;
  margin-bottom: 2rem;
}

.collection-page__kicker {
  margin: 0 0 0.5rem;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.collection-page__title {
  margin: 0 0 0.75rem;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(2rem, 4vw, 2.75rem);
  line-height: 1.1;
  letter-spacing: -0.02em;
}

.collection-page__lede {
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 0.9375rem;
  line-height: 1.7;
}

.collection-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
  gap: 1.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.collection-card {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  overflow: hidden;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    box-shadow var(--motion-fast) var(--motion-ease);
}

.collection-card:hover {
  border-color: var(--color-line-strong);
  box-shadow: 0 18px 44px -28px var(--color-shadow);
}

.collection-card__link {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: inherit;
  text-decoration: none;
}

.collection-card__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: var(--color-bg-subtle);
}

.collection-card__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 0.5rem;
  padding: 1rem 1.125rem 1.25rem;
}

.collection-card__title {
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.125rem;
  line-height: 1.35;
}

.collection-card__description {
  flex: 1 1 auto;
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.6;
}

.collection-card__meta {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

.collection-card :is(a):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  border-radius: var(--radius-lg);
}
</style>
