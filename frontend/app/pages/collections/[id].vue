<script setup lang="ts">
import { computed } from "vue";
import OutlineTree from "~/components/outline/OutlineTree.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useOutlineCollapsed } from "~/composables/useOutlineCollapsed";
import { getCollectionDetail } from "~/services/collection";
import { collectionOutlineView } from "~/utils/collection";

/**
 * 合集详情（票 219：详情页 = 目录页，决定清单 A7–A8）。
 *
 * 渲染边界：不进 SSR 白名单（与列表页、搜索页同一处理，ADR-0019）。
 *
 * 后端返回的是嵌套目录树（`outline`），文件夹可展开 / 收起、文章节点带
 * `?from=collection:<id>` 打开阅读页；「第 N 篇」位置标记随扁平列表退役，
 * 层级只由缩进表达。折叠状态按树存在 localStorage，刷新或返回后保持。
 *
 * 未展示的合集后端按**不存在**返回错误：这里把错误渲染成中性空态，而不是
 * "0 篇文章"——后者会让读者以为合集是空的，也容易把内部状态说破。
 * 树里的失效行（文章变私有 / 被删）后端已在读模型里过滤，前端不补判；
 * 封面回退（深度优先第一篇公开文章的封面）同样由后端算好，前端不另写一份。
 */
const route = useRoute();
const collectionId = computed(() => String(route.params.id ?? ""));

// 折叠状态不依赖取数，先注册：async setup 里晚注册的生命周期钩子容易漏掉。
// 传 computed 而不是 `.value`：站内从 /collections/1 切到 /collections/2 时组件
// 不重挂载，折叠状态必须跟着路由参数换（票 225）。
const { collapsed, toggle } = useOutlineCollapsed("collection", collectionId);

const { data, pending, error } = await useAsyncData(
  () => `reader-collection-${collectionId.value}`,
  () => getCollectionDetail(collectionId.value),
  { watch: [collectionId] },
);

useSeoMeta({
  title: computed(() => data.value?.title || "合集"),
  description: computed(() => data.value?.description || "按主题成组阅读的系列文章。"),
});

const outlineView = computed(() => collectionOutlineView(data.value));
const outlineNodes = computed(() => outlineView.value.nodes);
const articleCount = computed(() => outlineView.value.articleCount);
const description = computed(() => (data.value?.description ?? "").trim());
const emptyTitle = computed(() => (error.value ? "合集不存在" : "这个合集还没有文章"));
const emptyDescription = computed(() =>
  error.value
    ? "这个合集暂时无法访问。"
    : "管理员还没有把文章收录进这个合集。",
);
</script>

<template>
  <main id="main-content" class="page-shell collection-detail">
    <header class="collection-detail__header">
      <NuxtLink to="/collections" class="collection-detail__back">← 返回合集</NuxtLink>

      <h1 class="collection-detail__title">{{ data?.title || "合集" }}</h1>
      <p v-if="description" class="collection-detail__lede">{{ description }}</p>
      <p v-if="articleCount" class="collection-detail__meta">共 {{ articleCount }} 篇</p>
    </header>

    <OutlineTree
      v-if="outlineNodes.length"
      class="collection-detail__outline"
      data-collection-outline="true"
      :nodes="outlineNodes"
      kind="collection"
      :owner-id="collectionId"
      :collapsed="collapsed"
      @toggle="toggle"
    />

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

/* 显式带上 `a` 前缀：链接色只允许用在文字链接上（设计门禁按选择器判定）。 */
a.collection-detail__back {
  display: inline-block;
  margin-bottom: 1rem;
  color: var(--color-link);
  font-size: 0.875rem;
  text-decoration: none;
}

a.collection-detail__back:hover {
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

.collection-detail__outline {
  max-width: 48rem;
}

.collection-detail :is(a, button, [tabindex]):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}
</style>
