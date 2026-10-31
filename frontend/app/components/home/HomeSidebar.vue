<script setup lang="ts">
import { computed } from "vue";
import type { StyleValue } from "vue";
import { NSkeleton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SearchArticleItem, SiteRuntimeConfig } from "~/types/api";
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

const props = withDefaults(defineProps<{
  pending?: boolean;
  runtimeConfig: SiteRuntimeConfig | null;
  articles: SearchArticleItem[];
  sticky?: boolean;
  stickyTop?: number | string;
}>(), {
  pending: false,
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
</style>
