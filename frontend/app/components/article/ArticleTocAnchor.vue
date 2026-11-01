<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { NAnchor, NAnchorLink } from "naive-ui";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { formatProgressPercent, resolveTocFollow, tocIndentLevel } from "~/utils/toc";

const props = defineProps<{
  headings: ArticleHeadingAnchor[];
  activeHeadingId: string;
  progressPercent: number;
}>();

const emit = defineEmits<{
  jump: [id: string];
}>();

const tocBodyRef = ref<HTMLElement | null>(null);

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

function handleJump(id: string) {
  emit("jump", id);
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

onMounted(() => {
  syncTocVisible();
  mediaQuery = window.matchMedia(`(min-width: ${TOC_SIDEBAR_MIN_WIDTH}px)`);
  mediaQuery.addEventListener("change", syncTocVisible);
  window.addEventListener("resize", syncTocVisible);
});

onBeforeUnmount(() => {
  mediaQuery?.removeEventListener("change", syncTocVisible);
  window.removeEventListener("resize", syncTocVisible);
  mediaQuery = null;
});
</script>

<template>
  <section v-if="headings.length && tocVisible" class="article-paper article-toc">
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
      <NAnchor :bound="96" :show-rail="false" type="block">
        <NAnchorLink
          v-for="heading in headings"
          :key="heading.id"
          :href="heading.href"
          :title="heading.title"
          :data-heading-id="heading.id"
          :class="[
            'article-toc-link',
            `article-toc-link--indent-${tocIndentLevel(heading.level)}`,
            { 'is-active': activeHeadingId === heading.id },
          ]"
          @click.prevent="handleJump(heading.id)"
        />
      </NAnchor>
    </nav>
  </section>
</template>
