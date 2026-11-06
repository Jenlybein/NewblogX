<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { motion, useSpring, useTransform } from "motion-v";

import { useMotionPreference } from "~/composables/useMotionPreference";
import type { OutlineSourceKind } from "~/utils/outlineKind";
import type { DotPoint } from "~/utils/outlineDot";
import { outlineDotPointAt, outlineDotXPx } from "~/utils/outlineDot";
import { markCurrentOutlineRows, visibleOutlineRows } from "~/utils/outlineState";
import type { OutlineNode } from "~/utils/outlineTree";
import { outlineTreeRows } from "~/utils/outlineTree";
import { SPRING_DOT, resolveSpringMotion } from "~/utils/motion";

/**
 * 读者侧目录树（合集 / 项目详情页 + 阅读页抽屉共用）。
 *
 * 只渲染树，不取数：父页面在客户端拿到详情后把 `nodes` 与折叠状态传进来。
 * 文件夹可折叠、文章节点带来源链接、当前文章高亮，层级靠缩进表达（notes A7）。
 * 文章行是**纯文字**：封面缩略图已从三个使用点一起退场。
 *
 * 沟槽圆点（票 295）：一枚装饰性小圆点标出「当前文章」——
 *   - 横向压在该行所属层级的发丝引导线上（`depth × 1.5rem − 0.75rem`，`depth 0` 取 0），
 *     几何全部由 `utils/outlineDot.ts` 的纯函数给出；
 *   - 纵向取该行垂直中心（`offsetTop + 行高 / 2`），所以折叠 / 展开让行位移后要重测；
 *   - 同树内换文章（上一篇/下一篇、点树内文章）与折叠导致位移时沿弧线滑过去：
 *     弹簧（`SPRING_DOT`）驱动 0→1 进度，位置由 `useTransform` 从进度派生（线性插值
 *     + 正弦弧偏移），不依赖 `motion` 包的 `arc()`；
 *   - 首次进入 / 字体就绪时的首测**直接落位**（不播动画），「减少动态」下每次都瞬移；
 *   - `hover` / 键盘聚焦不动它，它也不进 Tab 顺序、不吃点击（装饰层）。
 *
 * 测量一律在客户端：抽屉收起态是 `transform + visibility`（不是 `display: none`），
 * `offsetTop` 仍然可测，所以不必等抽屉展开才落位。
 */
const props = withDefaults(
  defineProps<{
    nodes: OutlineNode[] | null | undefined;
    /** 来源参数里的 kind：决定文章链接的 `?from=`；空串表示来源未知（不给链接）。 */
    kind: OutlineSourceKind | "";
    ownerId: string;
    /** 被收起的文件夹节点 ID（由父页面持久化到 localStorage）。 */
    collapsed: ReadonlySet<string>;
    /** 当前文章 ID（阅读页回跳时用；详情页通常没有 → 不渲染圆点）。 */
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

const currentRow = computed(() => visible.value.find((row) => row.isCurrent) ?? null);
const currentRowId = computed(() => currentRow.value?.id ?? "");

const listRef = ref<HTMLUListElement | null>(null);

/* ------------------------------------------------------------------ 圆点状态 */

const { prefersReducedMotion } = useMotionPreference();

/**
 * 弹簧参数只有 `utils/motion.ts` 一个来源；「减少动态」经 `resolveSpringMotion`
 * 解析成**立即落位**（时长 0 的 tween，不是"慢速弹簧"）。
 */
const dotSpring = computed(() => resolveSpringMotion(SPRING_DOT, prefersReducedMotion.value));

/** 弹簧驱动 0→1：不直接动坐标，只动"这一趟走了多少"。 */
const dotProgress = useSpring(0, dotSpring);

const dotFrom = ref<DotPoint>({ x: 0, y: 0 });
const dotTo = ref<DotPoint>({ x: 0, y: 0 });

/** 进度 → 坐标（线性插值 + 正弦弧偏移），弧只在 x/y 同时变化时出现。 */
const dotX = useTransform(dotProgress, (progress) =>
  outlineDotPointAt(dotFrom.value, dotTo.value, progress).x,
);
const dotY = useTransform(dotProgress, (progress) =>
  outlineDotPointAt(dotFrom.value, dotTo.value, progress).y,
);

const dotVisible = ref(false);
const dotTargetId = ref("");
const dotDepth = ref(0);
/** 最近一次对准的方式：`snap` 落位（首测 / 减少动态）或 `travel` 弹簧滑行。 */
const dotMotion = ref<"snap" | "travel">("snap");

/** 上一次落位坐标：新一趟从"圆点现在在哪"出发，不从上一次的终点重算。 */
let placed: DotPoint | null = null;

/** 已经播过一次弹簧了吗：字体晚到时按普通重对准处理，而不是把圆点拽回去。 */
let hasTraveled = false;

/** 根字号（rem 换算用）：树的缩进是 `rem`，但测量到的是 px。 */
function rootFontSizePx(): number {
  if (typeof window === "undefined") return 16;
  return Number.parseFloat(window.getComputedStyle(document.documentElement).fontSize);
}

/** 当前文章的 `<li>`；找不到说明这一行没渲染（不该发生：当前分支会被强制展开）。 */
function currentRowElement(rowId: string): HTMLElement | null {
  const list = listRef.value;
  if (!list) return null;
  const rows = [...list.querySelectorAll<HTMLElement>("[data-outline-row]")];
  return rows.find((element) => element.dataset.outlineRow === rowId) ?? null;
}

/**
 * 量一次当前文章的位置：横向按层级（纯函数），纵向取行的垂直中心。
 *
 * 取 `offsetTop` 而不是 `getBoundingClientRect()`：抽屉是 `transform` 移出屏幕的，
 * 矩形坐标会带上位移，`offsetTop` 只关心布局位置。
 */
function measureCurrentRow(rowId: string, depth: number): DotPoint | null {
  const element = currentRowElement(rowId);
  if (!element) return null;

  return {
    x: outlineDotXPx(depth, rootFontSizePx()),
    y: element.offsetTop + element.offsetHeight / 2,
  };
}

/**
 * 对准当前文章。
 *
 * @param animate 允许播弹簧；首次落位、字体就绪后的首测与「减少动态」都会落位。
 */
function align(animate: boolean): void {
  const row = currentRow.value;
  if (!row) {
    dotVisible.value = false;
    return;
  }

  const next = measureCurrentRow(row.id, row.depth);
  if (!next) {
    dotVisible.value = false;
    return;
  }

  // 首测（`placed` 还是空）一律落位：入场不该滑一段。
  const isFirstPlacement = placed === null;
  const from = placed ?? next;
  placed = next;
  dotTargetId.value = row.id;
  dotDepth.value = row.depth;
  dotFrom.value = from;
  dotTo.value = next;

  const shouldAnimate = animate && !isFirstPlacement && !prefersReducedMotion.value;
  dotMotion.value = shouldAnimate ? "travel" : "snap";

  if (shouldAnimate) {
    // 同一帧内 jump 回 0 再 set(1)：弹簧从起点跑到终点，中途位置由 useTransform 派生。
    hasTraveled = true;
    dotProgress.jump(0);
    dotProgress.set(1);
  } else {
    dotProgress.jump(1);
  }

  dotVisible.value = true;
}

/**
 * 字体就绪后再落一次位（首测可能量在字体替换之前，行高会变）。
 *
 * 传的是**已经移动过**这个判断本身：还没播过弹簧时直接落位——入场不该因为字体
 * 替换而"滑一下"；已经切换过文章之后字体才就绪，才按普通重对准走弹簧，
 * 不把圆点拽回旧位置。
 */
function alignWhenFontsReady(): void {
  if (typeof document === "undefined") return;
  const fonts = document.fonts;
  if (!fonts?.ready) return;
  void fonts.ready.then(() => align(hasTraveled)).catch(() => undefined);
}

onMounted(() => {
  align(false);
  alignWhenFontsReady();
});

// 同树内换文章：圆点滑到新行（跨层级时 x/y 同时变，走弧线）。
watch(currentRowId, () => align(true), { flush: "post" });

// 折叠 / 展开让行上下位移：重新对准当前行。
watch(() => props.collapsed, () => align(true), { flush: "post" });
</script>

<template>
  <div class="outline-tree" data-outline-tree="true">
    <!-- 沟槽圆点：装饰层，压在当前文章那一层的引导线上；不抢焦点、不吃点击。 -->
    <motion.span
      v-if="dotVisible"
      class="outline-tree__dot"
      aria-hidden="true"
      data-outline-dot="true"
      :data-outline-dot-target="dotTargetId"
      :data-outline-dot-depth="dotDepth"
      :data-outline-dot-motion="dotMotion"
      :style="{ x: dotX, y: dotY }"
    />

    <ul ref="listRef" class="outline-tree__list">
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
          <strong class="outline-tree__label">{{ row.label }}</strong>
        </NuxtLink>
      </li>
    </ul>
  </div>
</template>

<style scoped>
/* 圆点相对这个盒子绝对定位；`ul` 与它同起点，所以行坐标可以直接用 `offsetTop`。 */
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

.outline-tree__item[data-depth]:not([data-depth="0"])::before {
  position: absolute;
  top: 0;
  bottom: 0;
  left: calc(var(--outline-indent) * 1.5rem - 0.75rem);
  width: 1px;
  background: var(--color-line);
  content: "";
}

/*
 * 沟槽圆点：半宽负偏移，让 `translate(x, y)` 落在圆心；
 * 抬一层是因为当前行的记号笔底会盖住它（否则"压线"看不见）。
 */
.outline-tree__dot {
  position: absolute;
  top: -0.25rem;
  left: -0.25rem;
  z-index: 1;
  width: 0.5rem;
  height: 0.5rem;
  border-radius: var(--radius-pill);
  background: var(--color-ink);
  pointer-events: none;
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
</style>
