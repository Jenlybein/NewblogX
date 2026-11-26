<script setup lang="ts">
import { computed, ref, watch } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import type { SearchArticleItem } from "~/types/api";
import { homeSearchLocation, resolveHomeSectionState } from "~/utils/home";
import { MOTION_DURATIONS_MS, MOTION_EASE, motionSeconds } from "~/utils/motion";

const props = withDefaults(
  defineProps<{
    articles: SearchArticleItem[];
    pending?: boolean;
    failed?: boolean;
    eyebrow?: string;
    title?: string;
  }>(),
  { pending: false, failed: false, eyebrow: "Latest", title: "最新文章" },
);

const emit = defineEmits<{ retry: [] }>();

const { prefersReducedMotion } = useMotionPreference();

/**
 * 列表的动效规则（票 443）：
 *
 *   1. **首屏那一批零入场** —— 不错峰、不淡入，直接可见。信息流是页面正文，而任何
 *      「SSR 内联 opacity: 0、等水合再入场」的写法都会让正文在弱网下空白，等于把
 *      站点能不能看押在 JS 上（上一版同屏第十条约 0.56s 才完全可见）。
 *   2. **翻页后的新批次只做一次短淡入** —— 那是「用户操作之后才出现」的内容，
 *      120ms、从 0.35 起步：文字始终可读，不留空白帧，也不排队拖慢。
 *
 * 用批次 id 的序列当身份，区分「翻页换了内容」与「同一批数据的原地刷新」。
 */
const firstBatchKey = ref<string | null>(null);
const isFirstBatch = ref(true);

watch(
  () => JSON.stringify(props.articles.map((article) => article.id)),
  (batchKey) => {
    if (batchKey === "[]") {
      return;
    }
    if (firstBatchKey.value === null) {
      firstBatchKey.value = batchKey;
      return;
    }
    if (batchKey !== firstBatchKey.value) {
      isFirstBatch.value = false;
    }
  },
  { immediate: true },
);

/**
 * 条目的入场 props。
 *
 * 首批：`initial` 就是可见状态、时长 0 —— 等价于「没有动画」，但保持 Motion 的
 * 结构统一（SSR 与客户端 markup 一致，不产生属性差异）。后续批次：短淡入。
 */
function entranceProps() {
  if (isFirstBatch.value) {
    return {
      initial: { opacity: 1, y: 0 },
      animate: { opacity: 1, y: 0 },
      transition: { duration: 0, delay: 0, ease: MOTION_EASE },
    };
  }

  return {
    // 从 0.35 起步而不是 0：文字始终可读，不会先出现一帧空列表。
    initial: { opacity: 0.35, y: 0 },
    animate: { opacity: 1, y: 0 },
    transition: {
      duration: prefersReducedMotion.value ? 0 : motionSeconds(MOTION_DURATIONS_MS.fast),
      delay: 0,
      ease: MOTION_EASE,
    },
  };
}

/*
 * 区块不再做"滚动进场"（票 441）：列表是页面正文，进入站点时可能刚好落在首屏下沿，
 * 而 reveal 的初态是 opacity: 0 —— 看起来就是"最新文章被隐藏了"。内容该常显：
 * 只有首屏的装饰色带还走进场，正文区块一律直接可见。
 */
const state = computed(() =>
  resolveHomeSectionState({ pending: props.pending, failed: props.failed, itemCount: props.articles.length }),
);
const searchTo = homeSearchLocation();
</script>

<template>
  <section class="blueprint-panel" data-home-section="feed">
    <header class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <div>
        <div class="eyebrow">{{ eyebrow }}</div>
        <h2 class="section-title mt-2">{{ title }}</h2>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <span class="blueprint-chip" data-role="feed-badge">{{ state.badge }}</span>
        <NuxtLink :to="searchTo" class="blueprint-link">进入搜索</NuxtLink>
      </div>
    </header>

    <div
      v-if="articles.length"
      class="flex flex-col gap-4"
      data-role="feed-list"
      :data-entrance="isFirstBatch ? 'visible' : 'swap'"
    >
      <Motion
        v-for="article in articles"
        :key="article.id"
        v-bind="entranceProps()"
        class="feed-motion-item"
      >
        <ArticleFeedItem :article="article" variant="list" />
      </Motion>
    </div>

    <!--
      数据未到 = 先出骨架（ADR-0016 决策 3）：切页时列表不再是"一行加载中文案"，
      而是按条目形状占位；数据一到即被真条目替换。失败态仍走下面的空态（带重试）。
    -->
    <div
      v-else-if="pending"
      class="flex flex-col gap-4"
      data-role="feed-skeleton"
      data-state="pending"
    >
      <LoadingSkeleton :rows="3" variant="card" label="正在读取最新文章" />
    </div>

    <ProfileEmptyState
      v-else
      size="md"
      :title="state.title"
      :description="state.description"
      :hints="state.hints"
    >
      <button v-if="failed" type="button" class="blueprint-tab" @click="emit('retry')">重试本页</button>
      <NuxtLink :to="searchTo" class="blueprint-link">去搜索文章</NuxtLink>
    </ProfileEmptyState>

    <slot name="footer" />
  </section>
</template>
