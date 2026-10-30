<script setup lang="ts">
import { computed } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
import type { SearchArticleItem } from "~/types/api";
import { homeSearchLocation, resolveHomeSectionState } from "~/utils/home";

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
const { entranceProps } = useStaggerEntrance();
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

    <div v-if="articles.length" class="flex flex-col gap-4" data-role="feed-list">
      <Motion
        v-for="(article, index) in articles"
        :key="article.id"
        v-bind="entranceProps(index)"
        class="motion-stagger-item"
      >
        <ArticleFeedItem :article="article" variant="list" />
      </Motion>
    </div>

    <ProfileEmptyState
      v-else
      size="md"
      :pending="pending"
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
