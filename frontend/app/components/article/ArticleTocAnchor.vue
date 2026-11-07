<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { NAnchor, NAnchorLink } from "naive-ui";
import ArticleTocRail from "~/components/article/ArticleTocRail.vue";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { whenFontsReady } from "~/utils/fontsReady";
import { formatProgressPercent, resolveTocFollow, tocIndentLevel } from "~/utils/toc";
import { buildTocRailNodes, tocRailLabelInsetPx, tocRailRowCenterPx } from "~/utils/tocRail";

/**
 * 右栏标题大纲（票 296：轨道 + 飞行器）。
 *
 * 分两层，互不越界：
 *   1. **行为层**仍是 naive `NAnchor`（`bound=96` 的滚动绑定、当前项判定、aria、
 *      键盘），`ArticleTocRail` 只叠在它左边当视觉层；
 *   2. **视觉层**吃两样东西：标题的层级（缩进）与每行的实际 y（测量），几何由
 *      `utils/tocRail` 的纯函数算。
 *
 * 对齐只有两条算式，且都出自 `utils/tocRail`：
 *   - 纵向：节点 y = 行盒的**垂直中心**（`tocRailRowCenterPx`），不取行顶/行底；
 *   - 横向：文字 `padding-left` = 节点 x + 14px（`tocRailLabelInsetPx`），层级只改
 *     节点 x，间距恒定。CSS 里不再另写一套缩进，避免两套数字互相拉扯。
 *
 * 左侧那一列只有轨道：激活态靠字重 + `--color-ink` 表达，不再画竖条（naive 的
 * `border-left` 已从 `.article-toc-link` 上撤掉，它也压不到轨道那一列）。
 *
 * 卡头的进度条与百分比**保留**：它们与轨道的已读填充是刻意的双表达（用户明确选的
 * 口径），不要因为"重复"把它们删掉。
 */

const props = defineProps<{
  headings: ArticleHeadingAnchor[];
  activeHeadingId: string;
  progressPercent: number;
}>();

const emit = defineEmits<{
  jump: [id: string];
}>();

const { prefersReducedMotion } = useMotionPreference();

const tocBodyRef = ref<HTMLElement | null>(null);
/** 轨道与锚点共用的定位容器：轨道贴着它绝对定位，实测 y 也相对它取。 */
const plotRef = ref<HTMLElement | null>(null);

/**
 * 目录是否处于"宽屏侧栏"布局。
 *
 * 与侧栏 `position: sticky → static` 的断点保持一致（1024px）：窄屏下侧栏退化为
 * 静态、目录掉到文档底部，此时任何"滚动到可见"都会把整页往下拽。
 */
const TOC_SIDEBAR_MIN_WIDTH = 1024;
const tocVisible = ref(true);
let mediaQuery: MediaQueryList | null = null;

function syncTocVisible() {
  tocVisible.value = window.innerWidth >= TOC_SIDEBAR_MIN_WIDTH;
}

/**
 * 程序化滚动的抑制窗口。
 *
 * 自己刚滚过就别再跟着滚：否则"滚动 → 高亮变化 → 又滚动"会形成闭环。
 * 用时间戳而不是布尔量，是因为滚动事件可能连续到达，需要一个衰减窗口。
 */
const SUPPRESS_WINDOW_MS = 120;
let lastProgrammaticScrollAt = 0;

function isSuppressed() {
  return Date.now() - lastProgrammaticScrollAt < SUPPRESS_WINDOW_MS;
}

/**
 * 每个标题行的中心 y（相对轨道容器顶部）。
 *
 * 量不到（SSR、首帧、字体未就绪、行高为 0）就留空数组，`buildTocRailNodes` 会
 * 按等距兜底 —— 首屏立刻有一条完整轨道，量到真值后再校准，不会先闪一坨叠点。
 */
const rowCenters = ref<(number | null)[]>([]);

function measureRows() {
  const plot = plotRef.value;
  if (!plot || props.headings.length === 0) {
    rowCenters.value = [];
    return;
  }

  const origin = plot.getBoundingClientRect().top;
  rowCenters.value = props.headings.map((heading) => {
    const link = plot.querySelector<HTMLElement>(`[data-heading-id="${heading.id}"]`);
    if (!link) {
      return null;
    }

    const rect = link.getBoundingClientRect();
    // 行盒的中心，不是行顶：节点与文字的中线对齐（算式在 utils/tocRail 里，测试直接钉它）。
    return rect.height > 0 ? tocRailRowCenterPx(rect.top, rect.height, origin) : null;
  });
}

const railNodes = computed(() => buildTocRailNodes(props.headings, rowCenters.value));

/**
 * 「钉住」：点完标题的这段时间里，飞机停在点中的节点上，不跟着平滑滚动的途中高亮跑。
 *
 * 解除条件是**读者自己滚动**（wheel / 触摸 / 方向键这类主动滚动手势），而不是
 * "任意 scroll 事件"：点击触发的平滑滚动本身也是一串 scroll 事件，按 scroll 解钉
 * 等于刚钉上就松开。
 */
const pinnedId = ref("");

const USER_SCROLL_KEYS = new Set([
  "ArrowUp",
  "ArrowDown",
  "PageUp",
  "PageDown",
  "Home",
  "End",
  " ",
  "Spacebar",
]);

function releasePin() {
  pinnedId.value = "";
}

function handleUserKeydown(event: KeyboardEvent) {
  if (USER_SCROLL_KEYS.has(event.key)) {
    releasePin();
  }
}

/** 钉住的目标若已不在标题表里（换文章），退回跟随当前高亮，别把飞机钉在虚空里。 */
const railActiveId = computed(() => {
  const pinned = pinnedId.value;
  if (pinned && props.headings.some((heading) => heading.id === pinned)) {
    return pinned;
  }

  return props.activeHeadingId;
});

function handleJump(id: string) {
  emit("jump", id);
  pinnedId.value = id;
}

watch(
  () => props.activeHeadingId,
  async (nextId) => {
    if (!nextId) {
      return;
    }

    await nextTick();
    const body = tocBodyRef.value;
    const currentLink = body?.querySelector<HTMLElement>(`[data-heading-id="${nextId}"]`);
    if (!body || !currentLink) {
      return;
    }

    // 只算目录容器自己的 scrollTop，绝不调用会滚动整页的 API。
    // 该 API 会滚动所有祖先滚动容器（含 document），这正是窄屏"页面自己下跳"
    // 的根因：宽屏侧栏 sticky 时被掩盖，窄屏侧栏变静态后就开始拽整页。
    const decision = resolveTocFollow({
      visible: tocVisible.value,
      suppressed: isSuppressed(),
      offsetTop: currentLink.offsetTop,
      scrollTop: body.scrollTop,
      clientHeight: body.clientHeight,
      targetHeight: currentLink.offsetHeight,
    });

    if (!decision.follow || decision.scrollTop === null) {
      return;
    }

    lastProgrammaticScrollAt = Date.now();
    body.scrollTop = decision.scrollTop;
  },
  { flush: "post" },
);

watch(
  () => props.headings,
  async () => {
    await nextTick();
    measureRows();
  },
  { flush: "post" },
);

function handleResize() {
  syncTocVisible();
  measureRows();
}

/** 字体晚到会改行高：就绪后再量一次（SSR 守卫与目录树的圆点共用同一个工具）。 */
function measureAfterFonts() {
  whenFontsReady(measureRows);
}

onMounted(() => {
  syncTocVisible();
  measureRows();
  measureAfterFonts();
  mediaQuery = window.matchMedia(`(min-width: ${TOC_SIDEBAR_MIN_WIDTH}px)`);
  mediaQuery.addEventListener("change", syncTocVisible);
  window.addEventListener("resize", handleResize);
  window.addEventListener("wheel", releasePin, { passive: true });
  window.addEventListener("touchmove", releasePin, { passive: true });
  window.addEventListener("keydown", handleUserKeydown);
});

onBeforeUnmount(() => {
  mediaQuery?.removeEventListener("change", syncTocVisible);
  window.removeEventListener("resize", handleResize);
  window.removeEventListener("wheel", releasePin);
  window.removeEventListener("touchmove", releasePin);
  window.removeEventListener("keydown", handleUserKeydown);
  mediaQuery = null;
});
</script>

<template>
  <section
    v-if="headings.length && tocVisible"
    class="article-paper article-toc"
    :data-toc-pinned="pinnedId ? 'true' : 'false'"
  >
    <div class="article-toc__head">
      <p class="article-eyebrow"><span class="article-eyebrow__mark" aria-hidden="true" />Contents</p>
      <h2 class="article-toc__title">目录</h2>
      <span class="article-toc__progress">{{ formatProgressPercent(progressPercent) }}</span>
    </div>

    <div
      class="article-reading-progress"
      role="progressbar"
      aria-label="阅读进度"
      :aria-valuemin="0"
      :aria-valuemax="100"
      :aria-valuenow="Math.round(progressPercent)"
    >
      <span class="article-reading-progress__fill" :style="{ width: `${progressPercent}%` }" />
    </div>

    <nav ref="tocBodyRef" class="article-toc__body" aria-label="文章目录">
      <div ref="plotRef" class="article-toc__plot">
        <!-- 视觉层：轨道 + 节点 + 飞机。纯装饰，读屏与键盘都走下面的锚点。 -->
        <div class="article-toc__rail">
          <ArticleTocRail
            :nodes="railNodes"
            :active-id="railActiveId"
            :prefers-reduced-motion="prefersReducedMotion"
          />
        </div>

        <NAnchor :bound="96" :show-rail="false" type="block">
          <NAnchorLink
            v-for="heading in headings"
            :key="heading.id"
            :href="heading.href"
            :title="heading.title"
            :data-heading-id="heading.id"
            :class="['article-toc-link', { 'is-active': activeHeadingId === heading.id }]"
            :style="{ paddingLeft: `${tocRailLabelInsetPx(tocIndentLevel(heading.level))}px` }"
            @click.prevent="handleJump(heading.id)"
          />
        </NAnchor>
      </div>
    </nav>
  </section>
</template>

<style scoped>
.article-toc__plot {
  position: relative;
}

/* 轨道占左侧一条窄带（画布 44px，最深的节点在 x=33）；锚点不整体右移——
   文字的内边距由 `tocRailLabelInsetPx` 逐行给，轨道与文字共用同一套 x。 */
.article-toc__rail {
  position: absolute;
  top: 0;
  left: 0;
  pointer-events: none;
}
</style>
