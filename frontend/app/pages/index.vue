<script setup lang="ts">
import { computed, shallowRef } from "vue";
import { NButton } from "naive-ui";
import BannerCarousel from "~/components/article/BannerCarousel.vue";
import HomeFeedSection from "~/components/home/HomeFeedSection.vue";
import HomePinnedSection from "~/components/home/HomePinnedSection.vue";
import HomeSidebar from "~/components/home/HomeSidebar.vue";
import { useParallax } from "~/composables/useParallax";
import { useSectionReveal } from "~/composables/useSectionReveal";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
import { getTopArticles } from "~/services/article";
import { searchArticles } from "~/services/search";
import { getBannerList } from "~/services/site";
import { BRAND, brandDescription, brandFullName } from "~/utils/brand";
import { homeSearchLocation } from "~/utils/home";

const siteStore = useSiteStore();
const topRequestError = shallowRef<unknown>(null);
const latestRequestError = shallowRef<unknown>(null);

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
}

// 首页数据全部"失败降级不抛错"：区块组件拿到 pending/failed 后渲染统一空态，
// 不会因为一个接口挂了就让整页白屏。
const { data: bannerData, pending: bannerPending } = await useAsyncData("home-banners", () =>
  getBannerList().catch(() => ({ list: [], has_more: false })),
);
const { data: topData, pending: topPending, refresh: refreshTop } = await useAsyncData(
  "home-top-articles",
  async () => {
    try {
      topRequestError.value = null;
      return await getTopArticles();
    } catch (error) {
      topRequestError.value = error;
      console.warn(`[home-top-articles] request failed, using fallback: ${formatRequestError(error)}`);
      return { list: [], count: 0 };
    }
  },
);

const latestPager = await usePagedResourceCache({
  cacheKey: () => "home-latest-articles",
  pageSize: () => 9,
  fetchPage: async (page, limit) => {
    try {
      latestRequestError.value = null;
      const payload = await searchArticles({
        type: 1,
        page,
        limit,
        page_mode: "has_more",
        sort: 2,
      });
      return {
        items: payload.list,
        hasMore: payload.pagination.has_more,
      };
    } catch (error) {
      latestRequestError.value = error;
      console.warn(`[home-latest-articles] request failed, using fallback: ${formatRequestError(error)}`);
      return {
        items: [],
        hasMore: false,
      };
    }
  },
});

useSeoMeta({
  title: siteStore.seo?.site_title || brandFullName(),
  description: brandDescription(siteStore.seo?.description),
  keywords: siteStore.seo?.keywords || "前端, Nuxt, Vue, OpenAPI",
  ogTitle: siteStore.seo?.site_title || brandFullName(),
  ogDescription: brandDescription(siteStore.seo?.description),
});

const banners = computed(() => bannerData.value?.list || []);
const topArticles = computed(() => topData.value?.list || []);
const latestArticles = computed(() => latestPager.currentItems.value);
const latestPending = computed(() => latestPager.pending.value);
const latestPage = computed(() => latestPager.currentPage.value);
const latestHasPrevious = computed(() => latestPager.hasPreviousPage.value);
const latestHasNext = computed(() => latestPager.hasNextPage.value);
const siteBootstrapPending = computed(() => !siteStore.fetched);
const topFailed = computed(() => Boolean(topRequestError.value));
const latestFailed = computed(() => Boolean(latestRequestError.value));

// 首屏 CTA 的搜索落点复用页头同一个 URL 契约（/search，空关键词不拼 ?key=）。
const searchTo = homeSearchLocation();

// 首屏封面的分层动效（ADR-0013 决策 9，全部归口 #23 的同一份预算）：
//   · 装饰层（色带 / 网格刻度）走 useSectionReveal：只动 transform/opacity，位移 ≤12px；
//   · 文字层（h1/描述/CTA/记号）走 useStaggerEntrance：错峰 40ms；
//   · 记号再加一条 useParallax：≤10px，reduced-motion 下两者都归零。
// 首屏不自己写关键帧动画，也不对布局属性做过渡。
const { target: coverRevealTarget, revealStyle } = useSectionReveal();
const { parallaxStyle } = useParallax();
const { entranceProps } = useStaggerEntrance();

async function handlePreviousLatestPage() {
  await latestPager.goToPreviousPage();
}

async function handleNextLatestPage() {
  await latestPager.goToNextPage();
}

async function handleRetryTop() {
  await refreshTop();
}

async function handleRetryLatest() {
  await latestPager.refreshCurrentPage();
}
</script>

<template>
  <div class="page-stack" data-page="home">
    <!--
      首屏封面（ADR-0013）：全站唯一的编辑部式例外区。
      色带 / 网格刻度 / 方括号与箭头记号 / 胶囊小签只允许在这里出现（有设计测试锁住）。
      图层自下而上：底图（BannerCarousel 的媒体层）→ 罩层 → 色带 → 网格刻度 →
      超大 h1 → 记号 → 玻璃胶囊轮播控件。
    -->
    <section ref="coverRevealTarget" class="home-cover" data-home-hero>
      <!-- 2 色带：黄 / 功能蓝 / 珊瑚，仅首屏可作装饰（3 条，窄屏只留 1 条） -->
      <div class="home-cover__bands motion-reveal" :style="revealStyle" aria-hidden="true">
        <span class="home-cover__band home-cover__band--yellow" />
        <span class="home-cover__band home-cover__band--blue" />
        <span class="home-cover__band home-cover__band--coral" />
      </div>

      <!-- 3 网格线与下沿刻度：≥1024px 才显示（窄屏隐藏） -->
      <div class="home-cover__grid motion-reveal" :style="revealStyle" aria-hidden="true">
        <span class="home-cover__grid-line home-cover__grid-line--a" />
        <span class="home-cover__grid-line home-cover__grid-line--b" />
        <span class="home-cover__grid-line home-cover__grid-line--c" />
      </div>

      <!-- 4 超大 h1 + 5 记号 -->
      <div class="home-cover__inner">
        <div class="home-cover__copy">
          <Motion v-bind="entranceProps(0)" class="motion-stagger-item">
            <div class="eyebrow home-cover__eyebrow">{{ BRAND.name.en }} · {{ BRAND.name.zh }}</div>
            <h1 class="home-cover__title">
              <span class="home-cover__title-name">{{ BRAND.name.en.toUpperCase() }}</span>
              <span class="home-cover__title-bracket">[{{ BRAND.name.zh }}]</span>
            </h1>
            <p class="home-cover__lede">{{ brandDescription(siteStore.seo?.description) }}</p>
          </Motion>

          <Motion v-bind="entranceProps(1)" class="motion-stagger-item home-cover__actions">
            <NuxtLink :to="searchTo" class="blueprint-tab">搜索文章</NuxtLink>
            <NuxtLink to="/studio/write" class="blueprint-link">开始创作</NuxtLink>
          </Motion>
        </div>

        <Motion v-bind="entranceProps(2)" class="motion-stagger-item home-cover__aside">
          <ul class="home-cover__marks" aria-hidden="true">
            <li class="home-cover__mark home-cover__arrow motion-parallax" :style="parallaxStyle">↗</li>
            <li class="home-cover__mark home-cover__capsule">VOL. 01</li>
            <li class="home-cover__mark home-cover__bracket">]</li>
          </ul>
          <ul class="home-cover__ticks" aria-hidden="true">
            <li v-for="tick in ['000', '025', '050', '075', '100']" :key="tick" class="home-cover__tick">
              {{ tick }}
            </li>
          </ul>
        </Motion>
      </div>

      <!-- 0 底图 + 1 罩层 + 6 控件：轮播并入首屏（≥2 张才出现玻璃胶囊控件；空态沿用既有组件） -->
      <BannerCarousel class="home-cover__media" :banners="banners" :pending="bannerPending" :photo-style="parallaxStyle" />
    </section>

    <div class="hero-grid">
      <div class="page-stack">
        <HomePinnedSection
          :articles="topArticles"
          :pending="topPending"
          :failed="topFailed"
          @retry="handleRetryTop"
        />

        <HomeFeedSection
          :articles="latestArticles"
          :pending="latestPending"
          :failed="latestFailed"
          @retry="handleRetryLatest"
        >
          <template #footer>
            <div
              v-if="latestPage > 1 || latestHasNext"
              class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-line-faint pt-3 text-sm text-ink-muted"
              data-role="feed-pager"
            >
              <span>已访问页会复用缓存，翻页不重复请求旧页。</span>
              <div class="flex items-center gap-3 blueprint-actions">
                <NButton quaternary :disabled="!latestHasPrevious" @click="handlePreviousLatestPage">上一页</NButton>
                <NButton
                  type="primary"
                  ghost
                  :disabled="!latestHasNext"
                  :loading="latestPending"
                  @click="handleNextLatestPage"
                >
                  下一页
                </NButton>
              </div>
            </div>
          </template>
        </HomeFeedSection>
      </div>

      <HomeSidebar
        :pending="siteBootstrapPending || bannerPending"
        :runtime-config="siteStore.runtimeConfig"
        :articles="latestArticles"
        sticky
      />
    </div>
  </div>
</template>

<style scoped>
/*
 * 首屏封面样式集中在同一个文件里（ADR-0013 约束 1/3）。
 *
 * 满幅实现：width:100vw + margin-inline:calc(50% - 50vw) 让封面脱离 page-shell 的
 * max-width/px 内边距；1px 的视口溢出不在这里自裁，而是由布局层（site-layout）
 * 的 overflow-x: clip 兜住，避免桌面上多出一条横向滚动条。
 * 没有卡片边框、没有纸面底色：图像缺失时坐在深墨兜底面上（与文章封面兜底同一口径），
 * 所以反白文字在任何情况下都满足对比度。
 */
.home-cover {
  position: relative;
  isolation: isolate;
  display: flex;
  flex-direction: column;
  justify-content: center;
  width: 100vw;
  max-width: 100vw;
  min-height: clamp(380px, 62vh, 620px);
  margin-inline: calc(50% - 50vw);
  overflow: clip;
  background: var(--color-surface-inverse);
  color: var(--color-on-inverse);
  padding: clamp(1.5rem, 4vw, 3rem) clamp(1rem, 5vw, 4rem);
}

/* ---- 图层 2：色带（仅首屏可出现的装饰；黄 / 功能蓝 / 珊瑚三色，不得第四色） ---- */
.home-cover__bands {
  position: absolute;
  inset: 0;
  z-index: 2;
  overflow: hidden;
  pointer-events: none;
}

.home-cover__band {
  position: absolute;
  display: block;
  height: clamp(8px, 1.1vw, 14px);
}

.home-cover__band--yellow {
  top: 0;
  left: 0;
  width: 100%;
  background: var(--jelly-yellow);
}

.home-cover__band--blue {
  top: 52%;
  right: 0;
  width: 28%;
  background: var(--jelly-interactive);
}

.home-cover__band--coral {
  bottom: 0;
  left: 0;
  width: 26%;
  background: var(--jelly-coral);
}

/* ---- 图层 3：网格线与下沿刻度（≥1024px 才显示） ---- */
.home-cover__grid {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: none;
  pointer-events: none;
  background-image: linear-gradient(to right, var(--color-line-faint) 1px, transparent 1px);
  background-size: 25% 100%;
}

.home-cover__grid-line {
  position: absolute;
  display: block;
  background: var(--color-line-faint);
}

.home-cover__grid-line--a {
  inset-block: 0;
  left: 0;
  width: 1px;
}

.home-cover__grid-line--b {
  inset-block: 0;
  right: 0;
  width: 1px;
}

.home-cover__grid-line--c {
  inset-inline: 0;
  bottom: 18%;
  height: 1px;
}

/* ---- 图层 4/5：两栏内容（左文字 / 右刻度 + 记号） ---- */
.home-cover__inner {
  position: relative;
  z-index: 4;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(9rem, 16rem);
  align-items: end;
  gap: clamp(1rem, 3vw, 3rem);
  width: 100%;
}

.home-cover__copy {
  min-width: 0;
}

/* 11px 小字从"页头/首屏 h1"降为普通文本：颜色跟着反白层走（不再用纸面次级墨）。 */
.home-cover__eyebrow {
  color: var(--color-on-inverse-muted);
}

.home-cover__title {
  font-family: var(--font-title);
  font-size: clamp(2rem, 9vw, 6rem);
  font-weight: var(--type-weight-title);
  line-height: 0.92;
  letter-spacing: -0.04em;
  color: var(--color-on-inverse);
  text-wrap: balance;
}

.home-cover__title-name {
  display: inline-block;
}

.home-cover__title-bracket {
  display: inline-block;
  margin-inline-start: 0.5em;
  font-size: clamp(0.95rem, 2.4vw, 1.75rem);
  font-weight: 400;
  letter-spacing: 0;
  color: var(--color-on-inverse-muted);
}

.home-cover__lede {
  margin-top: 1rem;
  max-width: 40rem;
  font-size: 0.9375rem;
  line-height: 1.8;
  color: var(--color-on-inverse-muted);
}

.home-cover__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.75rem;
  margin-top: 1.25rem;
}

/*
 * 两个入口沿用既有 blueprint 控件，但首屏是深墨面/底图：把默认的"纸面 + 墨字"
 * 换成反白语义色，否则反白封面上的深色文字读不出来（对比度由 on-inverse 保证）。
 */
.home-cover__actions .blueprint-tab {
  border-color: var(--color-on-inverse-muted);
  background: transparent;
  color: var(--color-on-inverse);
}

.home-cover__actions .blueprint-tab:hover {
  border-color: var(--color-on-inverse);
  color: var(--color-on-inverse);
}

.home-cover__actions .blueprint-link {
  color: var(--color-on-inverse);
  text-decoration-color: var(--color-on-inverse-muted);
}

.home-cover__aside {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: flex-end;
  gap: 1.25rem;
}

.home-cover__marks {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.home-cover__mark {
  color: var(--color-on-inverse);
  line-height: 1;
}

.home-cover__arrow {
  font-size: clamp(1.5rem, 3vw, 2.5rem);
}

.home-cover__bracket {
  font-family: var(--font-title);
  font-size: clamp(2rem, 5vw, 4rem);
  color: var(--color-on-inverse-muted);
}

/* 黑底白字胶囊小签（纯装饰，aria-hidden） */
.home-cover__capsule {
  padding: 0.35rem 0.75rem;
  border: 1px solid var(--color-on-inverse-muted);
  border-radius: var(--radius-pill);
  background: var(--color-surface-inverse);
  color: var(--color-on-inverse);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.12em;
}

.home-cover__ticks {
  display: flex;
  width: 100%;
  justify-content: space-between;
  margin: 0;
  padding: 0.5rem 0 0;
  border-top: 1px solid var(--color-on-inverse-muted);
  list-style: none;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.08em;
  color: var(--color-on-inverse-muted);
}

.home-cover__tick {
  min-width: 0;
}

/* ---- 图层 0/1/6：轮播媒体层（底图 + 罩层 + 玻璃胶囊控件）由 BannerCarousel 渲染 ---- */
.home-cover__media {
  display: block;
}

@media (min-width: 1024px) {
  .home-cover__grid {
    display: block;
  }
}

@media (max-width: 767px) {
  .home-cover__inner {
    grid-template-columns: minmax(0, 1fr);
    align-items: start;
  }

  .home-cover__title {
    font-size: clamp(2rem, 9vw, 3rem);
  }

  .home-cover__aside {
    align-items: flex-start;
  }

  .home-cover__grid {
    display: none;
  }

  .home-cover__ticks {
    display: none;
  }

  .home-cover__bracket {
    display: none;
  }

  .home-cover__arrow {
    display: none;
  }

  .home-cover__band--blue {
    display: none;
  }

  .home-cover__band--coral {
    display: none;
  }
}
</style>
