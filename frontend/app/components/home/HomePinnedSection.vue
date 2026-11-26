<script setup lang="ts">
import { computed } from "vue";
import type { ArticleHotTopItem } from "~/types/api";
import { formatCount } from "~/utils/format";
import { pinnedArticleView, type PinnedArticleView } from "~/utils/home";

const props = defineProps<{ articles: ArticleHotTopItem[] }>();

/*
 * 不做"滚动进场"（票 441）：置顶是正文区块，进入站点时可能落在首屏下沿，
 * reveal 的 opacity: 0 初态会让它看起来是"被隐藏了"。内容直接可见。
 */
// 顺序就是运营位接口下发的顺序（后端按管理员手动排序返回），这里不重排、不排序。
const views = computed<PinnedArticleView[]>(() => props.articles.map((article) => pinnedArticleView(article)));
const count = computed(() => views.value.length);
</script>

<template>
  <!--
    ADR-0015 决策 5：热门置顶只在**真有内容**时出现。
    无置顶 / 加载中 / 请求失败一律走「整块不渲染」——连 eyebrow、标题、徽标都不出现，
    所以这里刻意没有空态分支（也就没有可显示的空态文案与重试入口）。
  -->
  <Transition name="pinned-in">
    <section v-if="count > 0" class="blueprint-panel" data-home-section="pinned">
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
  </Transition>
</template>

<style scoped>
/* 置顶卡是"编号 + 标题 + 摘要 + 出处"的图纸条目；三档列数由模板里的 grid 控制。
 * 圆角取卡片阶梯（票 429）：网格在 blueprint-panel 里，面板自己是 20px 圆角，
 * 里面的卡若还是直角就会"咬"住面板的圆边，读起来像卡没做完。同一档 = 同轴曲率。 */
.home-pinned-card {
  display: flex;
  min-height: 168px;
  flex-direction: column;
  gap: 0.5rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  padding: 1rem;
  transition:
    background-color var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
}

/* 悬停：与信息流卡一致的"纸面被指到"（底色 + 描边，不加阴影、不改层级）。 */
.home-pinned-card:hover {
  border-color: var(--color-line-strong);
  background-color: var(--color-hover-surface);
}

.home-pinned-card__index {
  font-family: var(--font-latin);
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1;
  letter-spacing: var(--type-tracking-md);
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

/*
 * 置顶区块晚到时的进场：客户端导航时置顶数据比信息流晚一步到，区块会插在
 * 已经渲染的列表上方、把正文直接推下去。这里只做 200ms 的淡入 + 4px 落下，
 * 让"内容被推"读成一次有序插入，而不是画面跳一下。
 * （它不隐藏内容：数据没到就不渲染，到了才出现一次。）
 */
.pinned-in-enter-active {
  transition:
    opacity var(--motion-base) var(--motion-ease),
    transform var(--motion-base) var(--motion-ease);
}

.pinned-in-enter-from {
  opacity: 0;
  transform: translateY(-4px);
}

@media (prefers-reduced-motion: reduce) {
  .pinned-in-enter-active {
    transition: none;
  }
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
