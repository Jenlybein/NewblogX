<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";

import ArticleOutlinePanel from "~/components/article/ArticleOutlinePanel.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import type { ArticleOutlineContext } from "~/utils/outlineContext";

/**
 * 阅读页左栏目录（票 220 / 决定清单 A9–A11）。
 *
 * 三种状态：
 *  - `pending`：先出与真实侧栏同构的骨架，树到达后原地替换（不引发布局跳动）；
 *  - `ready`：渲染树 + 上一篇/下一篇（树复用 `OutlineTree`）；
 *  - `empty` / 无归属：整栏不渲染，右栏标题大纲与阅读进度保持原样。
 *
 * `<1024px` 时整栏收进「目录」抽屉：宽屏是 `.article-layout` 的第一列，
 * 窄屏默认只留一个「目录」按钮，点开是覆盖正文的抽屉（带遮罩与 Esc 关闭）。
 */
const props = defineProps<{
  context: ArticleOutlineContext | null;
  pending: boolean;
  currentArticleId: string;
}>();

const hasTree = computed(() => props.context?.has_tree === true);
const visible = computed(() => props.pending || hasTree.value);
const panelState = computed(() => (props.pending ? "pending" : "ready"));
// 切树（合集/项目换了 owner）时重建面板：折叠状态按树存 localStorage。
const panelKey = computed(() => `${props.context?.owner_kind ?? 0}:${props.context?.owner_id ?? ""}`);

const drawerOpen = ref(false);

function closeDrawer() {
  drawerOpen.value = false;
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    closeDrawer();
  }
}

watch(drawerOpen, (open) => {
  if (!import.meta.client) {
    return;
  }
  if (open) {
    window.addEventListener("keydown", handleKeydown);
  } else {
    window.removeEventListener("keydown", handleKeydown);
  }
});

onBeforeUnmount(() => {
  if (import.meta.client) {
    window.removeEventListener("keydown", handleKeydown);
  }
});
</script>

<template>
  <aside v-if="visible" class="article-outline" data-article-outline="true" :data-state="panelState">
    <button
      type="button"
      class="article-outline__drawer-toggle"
      :aria-expanded="drawerOpen ? 'true' : 'false'"
      aria-controls="article-outline-panel"
      data-outline-drawer-toggle
      @click="drawerOpen = !drawerOpen"
    >
      <span class="article-outline__drawer-mark" aria-hidden="true">≡</span>
      目录
    </button>

    <button
      v-if="drawerOpen"
      type="button"
      class="article-outline__backdrop"
      aria-label="关闭目录"
      data-outline-drawer-backdrop
      @click="closeDrawer"
    />

    <section
      id="article-outline-panel"
      class="article-paper article-outline__panel"
      :class="{ 'is-drawer-open': drawerOpen }"
      data-outline-panel
    >
      <div class="article-outline__head">
        <p class="article-eyebrow">
          <span class="article-eyebrow__mark" aria-hidden="true" />
          Outline
        </p>
        <h2 class="article-outline__title">目录</h2>
      </div>

      <div v-if="pending" data-role="article-outline-skeleton">
        <LoadingSkeleton :rows="8" variant="text" label="文章目录正在加载" />
      </div>
      <ArticleOutlinePanel
        v-else-if="context && hasTree"
        :key="panelKey"
        :context="context"
        :current-article-id="currentArticleId"
      />
    </section>
  </aside>
</template>

<style scoped>
/* 宽屏：这一栏是 .article-layout 的第一列，与正文/右栏同高起点。 */
.article-outline {
  min-width: 0;
}

.article-outline__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.article-outline__title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1rem;
  letter-spacing: -0.01em;
}

/* 窄屏默认只留按钮；宽屏隐藏按钮、面板回到文档流。 */
.article-outline__drawer-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  min-height: 2.75rem;
  padding: 0.5rem 0.875rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-ink);
  font-family: var(--font-title);
  cursor: pointer;
}

.article-outline__drawer-mark {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
}

.article-outline__panel {
  display: none;
  padding: 1rem;
}

.article-outline__backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
  border: 0;
  background: var(--color-ink);
  opacity: 0.6;
  cursor: pointer;
}

/* 窄屏抽屉：覆盖在正文之上，内部自己滚动；正文不被长期挤窄。 */
.article-outline__panel.is-drawer-open {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 50;
  display: block;
  width: min(22rem, 88vw);
  overflow-y: auto;
  border-left: 1px solid var(--color-line);
  border-radius: 0;
}

@media (min-width: 1024px) {
  .article-outline__drawer-toggle,
  .article-outline__backdrop {
    display: none;
  }

  .article-outline__panel,
  .article-outline__panel.is-drawer-open {
    position: static;
    display: block;
    width: auto;
    overflow: visible;
    border-left: 0;
    border-radius: var(--radius-lg);
  }
}
</style>
