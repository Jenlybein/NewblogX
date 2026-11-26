<script setup lang="ts">
import { computed, ref, watch } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
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

// 列表入场：只动 opacity/translateY，错峰 40ms；减少动态时归零（静态出现）。
// SSR 的 initial 会内联成 opacity:0，app.vue 的 <noscript> 兜底样式保证无 JS 仍可读。
const { entranceProps: staggerEntranceProps, prefersReducedMotion } = useStaggerEntrance();

/**
 * 「已入场」标记（票 433）：只活在本组件内，不落 session 存储。
 *
 * 首次进入该区块的那一批照常逐条错峰；翻页之后批次换了，新一批**直接出现** ——
 * 读者不用在每次点"下一页"后等半秒。空批次（加载中 / 空态）不消耗入场。
 * 用批次 id 的序列当身份，是为了区分"翻页换了内容"与"同一批数据的原地刷新"
 * （后者不该改变入场判定，Motion 也不会因为同 key 重播）。
 */
const firstBatchKey = ref<string | null>(null);
const hasEntered = ref(false);

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
      hasEntered.value = true;
    }
  },
  { immediate: true },
);

/**
 * 条目的入场 props：首批逐条错峰；已入场后返回"立即可见"（不隐藏、不延迟、时长 0），
 * 新一批因此直接出现，而不是又播一遍错峰的隐藏初态。
 */
function entranceProps(index: number) {
  if (!hasEntered.value) {
    return staggerEntranceProps(index);
  }

  /*
   * 翻页后的新一批（票 441 加法）：不再"瞬间换掉整屏文字"。
   * 一次 120ms 的淡入 + 4px 落位，既不隐藏内容也不排队（delay 恒为 0，不动高度），
   * 只是让"这一批换完了"有个发生的瞬间。减少动态下时长归零（由 motion 预算给出）。
   */
  return {
    initial: { opacity: 0, y: 4 },
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
      :data-entrance="hasEntered ? 'instant' : 'stagger'"
    >
      <Motion
        v-for="(article, index) in articles"
        :key="article.id"
        v-bind="entranceProps(index)"
        class="motion-stagger-item"
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
