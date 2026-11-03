<script setup lang="ts">
import { computed } from "vue";

import ArticleCover from "~/components/article/ArticleCover.vue";
import type { OutlineSourceKind } from "~/utils/outlineKind";
import { markCurrentOutlineRows, visibleOutlineRows } from "~/utils/outlineState";
import type { OutlineNode } from "~/utils/outlineTree";
import { outlineTreeRows } from "~/utils/outlineTree";

/**
 * 读者侧目录树（合集 / 项目详情页共用）。
 *
 * 只渲染树，不取数：父页面在客户端拿到详情后把 `nodes` 与折叠状态传进来。
 * 文件夹可折叠、文章节点带来源链接、当前文章高亮，层级靠缩进表达（notes A7）。
 */
const props = withDefaults(
  defineProps<{
    nodes: OutlineNode[] | null | undefined;
    /** 来源参数里的 kind：决定文章链接的 `?from=`；空串表示来源未知（不给链接）。 */
    kind: OutlineSourceKind | "";
    ownerId: string;
    /** 被收起的文件夹节点 ID（由父页面持久化到 localStorage）。 */
    collapsed: ReadonlySet<string>;
    /** 当前文章 ID（阅读页回跳时用；详情页通常没有）。 */
    currentArticleId?: string;
  }>(),
  { currentArticleId: "" },
);

const emit = defineEmits<{
  (event: "toggle", nodeId: string): void;
}>();

const allRows = computed(() => outlineTreeRows(props.nodes, props.kind, props.ownerId));
const visible = computed(() =>
  markCurrentOutlineRows(
    visibleOutlineRows(
      allRows.value,
      new Set(props.currentArticleId ? [props.currentArticleId] : []),
      props.collapsed,
    ),
    props.currentArticleId,
  ),
);
</script>

<template>
  <ul class="outline-tree" data-outline-tree="true">
    <li
      v-for="row in visible"
      :key="row.id"
      class="outline-tree__item"
      :class="{ 'is-current': row.isCurrent }"
      :data-outline-row="row.id"
      :data-depth="row.depth"
      :data-outline-kind="row.isFolder ? 'folder' : 'article'"
      :style="{ '--outline-indent': row.depth }"
      :aria-current="row.isCurrent ? 'page' : undefined"
    >
      <button
        v-if="row.isFolder"
        type="button"
        class="outline-tree__toggle"
        :aria-expanded="!collapsed.has(row.id)"
        :aria-label="`${collapsed.has(row.id) ? '展开' : '收起'} ${row.label}`"
        :data-outline-toggle="row.id"
        @click="emit('toggle', row.id)"
      >
        <span class="outline-tree__arrow" aria-hidden="true">{{ collapsed.has(row.id) ? "▸" : "▾" }}</span>
        <span class="outline-tree__label">{{ row.label }}</span>
      </button>
      <span v-else-if="!row.href" class="outline-tree__folder-label">
        {{ row.label }}
      </span>
      <NuxtLink v-else :to="row.href" class="outline-tree__article" :data-outline-href="row.href">
        <span class="outline-tree__cover" aria-hidden="true">
          <ArticleCover :src="row.cover" :title="row.label" />
        </span>
        <span class="outline-tree__body">
          <strong class="outline-tree__label">{{ row.label }}</strong>
        </span>
      </NuxtLink>
    </li>
  </ul>
</template>

<style scoped>
.outline-tree {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

/* 层级靠缩进表达：每层一条发丝引导线，不再是「第 N 篇」位置刻度。 */
.outline-tree__item {
  position: relative;
  padding-left: calc(var(--outline-indent) * 1.5rem);
}

.outline-tree__item[data-depth]:not([data-depth="0"])::before {
  position: absolute;
  top: 0;
  bottom: 0;
  left: calc(var(--outline-indent) * 1.5rem - 0.75rem);
  width: 1px;
  background: var(--color-line);
  content: "";
}

.outline-tree__toggle,
.outline-tree__folder-label,
.outline-tree__article {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  width: 100%;
  min-height: 2.75rem;
  padding: 0.5rem 0.75rem;
  border: 0;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-ink);
  text-align: left;
  text-decoration: none;
}

.outline-tree__toggle {
  cursor: pointer;
  font-family: var(--font-title);
  font-size: 1rem;
}

.outline-tree__toggle:hover {
  background: var(--color-bg-subtle);
}

.outline-tree__arrow {
  width: 1rem;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
}

.outline-tree__label {
  min-width: 0;
  overflow-wrap: anywhere;
}

.outline-tree__folder-label {
  color: var(--color-ink-muted);
  font-size: 0.875rem;
}

.outline-tree__article {
  gap: 0.875rem;
}

.outline-tree__cover {
  display: block;
  flex: 0 0 5rem;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--color-bg-subtle);
}

.outline-tree__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

.outline-tree__article:hover .outline-tree__label {
  text-decoration: underline;
}

.outline-tree__item.is-current {
  border-radius: var(--radius-md);
  background: var(--color-highlight);
}

/* 显式带上 `a` 前缀：链接色只允许用在文字链接上（设计门禁按选择器判定）。 */
.outline-tree__item.is-current > a.outline-tree__article .outline-tree__label {
  color: var(--color-link);
}

.outline-tree :is(a, button):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (max-width: 640px) {
  .outline-tree__cover {
    flex-basis: 3.5rem;
  }
}
</style>
