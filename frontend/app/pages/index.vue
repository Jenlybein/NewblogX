<script setup lang="ts">
import { computed, shallowRef } from "vue";
import { NButton } from "naive-ui";
import BannerCarousel from "~/components/article/BannerCarousel.vue";
import HomeFeedSection from "~/components/home/HomeFeedSection.vue";
import HomePinnedSection from "~/components/home/HomePinnedSection.vue";
import HomeSidebar from "~/components/home/HomeSidebar.vue";
import { useStaggerEntrance } from "~/composables/useStaggerEntrance";
import { getTopArticles } from "~/services/article";
import { searchArticles } from "~/services/search";
import { getBannerList } from "~/services/site";
import { BRAND, brandDescription, brandFullName } from "~/utils/brand";
import { homeHeroFacts, homeSearchLocation } from "~/utils/home";

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
const heroFacts = computed(() =>
  homeHeroFacts({
    topCount: topArticles.value.length,
    latestCount: latestArticles.value.length,
    pageSize: 9,
  }),
);

// 首屏分级入场：标题/CTA 与事实条错峰 40ms，只动 opacity/translateY；reduce 时归零。
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
    <section class="home-hero" data-home-hero>
      <div class="home-hero__intro">
        <Motion v-bind="entranceProps(0)" class="motion-stagger-item">
          <div class="eyebrow">{{ BRAND.name.en }} · {{ BRAND.name.zh }}</div>
          <h1 class="home-hero__title">{{ BRAND.tagline }}</h1>
          <p class="home-hero__lede">{{ brandDescription(siteStore.seo?.description) }}</p>
        </Motion>

        <Motion v-bind="entranceProps(1)" class="motion-stagger-item home-hero__actions">
          <NuxtLink :to="searchTo" class="blueprint-tab">搜索文章</NuxtLink>
          <NuxtLink to="/studio/write" class="blueprint-link">开始创作</NuxtLink>
        </Motion>
      </div>

      <dl class="home-hero__facts">
        <div v-for="fact in heroFacts" :key="fact.label" class="home-hero__fact">
          <dt>{{ fact.label }}</dt>
          <dd>{{ fact.value }}</dd>
        </div>
      </dl>
    </section>

    <div class="hero-grid">
      <div class="page-stack">
        <BannerCarousel :banners="banners" :pending="bannerPending" />

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
/* 首屏 = 图纸的第一格：一句主张 + 三格事实 + 两个入口；颜色全部来自 token。 */
.home-hero {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1.25rem;
  border: 1px solid var(--color-line);
  background: var(--color-surface);
  padding: 1.25rem;
}

.home-hero__intro {
  flex: 1 1 320px;
  min-width: 0;
}

.home-hero__title {
  font-family: var(--font-title);
  font-size: clamp(1.9rem, 4vw, 2.75rem);
  font-weight: 400;
  line-height: 1.12;
  letter-spacing: var(--type-tracking-title);
  color: var(--color-ink);
  text-wrap: balance;
}

.home-hero__lede {
  margin-top: 0.75rem;
  max-width: 46rem;
  font-size: 0.875rem;
  line-height: 1.8;
  color: var(--color-ink-muted);
}

.home-hero__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.75rem;
  margin-top: 1.25rem;
}

.home-hero__facts {
  display: grid;
  width: 100%;
  max-width: 22rem;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-self: flex-start;
  border: 1px solid var(--color-line-faint);
}

.home-hero__fact {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border-inline-start: 1px solid var(--color-line-faint);
  padding: 0.75rem 1rem;
}

.home-hero__fact:first-child {
  border-inline-start: 0;
}

.home-hero__fact dt {
  font-size: 0.75rem;
  color: var(--color-ink-muted);
}

.home-hero__fact dd {
  margin: 0;
  font-family: var(--font-title);
  font-size: 1.25rem;
  line-height: 1.1;
  color: var(--color-ink);
}

@media (min-width: 768px) {
  .home-hero {
    padding: 1.5rem;
  }
}
</style>
