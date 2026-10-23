<script setup lang="ts">
import { computed } from "vue";
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from "@tabler/icons-vue";
import { NTag } from "naive-ui";
import ArticleCover from "~/components/article/ArticleCover.vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SearchArticleItem } from "~/types/api";
import { formatCount, formatDateLabel } from "~/utils/format";

const props = withDefaults(
  defineProps<{
    article: SearchArticleItem;
    compact?: boolean;
    showAuthor?: boolean;
  }>(),
  {
    compact: false,
    showAuthor: true,
  },
);

const titleText = computed(() => props.article.highlight?.title || props.article.title);
const abstractText = computed(() => props.article.highlight?.abstract || props.article.abstract || "这篇文章暂时没有摘要。");
</script>

<template>
  <article class="article-feed-item">
    <NuxtLink :to="`/article/${article.id}`" class="article-feed-cover">
      <ArticleCover :src="article.cover" :title="article.title" />
    </NuxtLink>

    <div class="article-feed-body">
      <div class="article-feed-meta">
        <span>{{ formatDateLabel(article.created_at) }}</span>
        <span v-if="article.category?.title">{{ article.category.title }}</span>
      </div>

      <NuxtLink :to="`/article/${article.id}`" class="article-feed-title" :class="{ 'article-feed-title--compact': compact }">
        {{ titleText }}
      </NuxtLink>

      <NuxtLink :to="`/article/${article.id}`" class="article-feed-abstract" :class="compact ? 'line-clamp-1' : 'line-clamp-2'">
        {{ abstractText }}
      </NuxtLink>

      <div class="article-feed-footer">
        <div class="article-feed-author-row">
          <NuxtLink
            v-if="showAuthor"
            :to="`/users/${article.author.id}`"
            class="inline-flex items-center gap-2"
          >
            <AppAvatar :size="30" :src="article.author.avatar" :name="article.author.nickname" fallback="作" />
            <span class="text-sm font-medium">{{ article.author.nickname }}</span>
          </NuxtLink>

          <div class="article-feed-tags">
            <NTag
              v-for="tag in article.tags.slice(0, 3)"
              :key="tag.id"
              size="small"
              round
              :bordered="false"
            >
              {{ tag.title }}
            </NTag>
          </div>
        </div>

        <div class="article-feed-stats">
          <span class="article-feed-stat"><IconThumbUp :size="16" /> {{ formatCount(article.digg_count) }}</span>
          <span class="article-feed-stat"><IconHeart :size="16" /> {{ formatCount(article.favor_count) }}</span>
          <span class="article-feed-stat"><IconMessageCircle2 :size="16" /> {{ formatCount(article.comment_count) }}</span>
          <span class="article-feed-stat"><IconEye :size="16" /> {{ formatCount(article.view_count) }}</span>
        </div>
      </div>
    </div>
  </article>
</template>
