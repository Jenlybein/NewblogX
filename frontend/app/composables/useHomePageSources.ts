import { shallowRef } from "vue";
import { usePagedResourceCache } from "~/composables/usePagedResourceCache";
import { getHotTopArticles } from "~/services/article";
import { searchArticles } from "~/services/search";
import { getBannerList } from "~/services/site";
import type { ArticleHotTopListData, BannerListData, SearchArticleItem } from "~/types/api";
import { detectReaderRenderMode, type ReaderRenderMode } from "~/utils/readerRenderMode";

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
}

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
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
  const bannerSource = useAsyncData("home-banners", () =>
    fetchBanners().catch(() => ({ list: [], has_more: false } satisfies BannerListData)),
  );
  const hotTopSource = useAsyncData("home-hot-top", async () => {
    try {
      return await fetchHotTop();
    } catch (error) {
      console.warn(`[home-hot-top] request failed, using fallback: ${formatRequestError(error)}`);
      return { list: [], count: 0 } satisfies ArticleHotTopListData;
    }
  });

  const latestPager = await usePagedResourceCache({
    cacheKey: () => "home-latest-articles",
    pageSize: () => 9,
    // 客户端路由跳转时首屏交给后台跑（先出骨架）；服务端与首屏 hydration 等它（SSR 内容 / 不闪）。
    initialLoad: mode === "navigation" ? "background" : "block",
    fetchPage: async (page, limit) => {
      try {
        latestRequestError.value = null;
        return await fetchLatest(page, limit);
      } catch (error) {
        latestRequestError.value = error;
        console.warn(`[home-latest-articles] request failed, using fallback: ${formatRequestError(error)}`);
        return { items: [], hasMore: false };
      }
    },
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
