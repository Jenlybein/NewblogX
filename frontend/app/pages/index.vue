<script setup lang="ts">
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import BannerCarousel from "~/components/article/BannerCarousel.vue";
import HomeSidebar from "~/components/home/HomeSidebar.vue";
import { computed, shallowRef } from "vue";
import { NButton, NSkeleton } from "naive-ui";
import { getTopArticles } from "~/services/article";
import { searchArticles } from "~/services/search";
import { getBannerList } from "~/services/site";
import { formatCount } from "~/utils/format";

const siteStore = useSiteStore();
const topRequestError = shallowRef<unknown>(null);
const latestRequestError = shallowRef<unknown>(null);

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
}

const { data: bannerData, pending: bannerPending } = await useAsyncData("home-banners", () =>
  getBannerList().catch(() => ({ list: [], has_more: false })),
);
const { data: topData, pending: topPending } = await useAsyncData("home-top-articles", async () => {
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
  title: siteStore.seo?.site_title || "BlogX",
  description: siteStore.seo?.description || "开发者内容平台",
  keywords: siteStore.seo?.keywords || "前端, Nuxt, Vue, OpenAPI",
  ogTitle: siteStore.seo?.site_title || "BlogX",
  ogDescription: siteStore.seo?.description || "开发者内容平台",
});

const banners = computed(() => bannerData.value?.list || []);
const topArticles = computed(() => topData.value?.list || []);
const latestArticles = computed(() => latestPager.currentItems.value);
const latestPending = computed(() => latestPager.pending.value);
const siteBootstrapPending = computed(() => !siteStore.fetched);

async function handlePreviousLatestPage() {
  await latestPager.goToPreviousPage();
}

async function handleNextLatestPage() {
  await latestPager.goToNextPage();
}
</script>

<template>
  <div class="page-stack">
    <div class="hero-grid">
      <div class="page-stack">
        <BannerCarousel :banners="banners" />

        <section v-if="topArticles.length" class="surface-card p-5 md:p-6">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <div class="eyebrow">Pinned</div>
              <h2 class="section-title mt-2">热门置顶</h2>
            </div>
            <div class="glass-badge">{{ topArticles.length }} 篇</div>
          </div>

          <div class="space-y-4">
            <article
              v-for="article in topArticles"
              :key="article.id"
              class="home-pinned-article line-divider pt-4 first:border-0 first:pt-0"
            >
              <NuxtLink :to="`/article/${article.id}`" class="home-pinned-article__cover">
                <ArticleCover :src="article.cover" :title="article.title" label="Pinned" />
              </NuxtLink>
              <NuxtLink :to="`/article/${article.id}`" class="block min-w-0">
                <div class="text-lg font-semibold leading-snug">{{ article.title }}</div>
                <p class="mt-2 line-clamp-2 text-sm leading-7 muted">{{ article.abstract }}</p>
                <div class="mt-3 flex flex-wrap items-center gap-3 text-sm muted">
                  <span>{{ article.user_nickname }}</span>
                  <span>{{ article.category_title || "未分类" }}</span>
                  <span>{{ formatCount(article.view_count) }} 阅读</span>
                </div>
              </NuxtLink>
            </article>
          </div>
        </section>
        <section v-else-if="topPending" class="surface-card p-5 md:p-6">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <div class="eyebrow">Pinned</div>
              <h2 class="section-title mt-2">热门置顶</h2>
            </div>
            <div class="glass-badge">加载中</div>
          </div>
          <div class="space-y-4">
            <div v-for="idx in 3" :key="`top-skeleton-${idx}`" class="line-divider pt-4 first:border-0 first:pt-0">
              <NSkeleton text width="80%" height="24px" />
              <NSkeleton text class="mt-3" :repeat="2" />
            </div>
          </div>
        </section>

        <section class="surface-card p-5 md:p-6">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <div class="eyebrow">Latest</div>
              <h2 class="section-title mt-2">最新文章</h2>
            </div>
            <div class="flex items-center gap-3">
              <div class="glass-badge">第 {{ latestPager.currentPage }} 页</div>
              <NuxtLink to="/search" class="glass-badge">进入搜索</NuxtLink>
            </div>
          </div>

          <div v-if="latestArticles.length" class="space-y-4">
            <ArticleFeedItem
              v-for="article in latestArticles"
              :key="article.id"
              :article="article"
              compact
            />

            <div class="flex flex-wrap items-center justify-between gap-3 border-t border-white/60 pt-5 text-sm muted">
              <span>每页 9 篇，本次已缓存 {{ Object.keys(latestPager.pages).length }} 页。未刷新前会直接复用已访问页。</span>
              <div class="flex items-center gap-3">
                <NButton quaternary :disabled="!latestPager.hasPreviousPage" @click="handlePreviousLatestPage">
                  上一页
                </NButton>
                <NButton
                  type="primary"
                  ghost
                  :disabled="!latestPager.hasNextPage"
                  :loading="latestPending"
                  @click="handleNextLatestPage"
                >
                  下一页
                </NButton>
              </div>
            </div>
          </div>
          <div v-else-if="latestPending" class="space-y-4">
            <article
              v-for="idx in 4"
              :key="`latest-skeleton-${idx}`"
              class="rounded-2xl border border-slate-200/70 bg-white/75 p-4"
            >
              <NSkeleton text width="66%" height="24px" />
              <NSkeleton text class="mt-3" :repeat="2" />
              <NSkeleton text width="50%" class="mt-2" />
            </article>
          </div>
          <div v-else class="surface-section flex min-h-[220px] items-center justify-center p-6 text-sm muted">
            {{
              latestRequestError
                ? "文章加载失败，请检查前端 API 地址或测试环境状态。"
                : "暂时没有可展示的公开文章。"
            }}
          </div>
        </section>
      </div>

      <HomeSidebar
        :pending="siteBootstrapPending || bannerPending"
        :runtime-config="siteStore.runtimeConfig"
        :ai-info="siteStore.aiInfo"
        :articles="latestArticles"
      />
    </div>
  </div>
</template>

<style scoped>
.home-pinned-article {
  display: grid;
  grid-template-columns: 124px minmax(0, 1fr);
  gap: 14px;
  align-items: stretch;
}

.home-pinned-article__cover {
  display: block;
  min-height: 88px;
  overflow: hidden;
  border-radius: 16px;
}

@media (max-width: 640px) {
  .home-pinned-article {
    grid-template-columns: 1fr;
  }

  .home-pinned-article__cover {
    min-height: 164px;
  }
}
</style>
