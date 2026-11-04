<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";

import ArticleOutlinePanel from "~/components/article/ArticleOutlinePanel.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import type { ArticleOutlineContext } from "~/utils/outlineContext";

/**
 * 阅读页目录抽屉（票 220 的左栏，2026-11-04 验收改为「贴左边缘的抽屉」）。
 *
 * 为什么不再占一列：原来宽屏是 `.article-layout` 的**第一列**（13.5rem）。那等于用正文
 * 的宽度换目录——读者反馈"目录把正文挤了、两边还脱节"。现在目录**不参与文章布局**：
 *
 *   - 收起：窗口左边缘只剩一条竖排窄标（`≡ 目录`），正文完全不受影响；
 *   - 展开：面板从左侧滑出，**覆盖**在正文之上（宽 `min(20rem, 86vw)`），并压一层薄遮罩；
 *   - 关闭：点遮罩、按 Esc、或再点一次窄标。焦点回到窄标上。
 *
 * 三种状态只影响面板内容，不影响抽屉本身：`pending` 出同构骨架、`ready` 渲染树、
 * 无归属树时整个组件不渲染（`outlineColumnVisible` 在页面里已经判过）。
 */
const props = defineProps<{
  context: ArticleOutlineContext | null;
  pending: boolean;
  currentArticleId: string;
}>();

const hasTree = computed(() => props.context?.has_tree === true);
// 组件自己也要守住"无归属不渲染"：页面虽然已用 outlineColumnVisible 判过一次，
// 但组件被别处复用时不能凭空长出一条空的目录窄标。
const visible = computed(() => props.pending || hasTree.value);
const panelState = computed(() => (props.pending ? "pending" : "ready"));
// 切树（合集/项目换了 owner）时重建面板：折叠状态按树存 localStorage。
const panelKey = computed(() => `${props.context?.owner_kind ?? 0}:${props.context?.owner_id ?? ""}`);

const open = ref(false);
const tabRef = ref<HTMLButtonElement | null>(null);

function toggle() {
  open.value = !open.value;
}

function close(returnFocus = false) {
  open.value = false;
  if (returnFocus) {
    tabRef.value?.focus();
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    close(true);
  }
}

// 用 typeof window 而不是 import.meta.client：后者是 Nuxt 的编译期标记，
// 在纯 vitest 环境里是 undefined，会让 Esc 监听永远不挂上（测试也验不到）。
watch(open, (isOpen) => {
  if (typeof window === "undefined") {
    return;
  }
  if (isOpen) {
    window.addEventListener("keydown", handleKeydown);
  } else {
    window.removeEventListener("keydown", handleKeydown);
  }
});

onBeforeUnmount(() => {
  if (typeof window !== "undefined") {
    window.removeEventListener("keydown", handleKeydown);
  }
});
</script>

<template>
  <div
    v-if="visible"
    class="outline-rail"
    data-article-outline="true"
    :data-state="panelState"
    :data-open="open ? 'true' : 'false'"
  >
    <!-- 收起态：贴在窗口左边缘的竖排窄标（不占布局宽度，悬浮在正文之上）。 -->
    <button
      ref="tabRef"
      type="button"
      class="outline-rail__tab"
      :aria-expanded="open ? 'true' : 'false'"
      aria-controls="article-outline-panel"
      aria-label="目录（展开或收起）"
      data-outline-drawer-toggle
      @click="toggle"
    >
      <span class="outline-rail__tab-mark" aria-hidden="true">≡</span>
      <span class="outline-rail__tab-label">目录</span>
    </button>

    <!-- 遮罩只负责"点外面关掉"，不做成暗色蒙版：读者常常是边看正文边找章节。 -->
    <button
      v-if="open"
      type="button"
      class="outline-rail__scrim"
      aria-label="关闭目录"
      data-outline-drawer-backdrop
      @click="close(true)"
    />

    <aside
      id="article-outline-panel"
      class="article-paper outline-rail__panel"
      :class="{ 'is-drawer-open': open }"
      :aria-hidden="open ? 'false' : 'true'"
      :inert="open ? undefined : true"
      data-outline-panel
    >
      <div class="outline-rail__head">
        <p class="article-eyebrow">
          <span class="article-eyebrow__mark" aria-hidden="true" />
          Outline
        </p>
        <button
          type="button"
          class="outline-rail__close"
          aria-label="收起目录"
          data-outline-close
          @click="close(true)"
        >
          ×
        </button>
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
    </aside>
  </div>
</template>

<style scoped>
/* 一整层只吃"边缘"与"面板"两处的点击：正文其余位置照旧可读可选中。 */
.outline-rail {
  position: fixed;
  inset: 0;
  z-index: 46;
  pointer-events: none;
}

/* 竖排窄标：贴左边缘、垂直居中，宽 2rem 左右，不遮正文的有效内容区。 */
.outline-rail__tab {
  position: absolute;
  top: 46%;
  left: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 0.4rem;
  border: 1px solid var(--color-line);
  border-left: 0;
  border-radius: 0 var(--radius-md) var(--radius-md) 0;
  background: var(--color-surface);
  color: var(--color-ink-muted);
  font-family: var(--font-title);
  font-size: 0.8125rem;
  cursor: pointer;
  pointer-events: auto;
  transition: color var(--motion-fast) var(--motion-ease), background var(--motion-fast) var(--motion-ease);
}

.outline-rail__tab:hover {
  color: var(--color-ink);
  background: var(--color-surface-raised);
}

.outline-rail__tab-mark {
  font-family: var(--font-mono);
  font-size: 0.875rem;
  line-height: 1;
}

.outline-rail__tab-label {
  writing-mode: vertical-rl;
  letter-spacing: 0.18em;
}

/* 面板：从左侧滑出，压住正文但不改布局。 */
.outline-rail__panel {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: min(20rem, 86vw);
  padding: 1rem 1.1rem 1.25rem;
  overflow-y: auto;
  border-radius: 0 var(--radius-lg) var(--radius-lg) 0;
  border-left: 0;
  background: var(--color-surface);
  box-shadow: var(--shadow-lg);
  transform: translateX(-101%);
  visibility: hidden;
  pointer-events: auto;
  transition: transform var(--motion-base) var(--motion-ease), visibility var(--motion-base) var(--motion-ease);
}

.outline-rail__panel.is-drawer-open {
  transform: none;
  visibility: visible;
}

.outline-rail__scrim {
  position: absolute;
  inset: 0;
  border: 0;
  background: color-mix(in srgb, var(--color-ink) 18%, transparent);
  cursor: default;
  pointer-events: auto;
}

.outline-rail__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}

.outline-rail__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-ink-muted);
  font-size: 1.05rem;
  line-height: 1;
  cursor: pointer;
}

.outline-rail__close:hover {
  border-color: var(--color-line);
  color: var(--color-ink);
}

@media (prefers-reduced-motion: reduce) {
  .outline-rail__panel,
  .outline-rail__tab {
    transition: none;
  }
}
</style>
