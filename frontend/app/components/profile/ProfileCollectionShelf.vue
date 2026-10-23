<script setup lang="ts">
import { NButton, NTag, NThing } from "naive-ui";
import ArticleCover from "~/components/article/ArticleCover.vue";
import type { FavoriteArticleItem, FavoriteFolderItem } from "~/types/api";
import { formatCount, formatDateTimeLabel } from "~/utils/format";

defineProps<{
  folders: FavoriteFolderItem[];
  activeFolderId: string;
  articles: FavoriteArticleItem[];
  pending?: boolean;
  locked?: boolean;
  lockedTitle?: string;
  lockedDescription?: string;
}>();

const emit = defineEmits<{
  "update:activeFolderId": [value: string];
}>();
</script>

<template>
  <div class="profile-collection-layout">
    <aside class="profile-collection-layout__folders">
      <button
        v-for="folder in folders"
        :key="folder.id"
        type="button"
        class="profile-folder-card"
        :class="{ 'is-active': activeFolderId === folder.id }"
        @click="emit('update:activeFolderId', folder.id)"
      >
        <div class="profile-folder-card__title-row">
          <strong class="truncate">{{ folder.title }}</strong>
          <span class="glass-badge">{{ folder.article_count }}</span>
        </div>
        <p class="muted mt-2 text-sm leading-6">
          {{ folder.abstract || "这个收藏夹还没有补充简介。" }}
        </p>
      </button>
    </aside>

    <section class="profile-collection-layout__content">
      <div v-if="locked" class="surface-section flex min-h-[280px] flex-col items-center justify-center p-6 text-center">
        <h3 class="section-title">{{ lockedTitle }}</h3>
        <p class="mt-3 max-w-xl text-sm leading-7 muted">{{ lockedDescription }}</p>
      </div>

      <div v-else-if="articles.length" class="space-y-4">
        <article v-for="article in articles" :key="article.article_id" class="article-feed-item">
          <NuxtLink :to="`/article/${article.article_id}`" class="article-feed-cover">
            <ArticleCover :src="article.cover" :title="article.title" />
          </NuxtLink>

          <div class="article-feed-body">
            <NThing :title="article.title" :description="article.abstract">
              <template #header-extra>
                <NTag size="small">{{ article.article_status === 3 ? "已发布" : `状态 ${article.article_status}` }}</NTag>
              </template>
              <template #footer>
                <div class="article-feed-meta">
                  <span>{{ article.user_nickname }}</span>
                  <span>{{ formatDateTimeLabel(article.favorited_at) }}</span>
                  <span>{{ formatCount(article.view_count) }} 阅读</span>
                  <span>{{ formatCount(article.favor_count) }} 收藏</span>
                </div>
              </template>
            </NThing>
          </div>
        </article>
      </div>

      <div v-else class="surface-section flex min-h-[280px] flex-col items-center justify-center p-6 text-center">
        <h3 class="section-title">{{ pending ? "正在加载收藏内容…" : "这个收藏夹还是空的" }}</h3>
        <p class="mt-3 max-w-xl text-sm leading-7 muted">
          {{ pending ? "系统正在读取公开收藏内容。" : "当前分组还没有公开文章，换一个收藏夹继续看看。" }}
        </p>
      </div>
    </section>
  </div>
</template>
