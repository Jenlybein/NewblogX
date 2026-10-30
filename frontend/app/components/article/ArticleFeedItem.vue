<script setup lang="ts">
import { computed } from "vue";
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from "@tabler/icons-vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SearchArticleItem } from "~/types/api";
import {
  EMPTY_ARTICLE_ENTRY,
  fromSearchArticle,
  type ArticleEntryData,
  type ArticleEntryVariant,
} from "~/utils/articleEntry";
import { formatCount } from "~/utils/format";

const props = withDefaults(
  defineProps<{
    /** 列表 / 搜索结果的原始数据（旧 API）。 */
    article?: SearchArticleItem;
    /** 统一视图模型（收藏架用 fromFavoriteArticle 适配后传入）。优先级高于 article。 */
    entry?: ArticleEntryData;
    variant?: ArticleEntryVariant;
    /** 旧参数：紧凑尺寸，等价于搜索结果的密度。 */
    compact?: boolean;
    showAuthor?: boolean;
  }>(),
  {
    variant: "list",
    compact: false,
    showAuthor: true,
  },
);

const view = computed<ArticleEntryData>(() => {
  if (props.entry) {
    return props.entry;
  }
  if (props.article) {
    return fromSearchArticle(props.article);
  }
  return EMPTY_ARTICLE_ENTRY;
});

const hasData = computed(() => Boolean(view.value.id) && Boolean(view.value.href));
const effectiveVariant = computed<ArticleEntryVariant>(() => (props.compact && props.variant === "list" ? "search" : props.variant));
const visibleTags = computed(() => view.value.tags.slice(0, 3));
const hiddenTagCount = computed(() => Math.max(view.value.tags.length - 3, 0));
</script>

<template>
  <article
    class="feed-entry"
    :class="[`feed-entry--${effectiveVariant}`, hasData ? '' : 'feed-entry--empty']"
    :data-variant="effectiveVariant"
    :data-empty="hasData ? undefined : 'true'"
  >
    <NuxtLink
      v-if="hasData"
      :to="view.href"
      class="feed-entry__cover"
      :aria-label="`打开文章：${view.title}`"
    >
      <ArticleCover :src="view.cover" :title="view.title" />
    </NuxtLink>
    <span v-else class="feed-entry__cover feed-entry__cover--empty" aria-hidden="true" />

    <div class="feed-entry__body">
      <div v-if="view.dateLabel || view.categoryLabel || view.statusLabel" class="feed-entry__meta">
        <span v-if="view.dateLabel">{{ view.dateLabel }}</span>
        <span v-if="view.dateLabel && view.categoryLabel" class="feed-entry__dot" aria-hidden="true">/</span>
        <span v-if="view.categoryLabel">{{ view.categoryLabel }}</span>
        <span v-if="view.statusLabel" class="feed-entry__status">{{ view.statusLabel }}</span>
      </div>

      <NuxtLink v-if="hasData" :to="view.href" class="feed-entry__title">{{ view.title }}</NuxtLink>
      <p v-else class="feed-entry__title">{{ view.title }}</p>

      <p class="feed-entry__abstract">{{ view.abstract }}</p>

      <div v-if="visibleTags.length" class="feed-entry__tags">
        <span v-for="tag in visibleTags" :key="tag.id" class="feed-entry__tag">{{ tag.title }}</span>
        <span v-if="hiddenTagCount" class="feed-entry__tag feed-entry__tag--more">+{{ hiddenTagCount }}</span>
      </div>

      <div v-if="view.shelfMeta.length" class="feed-entry__shelf-meta">
        <span v-for="meta in view.shelfMeta" :key="meta">{{ meta }}</span>
      </div>

      <div class="feed-entry__footer">
        <div v-if="showAuthor" class="feed-entry__author">
          <NuxtLink
            v-if="view.author && view.author.id"
            :to="`/users/${view.author.id}`"
            class="feed-entry__author-link"
          >
            <AppAvatar :size="28" :src="view.author.avatar" :name="view.author.nickname" fallback="作" />
            <span>{{ view.author.nickname }}</span>
          </NuxtLink>
          <span v-else-if="view.author" class="feed-entry__author-link feed-entry__author-link--static">
            <AppAvatar :size="28" :src="view.author.avatar" :name="view.author.nickname" fallback="作" />
            <span>{{ view.author.nickname }}</span>
          </span>
          <span v-else class="feed-entry__author-link feed-entry__author-link--empty">站内文章</span>
        </div>

        <div class="feed-entry__stats" role="group" aria-label="文章统计">
          <span class="feed-entry__stat" data-stat="digg" :title="`点赞 ${view.stats.digg}`">
            <IconThumbUp :size="14" aria-hidden="true" />
            <span>{{ formatCount(view.stats.digg) }}</span>
          </span>
          <span class="feed-entry__stat" data-stat="favor" :title="`收藏 ${view.stats.favor}`">
            <IconHeart :size="14" aria-hidden="true" />
            <span>{{ formatCount(view.stats.favor) }}</span>
          </span>
          <span class="feed-entry__stat" data-stat="comment" :title="`评论 ${view.stats.comment}`">
            <IconMessageCircle2 :size="14" aria-hidden="true" />
            <span>{{ formatCount(view.stats.comment) }}</span>
          </span>
          <span class="feed-entry__stat" data-stat="view" :title="`阅读 ${view.stats.view}`">
            <IconEye :size="14" aria-hidden="true" />
            <span>{{ formatCount(view.stats.view) }}</span>
          </span>
        </div>
      </div>
    </div>
  </article>
</template>
