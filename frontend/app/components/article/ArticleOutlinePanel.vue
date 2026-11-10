<script setup lang="ts">
import { computed } from "vue";

import OutlineTree from "~/components/outline/OutlineTree.vue";
import { useOutlineCollapsed } from "~/composables/useOutlineCollapsed";
import type { ArticleOutlineContext } from "~/utils/outlineContext";
import {
  articleOutlineContextHref,
  outlineArticleRefTitle,
  outlineContextKind,
} from "~/utils/outlineContext";
import type { OutlineSourceKind } from "~/utils/outlineKind";

/**
 * 阅读页左栏的树与上一篇/下一篇（票 220 / 决定清单 A9）。
 *
 * 只消费文章上下文接口的读模型：树复用详情页的 `OutlineTree`（同一个高亮与
 * 折叠实现），相邻文章链接沿用当前树的 `from`，翻页仍留在同一棵树里。
 */
const props = defineProps<{
  /** 只在 `has_tree` 时挂载；可空是为了让页面/抽屉复用同一个组件属性形状。 */
  context: ArticleOutlineContext | null;
  currentArticleId: string;
}>();

const nodes = computed(() => props.context?.nodes ?? []);
// 归属 kind 未知 / 缺省时给空串：不猜默认归属，树里就不生成来源链接。
const kind = computed<OutlineSourceKind | "">(() => outlineContextKind(props.context));
const ownerId = computed(() => String(props.context?.owner_id ?? ""));
const previousHref = computed(() =>
  articleOutlineContextHref(props.context, props.context?.previous?.article_id),
);
const nextHref = computed(() => articleOutlineContextHref(props.context, props.context?.next?.article_id));
const previousTitle = computed(() => outlineArticleRefTitle(props.context?.previous));
const nextTitle = computed(() => outlineArticleRefTitle(props.context?.next));

// 传 computed 而不是 `.value`：翻页到另一棵树的文章时组件不重挂载，
// 折叠状态必须跟着 `?from=` 与 owner_id 换（票 225）。
const { collapsed, toggle } = useOutlineCollapsed(kind, ownerId);
</script>

<template>
  <div class="article-outline__body">
    <OutlineTree
      :nodes="nodes"
      :kind="kind"
      :owner-id="ownerId"
      :collapsed="collapsed"
      :current-article-id="currentArticleId"
      @toggle="toggle"
    />

    <nav class="article-outline__pager" aria-label="目录翻页">
      <p class="article-outline__pager-eyebrow">继续阅读</p>
      <NuxtLink
        v-if="previousHref"
        :to="previousHref"
        class="article-outline__pager-link"
        data-outline-neighbour="previous"
      >
        <span class="article-outline__pager-dir">← 上一篇</span>
        <span class="article-outline__pager-title">{{ previousTitle }}</span>
      </NuxtLink>
      <NuxtLink
        v-if="nextHref"
        :to="nextHref"
        class="article-outline__pager-link article-outline__pager-link--next"
        data-outline-neighbour="next"
      >
        <span class="article-outline__pager-dir">下一篇 →</span>
        <span class="article-outline__pager-title">{{ nextTitle }}</span>
      </NuxtLink>
    </nav>
  </div>
</template>

<style scoped>
.article-outline__body {
  margin-top: 0.75rem;
}

.article-outline__pager {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid var(--color-line);
}

/* 翻页区先给一句小标题，把"树的结束"和"继续往前读"分开。 */
.article-outline__pager-eyebrow {
  margin: 0 0 0.25rem;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.14em;
}

.article-outline__pager-link {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  min-width: 0;
  padding: 0.55rem 0.7rem;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  text-decoration: none;
  transition:
    background var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.article-outline__pager-link:hover {
  background: var(--color-bg-subtle);
  border-color: var(--color-line-faint);
}

/* 方向靠位移暗示：上一篇向左退一点，下一篇向右探一点。 */
.article-outline__pager-link[data-outline-neighbour="previous"]:hover {
  transform: translateX(-2px);
}

.article-outline__pager-link[data-outline-neighbour="next"]:hover {
  transform: translateX(2px);
}

.article-outline__pager-dir {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.08em;
}

.article-outline__pager-title {
  overflow-wrap: anywhere;
  color: var(--color-ink);
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.5;
  transition: color var(--motion-fast) var(--motion-ease);
}

a.article-outline__pager-link:hover .article-outline__pager-title {
  color: var(--color-link);
}

.article-outline__pager :is(a):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .article-outline__pager-link {
    transition: none;
  }

  .article-outline__pager-link:hover {
    transform: none;
  }
}
</style>
