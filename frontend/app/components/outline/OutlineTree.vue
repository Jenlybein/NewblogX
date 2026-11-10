<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { IconChevronRight, IconFileText, IconFolder, IconFolderOpen } from "@tabler/icons-vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import type { OutlineSourceKind } from "~/utils/outlineKind";
import { markCurrentOutlineRows, visibleOutlineRows } from "~/utils/outlineState";
import type { OutlineNode } from "~/utils/outlineTree";
import { outlineTreeRows } from "~/utils/outlineTree";
import { staggerDelayMs } from "~/utils/motion";

/**
 * 读者侧目录树（合集 / 项目详情页 + 阅读页抽屉共用）。
 *
 * 只渲染树，不取数：父页面在客户端拿到详情后把 `nodes` 与折叠状态传进来。
 * 文件夹可折叠、文章节点带来源链接、当前文章高亮，层级靠缩进表达（notes A7）。
 * 文章行是**纯文字**：封面缩略图已从三个使用点一起退场。
 *
 * 当前文章**不再强行掰开它所在的文件夹**（2026-11-10 按反馈修改）：读者收起哪一层
 * 就保持收起，当前行跟着隐藏；只有可见时才打 `is-current`。这也让「当前文章所在
 * 文件夹」和普通文件夹共用同一套开关，没有第二份只能展开不能折叠的分支。
 *
 * 展开时新行按 40ms 错峰入场；折叠不播退场（列表收拢比淡出更直接）。
 */
const props = withDefaults(
  defineProps<{
    nodes: OutlineNode[] | null | undefined;
    /** 来源参数里的 kind：决定文章链接的 `?from=`；空串表示来源未知（不给链接）。 */
    kind: OutlineSourceKind | "";
    ownerId: string;
    /** 被收起的文件夹节点 ID（由父页面持久化到 localStorage）。 */
    collapsed: ReadonlySet<string>;
    /** 当前文章 ID（阅读页回跳时用；详情页通常没有 → 不高亮）。 */
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
    visibleOutlineRows(allRows.value, props.collapsed),
    props.currentArticleId,
  ),
);

const { prefersReducedMotion } = useMotionPreference();

/* ------------------------------------------------------------ 展开时的新行入场 */

/**
 * 刚刚展开的那个文件夹带出来的行 → 入场顺序（0 起，最多错峰 7 步）。
 *
 * 折叠不播退场：行从列表里消失时列表本来就会立刻收拢，补一段"淡出"反而让
 * 下面几行先停一下再跳上去（两段动效互相打架）。展开则相反：空间先让出来，
 * 新行按 40ms 的节奏逐条落位，读者能看清"这些是刚打开的"。
 *
 * 只看可见行的差集，不关心是谁触发的：点文件夹展开、换树导致整批替换，都走
 * 同一套入场；首屏没有差异，因此不播。
 */
const enteringIndex = ref(new Map<string, number>());
// 首帧已经存在的行不算"新行"：入场批次从第一次可见行变化开始算。
let previousVisibleIds = new Set(visible.value.map((row) => row.id));

watch(
  visible,
  (rows) => {
    const next = new Map<string, number>();
    let order = 0;
    for (const row of rows) {
      if (!previousVisibleIds.has(row.id)) {
        next.set(row.id, Math.min(order, 7));
        order += 1;
      }
    }
    enteringIndex.value = next;
    previousVisibleIds = new Set(rows.map((row) => row.id));
  },
  // 在组件更新前算好：新行第一帧就带 `is-entering`，否则会先以最终样式闪一帧再重播。
  { flush: "pre" },
);

function isEntering(id: string): boolean {
  return enteringIndex.value.has(id);
}

function enterDelay(id: string): string {
  return `${staggerDelayMs(enteringIndex.value.get(id) ?? 0, prefersReducedMotion.value)}ms`;
}
</script>

<template>
  <div class="outline-tree" data-outline-tree="true">
    <ul class="outline-tree__list">
      <li
        v-for="row in visible"
        :key="row.id"
        class="outline-tree__item"
        :class="{ 'is-current': row.isCurrent, 'is-entering': isEntering(row.id) }"
        :data-outline-row="row.id"
        :data-depth="row.depth"
        :data-outline-kind="row.isFolder ? 'folder' : 'article'"
        :style="{ '--outline-indent': row.depth, '--outline-enter-delay': enterDelay(row.id) }"
        :aria-current="row.isCurrent ? 'page' : undefined"
      >
        <button
          v-if="row.isFolder"
          type="button"
          class="outline-tree__row outline-tree__toggle"
          :aria-expanded="!collapsed.has(row.id)"
          :aria-label="`${collapsed.has(row.id) ? '展开' : '收起'} ${row.label}`"
          :data-outline-toggle="row.id"
          @click="emit('toggle', row.id)"
        >
          <span class="outline-tree__chevron" aria-hidden="true">
            <IconChevronRight :size="14" :stroke-width="1.9" />
          </span>
          <span class="outline-tree__icon outline-tree__icon--folder" aria-hidden="true">
            <span class="outline-tree__folder-glyph" :class="{ 'is-open': !collapsed.has(row.id) }">
              <IconFolder class="outline-tree__folder-closed" :size="16" :stroke-width="1.7" />
              <IconFolderOpen class="outline-tree__folder-open" :size="16" :stroke-width="1.7" />
            </span>
          </span>
          <span class="outline-tree__label">{{ row.label }}</span>
        </button>
        <span v-else-if="!row.href" class="outline-tree__row outline-tree__folder-label">
          <span class="outline-tree__chevron outline-tree__chevron--empty" aria-hidden="true" />
          <span class="outline-tree__icon outline-tree__icon--article" aria-hidden="true">
            <IconFileText :size="16" :stroke-width="1.7" />
          </span>
          <span class="outline-tree__label">{{ row.label }}</span>
        </span>
        <NuxtLink v-else :to="row.href" class="outline-tree__row outline-tree__article" :data-outline-href="row.href">
          <span class="outline-tree__chevron outline-tree__chevron--empty" aria-hidden="true" />
          <span class="outline-tree__icon outline-tree__icon--article" aria-hidden="true">
            <IconFileText :size="16" :stroke-width="1.7" />
          </span>
          <strong class="outline-tree__label">{{ row.label }}</strong>
        </NuxtLink>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.outline-tree {
  position: relative;
}

.outline-tree__list {
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

/* 引导线跨过行距（上下各多探 2px），相邻两行的线在缝里接上，读成一条连续的树线。 */
.outline-tree__item[data-depth]:not([data-depth="0"])::before {
  position: absolute;
  top: -0.125rem;
  bottom: -0.125rem;
  left: calc(var(--outline-indent) * 1.5rem - 0.75rem);
  width: 1px;
  background: var(--color-line);
  content: "";
}

/* 展开时新行入场：容器先让出空间，条目按 40ms 一档落位（参考组件的 child stagger）。 */
.outline-tree__item.is-entering {
  animation: outline-tree-row-in var(--motion-base) var(--motion-ease) both;
  animation-delay: var(--outline-enter-delay, 0ms);
}

@keyframes outline-tree-row-in {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }

  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* 三种行共用同一套盒模型：文件夹按钮、无来源文章、可点文章。 */
.outline-tree__row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  min-height: 2.75rem;
  padding: 0.5rem 0.75rem;
  border: 0;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-ink);
  text-align: left;
  text-decoration: none;
  transition:
    background var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.outline-tree__row:hover {
  background: var(--color-bg-subtle);
}

/* 展开箭头：只有文件夹有；空槽占位让文章与文件夹的图标列对齐。 */
.outline-tree__chevron {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 1rem;
  height: 1rem;
  color: var(--color-ink-faint);
  transition:
    color var(--motion-fast) var(--motion-ease),
    transform var(--motion-base) var(--motion-ease);
}

.outline-tree__toggle[aria-expanded="true"] .outline-tree__chevron {
  color: var(--color-ink-muted);
  transform: rotate(90deg);
}

.outline-tree__chevron--empty {
  visibility: hidden;
}

/* 图标列固定 1.25rem：文件夹换形时不挤动文字。 */
.outline-tree__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 1.25rem;
  height: 1.25rem;
  color: var(--color-ink-faint);
  transition: color var(--motion-fast) var(--motion-ease);
}

.outline-tree__icon--folder {
  color: var(--color-ink-muted);
}

.outline-tree__toggle:hover .outline-tree__icon--folder {
  color: var(--color-ink);
}

/* 文件夹开合：两张图标叠在同一个格子里交叉淡入，不做尺寸动画。 */
.outline-tree__folder-glyph {
  position: relative;
  display: block;
  width: 1rem;
  height: 1rem;
}

.outline-tree__folder-glyph > svg {
  position: absolute;
  inset: 0;
  transition:
    opacity var(--motion-base) var(--motion-ease),
    transform var(--motion-base) var(--motion-ease);
}

.outline-tree__folder-open {
  opacity: 0;
  transform: scale(0.84);
}

.outline-tree__folder-glyph.is-open .outline-tree__folder-open {
  opacity: 1;
  transform: scale(1);
}

.outline-tree__folder-glyph.is-open .outline-tree__folder-closed {
  opacity: 0;
  transform: scale(0.84);
}

.outline-tree__label {
  min-width: 0;
  overflow-wrap: anywhere;
}

.outline-tree__toggle .outline-tree__label {
  font-family: var(--font-title);
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.005em;
  line-height: 1.45;
}

.outline-tree__folder-label {
  color: var(--color-ink-muted);
  font-size: 0.875rem;
}

.outline-tree__article .outline-tree__label {
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.5;
  transition: color var(--motion-fast) var(--motion-ease);
}

.outline-tree__article:hover .outline-tree__label {
  color: var(--color-ink);
  text-decoration: underline;
  text-underline-offset: 0.18em;
}

/* 当前文章：记号笔底铺在「行」而不是整条缩进上，缩进留白保持干净。 */
.outline-tree__item.is-current .outline-tree__row {
  background: var(--color-highlight);
}

.outline-tree__item.is-current .outline-tree__icon--article {
  color: var(--color-ink);
}

/* 显式带上 `a` 前缀：链接色只允许用在文字链接上（设计门禁按选择器判定）。 */
.outline-tree__item.is-current > a.outline-tree__article .outline-tree__label {
  color: var(--color-link);
  font-weight: 600;
}

.outline-tree :is(a, button):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* 减少动态：入场直接落位，箭头与图标换形不做过渡（全局兜底之外再显式声明）。 */
@media (prefers-reduced-motion: reduce) {
  .outline-tree__item.is-entering {
    animation: none;
  }

  .outline-tree__chevron,
  .outline-tree__icon,
  .outline-tree__folder-glyph > svg,
  .outline-tree__row,
  .outline-tree__article .outline-tree__label {
    transition: none;
  }
}
</style>
