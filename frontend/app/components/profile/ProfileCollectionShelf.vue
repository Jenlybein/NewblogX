<script setup lang="ts">
import { computed } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { FavoriteArticleItem, FavoriteFolderItem } from "~/types/api";
import { fromFavoriteArticle } from "~/utils/articleEntry";

/**
 * 公开主页的「收藏」栏。
 *
 * 2026-11-12：左栏从 280px 的大卡改成与「分类」同口径的 180px 窄栏
 * （一行一个收藏夹 + 篇数），给右侧文章让位；选中态与分类、顶部 tab 统一成
 * 墨色实底。右栏也不再重复一条「收藏夹名 + 篇数」的标题栏 —— 左边已经选中了。
 *
 * 空态文案同步收紧：原来一段话 + 两条提示，把 300px 高的空态填满，读者还是
 * 不知道下一步干什么；现在只留一句标题 + 两个出口。
 *
 * `failed` 是"读不到"而不是"空"：非默认收藏夹对匿名读面是 403，本人看自己的
 * 主页要带 token 才能读到（页面已按 isSelf 传）。混成"空"会让作者以为丢数据。
 */
const props = withDefaults(
  defineProps<{
    folders: FavoriteFolderItem[];
    activeFolderId: string;
    articles: FavoriteArticleItem[];
    pending?: boolean;
    locked?: boolean;
    failed?: boolean;
    lockedTitle?: string;
    lockedDescription?: string;
  }>(),
  { pending: false, locked: false, failed: false, lockedTitle: "", lockedDescription: "" },
);

const emit = defineEmits<{
  "update:activeFolderId": [value: string];
}>();

// 收藏架与列表/搜索结果复用同一个条目组件（#7 的卡片三处复用）。
const shelfEntries = computed(() => props.articles.map((article) => fromFavoriteArticle(article)));
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
        :aria-label="`${folder.title}：${folder.article_count} 篇公开收藏`"
        @click="emit('update:activeFolderId', folder.id)"
      >
        <strong class="profile-folder-card__title">{{ folder.title }}</strong>
        <span class="profile-folder-card__count">{{ folder.article_count }}</span>
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
        v-else-if="failed"
        size="lg"
        title="这个收藏夹的内容没有读到"
        description="可能只有本人能看，也可能是这次请求失败了；换一个收藏夹或稍后刷新试试。"
      />

      <ProfileEmptyState
        v-else-if="pending && !shelfEntries.length"
        size="lg"
        pending
        title="正在读取收藏内容"
        description="系统正在读取公开收藏，稍后会自动出现。"
      />

      <div v-else-if="shelfEntries.length" class="profile-shelf-list">
        <ArticleFeedItem
          v-for="entry in shelfEntries"
          :key="entry.id || entry.href"
          :entry="entry"
          variant="shelf"
        />
      </div>

      <ProfileEmptyState v-else size="lg" title="这个收藏夹还是空的">
        <NuxtLink to="/search" class="blueprint-link">去搜索文章</NuxtLink>
        <NuxtLink to="/" class="blueprint-link">回到首页文章流</NuxtLink>
      </ProfileEmptyState>
    </section>
  </div>
</template>
