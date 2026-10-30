<script setup lang="ts">
import { computed } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { FavoriteArticleItem, FavoriteFolderItem } from "~/types/api";
import { fromFavoriteArticle } from "~/utils/articleEntry";

const props = withDefaults(
  defineProps<{
    folders: FavoriteFolderItem[];
    activeFolderId: string;
    articles: FavoriteArticleItem[];
    pending?: boolean;
    locked?: boolean;
    lockedTitle?: string;
    lockedDescription?: string;
  }>(),
  { pending: false, locked: false, lockedTitle: "", lockedDescription: "" },
);

const emit = defineEmits<{
  "update:activeFolderId": [value: string];
}>();

// 收藏架与列表/搜索结果复用同一个条目组件（#7 的卡片三处复用）。
const shelfEntries = computed(() => props.articles.map((article) => fromFavoriteArticle(article)));
const activeFolder = computed(() => props.folders.find((folder) => folder.id === props.activeFolderId) || null);
</script>

<template>
  <div class="profile-collection-layout">
    <aside class="profile-collection-layout__folders" aria-label="公开收藏夹">
      <div v-if="locked" class="profile-folder-note">
        收藏夹未公开，这里只展示文章流与基础成就。
      </div>
      <div v-else-if="!folders.length" class="profile-folder-note">这个用户还没有公开收藏夹。</div>

      <button
        v-for="folder in folders"
        v-else
        :key="folder.id"
        type="button"
        class="profile-folder-card"
        :class="{ 'is-active': activeFolderId === folder.id }"
        :aria-pressed="activeFolderId === folder.id"
        @click="emit('update:activeFolderId', folder.id)"
      >
        <span class="profile-folder-card__title-row">
          <strong class="profile-folder-card__title">{{ folder.title }}</strong>
          <span class="blueprint-chip">{{ folder.article_count }}</span>
        </span>
        <span class="profile-folder-card__abstract">
          {{ folder.abstract || "这个收藏夹还没有补充简介。" }}
        </span>
      </button>
    </aside>

    <section class="profile-collection-layout__content" aria-label="收藏架内容">
      <ProfileEmptyState
        v-if="locked"
        size="lg"
        :title="lockedTitle || '收藏架暂未公开'"
        :description="lockedDescription || '这位作者把收藏夹设成了私密状态，目前只能查看文章流与基础成就。'"
      />

      <ProfileEmptyState
        v-else-if="pending && !shelfEntries.length"
        size="lg"
        pending
        title="正在读取收藏内容"
        description="系统正在读取公开收藏，稍后会自动出现。"
      />

      <div v-else-if="shelfEntries.length" class="profile-shelf-list">
        <div v-if="activeFolder" class="profile-shelf-heading">
          <span class="profile-shelf-heading__title">{{ activeFolder.title }}</span>
          <span class="blueprint-chip">{{ shelfEntries.length }} 篇</span>
        </div>
        <ArticleFeedItem
          v-for="entry in shelfEntries"
          :key="entry.id || entry.href"
          :entry="entry"
          variant="shelf"
        />
      </div>

      <ProfileEmptyState
        v-else
        size="lg"
        title="这个收藏夹还是空的"
        description="当前分组还没有公开文章，换一个收藏夹继续看看。"
        :hints="['空的收藏夹不会展示任何条目，但布局会保留完整占位。', '想看更多公开文章，可以回到文章流或搜索页。']"
      >
        <NuxtLink to="/search" class="blueprint-link">去搜索文章</NuxtLink>
        <NuxtLink to="/" class="blueprint-link">回到首页文章流</NuxtLink>
      </ProfileEmptyState>
    </section>
  </div>
</template>
