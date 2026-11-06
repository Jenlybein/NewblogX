<script setup lang="ts">
import { computed } from "vue";
import FolderCardList from "~/components/common/FolderCardList.vue";
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
 *
 * 卡片是文件夹形态（票 293）：网格与卡片都由共用组件 `FolderCardList` 提供，
 * 本页不再自带卡片样式——项目列表页用同一份实现。
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
      <h1 class="collection-page__title">合集</h1>
      <span class="collection-page__rule" aria-hidden="true" />
      <p v-if="cards.length" class="collection-page__count">{{ cards.length }} 个专题</p>
    </header>

    <FolderCardList v-if="cards.length" :cards="cards" data-collection-list="true" />

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
.collection-page {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.collection-page__header {
  display: flex;
  align-items: baseline;
  gap: 1rem;
  margin-bottom: 2.25rem;
}

/* 标题走编辑部式的大字：字重与字距拉开对比，不用再加一句解释性文案。 */
.collection-page__title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(2.75rem, 7vw, 4.5rem);
  font-weight: 700;
  letter-spacing: -0.04em;
  line-height: 0.95;
}

/* 标题右侧一道短色条：给标题配一个视觉锚点，纯粹的装饰，不承载信息。
   用墨色而不是首屏那三支色相 —— 三色只属于首屏封面（editorial-hero 的例外边界）。 */
.collection-page__rule {
  display: block;
  width: clamp(2rem, 6vw, 4.5rem);
  height: 4px;
  background: var(--color-ink);
  transform: translateY(-0.75rem);
}

.collection-page__count {
  margin: 0 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

/* 卡片与网格由共用组件 FolderCardList 提供（票 293）：两页不再各写一份卡片样式。 */
</style>
