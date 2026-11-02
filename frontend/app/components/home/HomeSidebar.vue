<script setup lang="ts">
import { computed } from "vue";
import type { StyleValue } from "vue";
import { NSkeleton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SearchArticleItem, SiteRuntimeConfig, SiteSidebarCardItem } from "~/types/api";
import { resolveAvatarUrl } from "~/utils/avatar";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
import { useSectionReveal } from "~/composables/useSectionReveal";
import {
  aggregateHotTags,
  aggregateRecommendedAuthors,
  homeSidebarVisibility,
} from "~/utils/home";
import { SIDEBAR_CARD_EYEBROWS, sidebarCardViews, tagCloudFontSize } from "~/utils/sidebarCards";

const props = withDefaults(defineProps<{
  pending?: boolean;
  runtimeConfig: SiteRuntimeConfig | null;
  articles: SearchArticleItem[];
  /**
   * 数据驱动卡片（票 111）：后端返回的卡片列表（种类 + 顺序 + 标题 + 各卡数据）。
   *
   * 非空时右栏**只**渲染这些卡；为空时回退到旧右栏（公告 / 热门标签 / 推荐作者），
   * 这是本票"旧配置仍兼容、不报错"的口径。下一票把旧三卡迁移成同一份数据后，
   * 这条回退分支会被删掉。
   */
  sidebarCards?: SiteSidebarCardItem[];
  /**
   * 主栏「热门置顶」已经露出的文章 ID。
   *
   * 右栏热门文章卡必须与它去重：同一篇文章在首页出现两次是版面事故。
   * 置顶与右栏卡片是两个独立请求（各走 30s SWR），列表可能短暂不同步，
   * 所以在渲染模型里按当前置顶列表再收一次。
   */
  pinnedArticleIds?: string[];
  sticky?: boolean;
  stickyTop?: number | string;
}>(), {
  pending: false,
  sidebarCards: () => [],
  pinnedArticleIds: () => [],
  sticky: false,
  stickyTop: "0.5rem",
});

// 右栏只有标签是"非关键内容"：给它错峰入场（40ms 一档，只动 opacity/translateY）。
const { entranceProps } = useStaggerEntrance();
// 右栏整体作为一个区块进场（内部条目用 stagger，正文段落不动）。
const { target: revealTarget, revealStyle } = useSectionReveal();

const sidebarClasses = computed(() => ({
  "profile-sidebar": true,
  "home-sidebar": true,
  "page-sidebar--sticky": props.sticky,
  // 区块进场的静态类：给 noscript / 减少动态下的 CSS 覆盖用（见 app.vue 与 tailwind.css）。
  "motion-reveal": true,
}));

const sidebarStyle = computed<StyleValue>(() => {
  if (!props.sticky) {
    return undefined;
  }

  return {
    "--page-sidebar-sticky-top": typeof props.stickyTop === "number" ? `${props.stickyTop}px` : props.stickyTop,
  };
});

const visibility = computed(() => homeSidebarVisibility(props.runtimeConfig));
const hotTags = computed(() => aggregateHotTags(props.articles, 8));
const recommendedAuthors = computed(() => aggregateRecommendedAuthors(props.articles, 4));

// 数据驱动的渲染模型：未知种类过滤、空数据隐藏、标题回退、顺序收敛、置顶去重都在纯函数里。
const cardViews = computed(() =>
  sidebarCardViews(props.sidebarCards, { pinnedArticleIds: props.pinnedArticleIds }),
);
// 回退看的是"有没有配置"，不是"有没有渲染出内容"：运营配了卡片但恰好都没数据时，
// 旧右栏不能又冒出来 —— 那会让人以为新配置没生效。
const hasConfiguredCards = computed(() => (props.sidebarCards?.length ?? 0) > 0);
</script>

<template>
  <aside
    ref="revealTarget"
    :class="sidebarClasses"
    :style="[sidebarStyle, revealStyle]"
    data-home-sidebar
  >
    <section v-if="pending" class="blueprint-panel" data-home-section="sidebar-skeleton">
      <NSkeleton text width="96px" />
      <NSkeleton text class="mt-3" :repeat="2" />
      <div class="mt-4 flex flex-wrap gap-2">
        <NSkeleton v-for="idx in 3" :key="`notice-tag-${idx}`" height="24px" width="64px" />
      </div>
    </section>

    <!--
      数据驱动卡片（票 111）：种类、顺序、标题、启用全来自 `GET /site/sidebar_cards`。
      空数据的卡已经在 sidebarCardViews 里被过滤掉，这里拿到的每一张都有内容。
    -->
    <section
      v-for="card in cardViews"
      :key="card.key"
      class="blueprint-panel"
      :data-sidebar-card="card.kind"
    >
      <div class="eyebrow">{{ SIDEBAR_CARD_EYEBROWS[card.kind] }}</div>
      <h2 class="section-title mt-2" data-sidebar-card-title>{{ card.title }}</h2>

      <!-- 热门文章：按点赞数降序，已经出现在主栏热门置顶的文章不会再重复露出。 -->
      <div v-if="card.kind === 'hot_articles'" class="mt-3 flex flex-col">
        <NuxtLink
          v-for="article in card.articles"
          :key="article.id"
          :to="`/article/${article.id}`"
          class="sidebar-card-row"
        >
          <span class="min-w-0 truncate text-sm text-ink">{{ article.title }}</span>
          <span class="shrink-0 text-xs text-ink-muted">{{ article.diggCount }} 赞</span>
        </NuxtLink>
      </div>

      <!-- 最新评论：内容一行截断，第二行是作者与文章标题（都指向文章页）。 -->
      <div v-else-if="card.kind === 'latest_comments'" class="mt-3 flex flex-col">
        <NuxtLink
          v-for="comment in card.comments"
          :key="comment.id"
          :to="`/article/${comment.articleId}`"
          class="sidebar-card-row sidebar-card-row--stacked"
        >
          <span class="min-w-0 truncate text-sm text-ink">{{ comment.content }}</span>
          <span class="mt-1 truncate text-xs text-ink-muted">
            {{ comment.userNickname }} · {{ comment.articleTitle }}
          </span>
        </NuxtLink>
      </div>

      <!-- 标签云：字号按本卡内的权重决定，不是热门标签那种等字号 chip 列表。 -->
      <div v-else-if="card.kind === 'tag_cloud'" class="mt-3 flex flex-wrap gap-2">
        <Motion
          v-for="(tag, index) in card.tags"
          :key="tag.id"
          v-bind="entranceProps(index)"
          class="motion-stagger-item inline-flex"
        >
          <NuxtLink
            :to="{ path: '/search', query: { tag_ids: tag.id } }"
            class="blueprint-chip"
            :style="{ fontSize: tagCloudFontSize(tag.weight) }"
          >
            {{ tag.title }}
          </NuxtLink>
        </Motion>
      </div>

      <!--
        推荐合集：只含展示中的合集（可见性与顺序都由后端按合集自己的口径过滤），
        整行进合集详情。空合集列表已在 sidebarCardViews 里把整张卡隐藏。
      -->
      <div v-else-if="card.kind === 'recommended_collections'" class="mt-3 flex flex-col">
        <NuxtLink
          v-for="collection in card.collections"
          :key="collection.id"
          :to="collection.href"
          class="sidebar-card-row sidebar-card-row--collection"
        >
          <span v-if="collection.cover" class="sidebar-collection-cover">
            <img :src="collection.cover" :alt="`${collection.title} 封面`" loading="lazy" width="48" height="48">
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm text-ink">{{ collection.title }}</span>
            <span
              v-if="collection.description"
              class="mt-1 block truncate text-xs text-ink-muted"
              data-sidebar-collection-description
            >
              {{ collection.description }}
            </span>
            <span class="mt-1 block text-xs text-ink-faint" data-sidebar-collection-count>
              {{ collection.countLabel }}
            </span>
          </span>
        </NuxtLink>
      </div>
      <!--
        富文本卡：正文已在 `sidebarCardViews` 里清洗过，这里只负责渲染。
        `v-html` 在这个文件里是唯一一处直接渲染 HTML 的地方 —— 只在 rich_text
        分支出现，且拿的都是清洗后的字符串。
      -->
      <!-- eslint-disable-next-line vue/no-v-html -- 正文已在渲染模型里过白名单，此处只渲染清洗后的字符串 -->
      <div v-else-if="card.kind === 'rich_text'" class="sidebar-rich-text" data-sidebar-rich-text v-html="card.contentHtml" />
    </section>

    <!-- 旧右栏（无卡片配置时回退；口径见 props.sidebarCards 的注释）。 -->
    <template v-if="!hasConfiguredCards">
      <section v-if="visibility.tags" class="blueprint-panel" data-home-section="tags">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div class="eyebrow">Tags</div>
            <h2 class="section-title mt-2">热门标签</h2>
          </div>
          <span class="blueprint-chip">{{ hotTags.length }} 个</span>
        </div>

        <div v-if="hotTags.length" class="mt-3 flex flex-wrap gap-2">
          <Motion
            v-for="(tag, index) in hotTags"
            :key="tag.id"
            v-bind="entranceProps(index)"
            class="motion-stagger-item inline-flex"
          >
            <NuxtLink :to="{ path: '/search', query: { tag_ids: tag.id } }" class="blueprint-chip">
              {{ tag.title }}
            </NuxtLink>
          </Motion>
        </div>
        <p v-else class="mt-3 text-xs leading-6 text-ink-muted">文章加载后，这里会按出现次数排出热门标签。</p>
      </section>

      <section v-if="visibility.authors" class="blueprint-panel" data-home-section="authors">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div class="eyebrow">Authors</div>
            <h2 class="section-title mt-2">推荐作者</h2>
          </div>
          <span class="blueprint-chip">{{ recommendedAuthors.length }} 位</span>
        </div>

        <div v-if="recommendedAuthors.length" class="mt-3 flex flex-col">
          <NuxtLink
            v-for="author in recommendedAuthors"
            :key="author.id"
            :to="`/users/${author.id}`"
            class="home-author-row"
          >
            <div class="flex min-w-0 items-center gap-3">
              <AppAvatar :size="36" :src="resolveAvatarUrl(author.avatar)" :name="author.nickname" fallback="作" />
              <div class="min-w-0">
                <div class="truncate text-sm font-semibold text-ink">{{ author.nickname }}</div>
                <div class="text-xs text-ink-muted">{{ author.articleCount }} 篇文章</div>
              </div>
            </div>
            <span class="shrink-0 text-xs text-ink-muted">{{ author.totalViews }} 阅读</span>
          </NuxtLink>
        </div>
        <p v-else class="mt-3 text-xs leading-6 text-ink-muted">文章加载后，这里会按文章数推荐作者。</p>
      </section>
    </template>
  </aside>
</template>

<style scoped>
/* 右栏作者行：整行可点（≥44px 触控区），1px 结构线分隔，不用阴影/圆角。 */
.home-author-row {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-top: 1px solid var(--color-line-faint);
  padding: 0.5rem 0;
}

.home-author-row:last-child {
  border-bottom: 1px solid var(--color-line-faint);
}

.home-author-row:hover {
  color: var(--color-ink);
}

/* 数据驱动卡片的行：与作者行同一套结构线，但不需要头像位。 */
.sidebar-card-row {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-top: 1px solid var(--color-line-faint);
  padding: 0.5rem 0;
}

.sidebar-card-row--stacked {
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
}

/* 推荐合集的一行：封面上对齐，文字块占满剩余宽度。 */
.sidebar-card-row--collection {
  align-items: flex-start;
  justify-content: flex-start;
}

.sidebar-collection-cover {
  display: block;
  flex: 0 0 48px;
  width: 48px;
  height: 48px;
  overflow: hidden;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-sm);
  background: var(--color-bg-subtle);
}

.sidebar-collection-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.sidebar-card-row:last-child {
  border-bottom: 1px solid var(--color-line-faint);
}

.sidebar-card-row:hover {
  color: var(--color-ink);
}

/* 富文本卡正文：只给基础排版一层紧凑样式，颜色/分隔线全部走语义 token。 */
.sidebar-rich-text {
  margin-top: 0.75rem;
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.75;
}

.sidebar-rich-text :deep(> :first-child) {
  margin-top: 0;
}

.sidebar-rich-text :deep(> :last-child) {
  margin-bottom: 0;
}

.sidebar-rich-text :deep(h2),
.sidebar-rich-text :deep(h3),
.sidebar-rich-text :deep(h4) {
  margin: 1rem 0 0.5rem;
  color: var(--color-ink);
  font-family: var(--font-display);
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.4;
}

.sidebar-rich-text :deep(p) {
  margin: 0.5rem 0;
}

.sidebar-rich-text :deep(strong) {
  color: var(--color-ink);
  font-weight: 600;
}

.sidebar-rich-text :deep(em) {
  font-style: italic;
}

.sidebar-rich-text :deep(ul),
.sidebar-rich-text :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.25rem;
}

.sidebar-rich-text :deep(ul) {
  list-style: disc;
}

.sidebar-rich-text :deep(ol) {
  list-style: decimal;
}

.sidebar-rich-text :deep(li) {
  margin: 0.25rem 0;
}

.sidebar-rich-text :deep(blockquote) {
  margin: 0.75rem 0;
  border-left: 2px solid var(--color-line-faint);
  padding-left: 0.75rem;
  color: var(--color-ink-muted);
}

.sidebar-rich-text :deep(a) {
  color: var(--color-accent);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.sidebar-rich-text :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.8125rem;
}

.sidebar-rich-text :deep(pre) {
  margin: 0.75rem 0;
  overflow-x: auto;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-sm);
  padding: 0.5rem 0.75rem;
  background: var(--color-bg-subtle);
}

.sidebar-rich-text :deep(hr) {
  margin: 1rem 0;
  border: 0;
  border-top: 1px solid var(--color-line-faint);
}
</style>
