<script setup lang="ts">
import { computed } from "vue";
import { useSectionReveal } from "~/composables/useSectionReveal";
import type { ArticleHotTopItem } from "~/types/api";
import { formatCount } from "~/utils/format";
import { pinnedArticleView, type PinnedArticleView } from "~/utils/home";

const props = defineProps<{ articles: ArticleHotTopItem[] }>();

// 区块级滚动进场：只动容器的 transform/opacity，卡片内容本身不进场。
const { target: revealTarget, revealStyle } = useSectionReveal();
// 顺序就是运营位接口下发的顺序（后端按管理员手动排序返回），这里不重排、不排序。
const views = computed<PinnedArticleView[]>(() => props.articles.map((article) => pinnedArticleView(article)));
const count = computed(() => views.value.length);
</script>

<template>
  <!--
    ADR-0017 决策 5：热门置顶只在**真有内容**时出现。
    无置顶 / 加载中 / 请求失败一律走「整块不渲染」——连 eyebrow、标题、徽标都不出现，
    所以这里刻意没有空态分支（也就没有可显示的空态文案与重试入口）。
  -->
  <section
    v-if="count > 0"
    ref="revealTarget"
    class="blueprint-panel motion-reveal"
    :style="revealStyle"
    data-home-section="pinned"
  >
    <header class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <div>
        <div class="eyebrow">Pinned</div>
        <h2 class="section-title mt-2">热门置顶</h2>
      </div>
      <span class="blueprint-chip" data-role="pinned-badge">{{ count }} 篇</span>
    </header>

    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3" data-role="pinned-grid">
      <article v-for="(view, index) in views" :key="view.id" class="home-pinned-card">
        <div class="flex items-center justify-between gap-2">
          <span class="home-pinned-card__index" aria-hidden="true">{{ String(index + 1).padStart(2, "0") }}</span>
          <span v-if="view.categoryLabel" class="blueprint-chip">{{ view.categoryLabel }}</span>
        </div>
        <NuxtLink :to="view.href" class="home-pinned-card__title">{{ view.title }}</NuxtLink>
        <p class="home-pinned-card__abstract">{{ view.abstract }}</p>
        <div class="home-pinned-card__footer">
          <span v-if="view.authorName" class="text-xs text-ink-muted">{{ view.authorName }}</span>
          <span class="text-xs text-ink-muted">{{ formatCount(view.viewCount) }} 阅读</span>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
/* 置顶卡是"编号 + 标题 + 摘要 + 出处"的图纸条目；三档列数由模板里的 grid 控制。 */
.home-pinned-card {
  display: flex;
  min-height: 168px;
  flex-direction: column;
  gap: 0.5rem;
  border: 1px solid var(--color-line);
  background: var(--color-surface);
  padding: 1rem;
}

.home-pinned-card__index {
  font-family: var(--font-latin);
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1;
  letter-spacing: var(--type-tracking-title);
  color: var(--color-line-strong);
}

.home-pinned-card__title {
  display: block;
  font-family: var(--font-sans);
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.35;
  color: var(--color-ink);
}

.home-pinned-card__title:hover {
  text-decoration: underline;
  text-underline-offset: 4px;
}

.home-pinned-card__abstract {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  margin: 0;
  font-size: 0.8125rem;
  line-height: 1.6;
  color: var(--color-ink-muted);
}

.home-pinned-card__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-top: auto;
  border-top: 1px solid var(--color-line-faint);
  padding-top: 0.5rem;
}
</style>
