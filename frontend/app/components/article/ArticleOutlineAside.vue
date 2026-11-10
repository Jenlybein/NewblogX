<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { IconList, IconX } from "@tabler/icons-vue";

import ArticleOutlinePanel from "~/components/article/ArticleOutlinePanel.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import type { ArticleOutlineContext } from "~/utils/outlineContext";
import { outlineContextKind } from "~/utils/outlineContext";

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
// 面板眉题只表达"这棵树属于谁"：语义与 `outlineKind` 的 owner_kind 口径一致，
// 未知值不猜归属，退回通用的 Outline。
const ownerEyebrow = computed(() => {
  switch (outlineContextKind(props.context)) {
    case "collection":
      return "Collection";
    case "project":
      return "Project";
    default:
      return "Outline";
  }
});

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
      <span class="outline-rail__tab-mark" aria-hidden="true">
        <IconList :size="15" :stroke-width="1.8" />
      </span>
      <span class="outline-rail__tab-label">目录</span>
    </button>

    <!-- 遮罩只负责"点外面关掉"，不做成暗色蒙版：读者常常是边看正文边找章节。 -->
    <Transition name="outline-scrim">
      <button
        v-if="open"
        type="button"
        class="outline-rail__scrim"
        aria-label="关闭目录"
        tabindex="-1"
        data-outline-drawer-backdrop
        @click="close(true)"
      />
    </Transition>

    <aside
      id="article-outline-panel"
      class="article-paper outline-rail__panel"
      :class="{ 'is-drawer-open': open }"
      :aria-hidden="open ? 'false' : 'true'"
      :inert="open ? undefined : true"
      data-outline-panel
    >
      <div class="outline-rail__head">
        <div class="outline-rail__heading-block">
          <p class="article-eyebrow">
            <span class="article-eyebrow__mark" aria-hidden="true" />
            {{ ownerEyebrow }}
          </p>
          <h2 class="outline-rail__heading">目录</h2>
        </div>
        <button
          type="button"
          class="outline-rail__close"
          aria-label="收起目录"
          data-outline-close
          @click="close(true)"
        >
          <IconX :size="16" :stroke-width="1.8" />
        </button>
      </div>

      <div class="outline-rail__body">
        <div v-if="pending" data-role="article-outline-skeleton">
          <LoadingSkeleton :rows="8" variant="text" label="文章目录正在加载" />
        </div>
        <ArticleOutlinePanel
          v-else-if="context && hasTree"
          :key="panelKey"
          :context="context"
          :current-article-id="currentArticleId"
        />
      </div>
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

/* 竖排窄标：贴左边缘、垂直居中，读作一页被折进去的书签。 */
.outline-rail__tab {
  position: absolute;
  top: 43%;
  left: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.55rem;
  padding: 0.65rem 0.42rem 0.8rem;
  border: 1px solid var(--color-line);
  border-left: 0;
  border-radius: 0 var(--radius-lg) var(--radius-lg) 0;
  background: var(--color-surface);
  color: var(--color-ink-muted);
  font-family: var(--font-title);
  font-size: 0.8125rem;
  cursor: pointer;
  pointer-events: auto;
  box-shadow: 0 18px 34px -26px var(--color-shadow);
  transition:
    color var(--motion-fast) var(--motion-ease),
    background var(--motion-fast) var(--motion-ease),
    box-shadow var(--motion-base) var(--motion-ease),
    transform var(--motion-base) var(--motion-ease);
}

.outline-rail__tab:hover {
  color: var(--color-ink);
  background: var(--color-surface-raised);
  box-shadow: 0 22px 42px -24px var(--color-shadow);
  transform: translateX(2px);
}

/* 窄标上的小色块：与面板眉题里的记号块同一语义，告诉读者"里面是一份目录"。 */
.outline-rail__tab-mark {
  display: grid;
  place-items: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: var(--radius-sm);
  background: var(--color-highlight);
  color: var(--color-ink);
  line-height: 1;
}

.outline-rail__tab-label {
  writing-mode: vertical-rl;
  letter-spacing: 0.18em;
}

/*
 * 面板：从左侧滑出，压住正文但不改布局。顶部一层很轻的抬升渐变，
 * 让"目录"区先亮出来；正文面本身仍是不透明的纸面（玻璃只用在这层抽屉上）。
 */
.outline-rail__panel {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  display: flex;
  flex-direction: column;
  width: min(22rem, 92vw);
  padding: 0;
  overflow: hidden;
  border-radius: 0 var(--radius-xl) var(--radius-xl) 0;
  border-left: 0;
  background: linear-gradient(180deg, var(--color-surface-raised), var(--color-surface) 10rem);
  box-shadow:
    0 34px 80px -36px var(--color-shadow),
    1px 0 0 var(--color-line-faint);
  transform: translateX(-101%);
  visibility: hidden;
  pointer-events: auto;
  transition:
    transform var(--motion-base) var(--motion-ease),
    visibility var(--motion-base) var(--motion-ease);
}

.outline-rail__panel.is-drawer-open {
  transform: none;
  visibility: visible;
}

/*
 * 遮罩只负责点击关闭；淡入比"啪地出现"更像一层纸。
 * 取**页面底色**而不是墨色：亮色下是一层暖纸，暗色下自然变暗；
 * 用 ink 会在暗色主题里把页面提亮成灰雾（旧实现的观感问题）。
 */
.outline-rail__scrim {
  position: absolute;
  inset: 0;
  border: 0;
  background: color-mix(in srgb, var(--color-bg-page) 55%, transparent);
  cursor: default;
  pointer-events: auto;
}

.outline-scrim-enter-active,
.outline-scrim-leave-active {
  transition: opacity var(--motion-fast) var(--motion-ease);
}

.outline-scrim-enter-from,
.outline-scrim-leave-to {
  opacity: 0;
}

/* 头部固定，目录本体单独滚动：树再长，"目录"两个字也不会跟着滚走。 */
.outline-rail__head {
  display: flex;
  flex: 0 0 auto;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1.15rem 1.05rem 0.85rem 1.25rem;
  border-bottom: 1px solid var(--color-line-faint);
}

.outline-rail__heading-block {
  min-width: 0;
}

.outline-rail__heading {
  margin: 0.3rem 0 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.375rem;
  font-weight: 650;
  letter-spacing: -0.02em;
  line-height: 1.15;
}

.outline-rail__close {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-ink-muted);
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    background var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.outline-rail__close:hover {
  border-color: var(--color-line);
  background: var(--color-surface);
  color: var(--color-ink);
}

.outline-rail__body {
  flex: 1 1 auto;
  min-height: 0;
  padding: 0.35rem 0.9rem 1.4rem 1.15rem;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.outline-rail :is(button, a):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .outline-rail__panel,
  .outline-rail__tab {
    transition: none;
  }

  .outline-rail__tab:hover {
    transform: none;
  }

  .outline-scrim-enter-active,
  .outline-scrim-leave-active {
    transition: none;
  }
}
</style>
