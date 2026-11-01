import { shallowRef } from "vue";
import { usePagedResourceCache } from "~/composables/usePagedResourceCache";
import { useReaderSwrAsyncData } from "~/composables/useReaderSwrAsyncData";
import { getHotTopArticles } from "~/services/article";
import { searchArticles } from "~/services/search";
import { getBannerList } from "~/services/site";
import type { ArticleHotTopListData, BannerListData, SearchArticleItem } from "~/types/api";
import { detectReaderRenderMode, type ReaderRenderMode } from "~/utils/readerRenderMode";
import { READER_HOME_CACHE_KEYS } from "~/utils/readerCacheKeys";
import type { ReaderSwrCache } from "~/utils/readerSwrCache";

export interface HomeLatestPage {
  items: SearchArticleItem[];
  hasMore: boolean;
}

export interface HomePageSourcesOptions {
  /** 覆盖运行环境判定（单测注入）；默认按 import.meta + isHydrating 判定。 */
  mode?: ReaderRenderMode;
  /** 三个取数入口可注入（单测用假请求，断言"谁在什么时候发起"）。 */
  fetchBanners?: () => Promise<BannerListData>;
  fetchHotTop?: () => Promise<ArticleHotTopListData>;
  fetchLatest?: (page: number, limit: number) => Promise<HomeLatestPage>;
  /** 跨导航 30s SWR 缓存（单测注入假时钟实例；默认客户端共享实例、服务端禁用）。 */
  swrCache?: ReaderSwrCache;
}

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
}

/**
 * 三路数据的失败语义收敛在一处（ADR-0017 决策 5：某个接口挂了只影响它自己那块）。
 * 成功返回原值；失败打一条带资源名的 warning、返回稳定的降级值，绝不把整页带崩。
 */
async function withFallback<TValue>(
  label: string,
  fallback: TValue,
  run: () => Promise<TValue>,
  onError?: (error: unknown) => void,
): Promise<TValue> {
  try {
    return await run();
  } catch (error) {
    onError?.(error);
    console.warn(`[${label}] request failed, using fallback: ${formatRequestError(error)}`);
    return fallback;
  }
}

/**
 * 首页的三个数据源（轮播 / 热门置顶 / 最新文章）。
 *
 * 病根在票面里：以前是 `await banners → await hot-top → await 最新文章`，三个请求首尾相接，
 * 墙钟等于三段之和。这里改成**先全部发起、再按环境决定等谁**：
 *
 * - 服务端：三个都等齐（SSR 的 HTML 里必须有轮播、置顶与最新列表）；
 * - 客户端跳转：一个都不等，setup 立刻返回，页面先画骨架；
 * - 首屏 hydration：只有分页首屏（最新文章）要等 —— 它不在 payload 里。其余复用 payload。
 *
 * 失败语义不变：轮播 / 置顶 / 最新都"失败降级不抛错"，某个接口挂了只影响它自己那块
 * （区块组件拿不到数据就整块不渲染，ADR-0017 决策 5）。
 */
export async function useHomePageSources(options: HomePageSourcesOptions = {}) {
  const mode = options.mode ?? detectReaderRenderMode();
  const latestRequestError = shallowRef<unknown>(null);

  const fetchBanners = options.fetchBanners ?? (() => getBannerList());
  const fetchHotTop = options.fetchHotTop ?? (() => getHotTopArticles());
  const fetchLatest =
    options.fetchLatest ??
    (async (page: number, limit: number) => {
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
    });

  // 三个数据源在这里**同时发起**：顺序只决定谁先占 socket，不决定谁等谁。
  // 轮播 / 置顶走 30s SWR：返回首页 30 秒内直接命中缓存、不发请求；过期则旧值先上屏、
  // 后台静默刷新（ADR-0019 决策 4）。
  const bannerSource = useReaderSwrAsyncData({
    key: READER_HOME_CACHE_KEYS.banners,
    handler: () =>
      withFallback("home-banners", { list: [], has_more: false } satisfies BannerListData, () => fetchBanners()),
    mode,
    cache: options.swrCache,
  });
  const hotTopSource = useReaderSwrAsyncData({
    key: READER_HOME_CACHE_KEYS.hotTop,
    handler: () =>
      withFallback("home-hot-top", { list: [], count: 0 } satisfies ArticleHotTopListData, () => fetchHotTop()),
    mode,
    cache: options.swrCache,
  });

  const latestPager = await usePagedResourceCache({
    cacheKey: () => READER_HOME_CACHE_KEYS.latestArticles,
    pageSize: () => 9,
    // 客户端路由跳转时首屏交给后台跑（先出骨架）；服务端与首屏 hydration 等它（SSR 内容 / 不闪）。
    initialLoad: mode === "navigation" ? "background" : "block",
    // 分页缓存：30 秒内返回首页直接命中（含翻过的页）；服务端渲染禁用。
    swrCache: mode === "server" ? null : options.swrCache,
    fetchPage: (page, limit) =>
      withFallback(
        "home-latest-articles",
        { items: [], hasMore: false },
        async () => {
          latestRequestError.value = null;
          return await fetchLatest(page, limit);
        },
        (error) => {
          latestRequestError.value = error;
        },
      ),
  });

  if (mode === "server") {
    // 服务端等齐：除了分页首屏（上面 block 已经等过），还有两个 useAsyncData 源。
    await Promise.all([bannerSource, hotTopSource, latestPager.ready]);
  }

  return {
    bannerData: bannerSource.data,
    bannerPending: bannerSource.pending,
    hotTopData: hotTopSource.data,
    latestPager,
    latestRequestError,
  };
}
