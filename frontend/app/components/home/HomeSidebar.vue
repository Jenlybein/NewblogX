<script setup lang="ts">
import { computed } from "vue";
import type { StyleValue } from "vue";
import { NSkeleton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SearchArticleItem, SiteRuntimeConfig, SiteSidebarCardItem } from "~/types/api";
import { resolveAvatarUrl } from "~/utils/avatar";
import { brandProjectTitle } from "~/utils/brand";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
import { useSectionReveal } from "~/composables/useSectionReveal";
import {
  aggregateHotTags,
  aggregateRecommendedAuthors,
  homeLoginModes,
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
  sticky?: boolean;
  stickyTop?: number | string;
}>(), {
  pending: false,
  sidebarCards: () => [],
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
const loginModes = computed(() => homeLoginModes(props.runtimeConfig));
const hotTags = computed(() => aggregateHotTags(props.articles, 8));
const recommendedAuthors = computed(() => aggregateRecommendedAuthors(props.articles, 4));

// 数据驱动的渲染模型：未知种类过滤、空数据隐藏、标题回退、顺序收敛都在纯函数里。
const cardViews = computed(() => sidebarCardViews(props.sidebarCards));
// 回退看的是"有没有配置"，不是"有没有渲染出内容"：运营配了卡片但恰好都没数据时，
// 旧右栏不能又冒出来 —— 那会让人以为新配置没生效。
const hasConfiguredCards = computed(() => (props.sidebarCards?.length ?? 0) > 0);

const projectTitle = computed(() =>
  brandProjectTitle(props.runtimeConfig?.project?.title || props.runtimeConfig?.site_info?.title),
);
// 站点简介：后端 `site` 段里没有这一项 —— `SiteInfo` 只有 title/logo/beian，
// `Project` 只有 title/icon/web_path。此前这里先挂 `project.abstract`、再挂
// `site_info.subtitle`，两个后端都没有的字段，读出来永远为空，整条链在生产里
// 恒等于这句默认值（#39 清掉了这两个死分支）。想让它可配置，得先在后端加字段。
const projectAbstract = "为开发者写作、搜索与知识整理提供稳定的内容入口。";
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

      <!-- 热门文章：按点赞数降序，与主栏的热门置顶不是同一份数据。 -->
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
      <div v-else class="mt-3 flex flex-wrap gap-2">
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
    </section>

    <!-- 旧右栏（无卡片配置时回退；口径见 props.sidebarCards 的注释）。 -->
    <template v-if="!hasConfiguredCards">
      <section v-if="visibility.notice" class="blueprint-panel" data-home-section="notice">
        <div class="eyebrow">Notice</div>
        <h2 class="section-title mt-2">站点公告</h2>
        <p class="mt-3 text-sm leading-prose text-ink-muted">{{ projectAbstract }}</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <span class="blueprint-chip">{{ projectTitle }}</span>
          <span v-for="mode in loginModes" :key="mode" class="blueprint-chip">{{ mode }}</span>
        </div>
      </section>

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

.sidebar-card-row:last-child {
  border-bottom: 1px solid var(--color-line-faint);
}

.sidebar-card-row:hover {
  color: var(--color-ink);
}
</style>
