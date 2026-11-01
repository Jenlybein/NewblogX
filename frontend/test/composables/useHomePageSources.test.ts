import { flushPromises } from "@vue/test-utils";
import { effectScope } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useHomePageSources } from "~/composables/useHomePageSources";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import type { ArticleHotTopListData, BannerListData, SearchArticleItem } from "~/types/api";
import { installAwaitableAsyncData } from "../mocks/awaitableAsyncData";

const scopes: Array<ReturnType<typeof effectScope>> = [];

async function runInScope<T>(factory: () => Promise<T>) {
  const scope = effectScope();
  scopes.push(scope);
  return await scope.run(factory);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function blockedAfter(ms = 200) {
  return new Promise<"blocked">((resolve) => setTimeout(() => resolve("blocked"), ms));
}

afterEach(() => {
  while (scopes.length) {
    scopes.pop()?.stop();
  }
});

describe("useHomePageSources：首页三个数据源并行 + 骨架优先", () => {
  it("客户端跳转：三个请求在任一返回前都已发起，句柄先回来（骨架状态可观察）", async () => {
    installAwaitableAsyncData();

    const starts: string[] = [];
    const banners = deferred<BannerListData>();
    const hotTop = deferred<ArticleHotTopListData>();
    const latest = deferred<{ items: SearchArticleItem[]; hasMore: boolean }>();

    const fetchBanners = vi.fn(() => {
      starts.push("banners");
      return banners.promise;
    });
    const fetchHotTop = vi.fn(() => {
      starts.push("hot-top");
      return hotTop.promise;
    });
    const fetchLatest = vi.fn(() => {
      starts.push("latest");
      return latest.promise;
    });

    const pendingSources = runInScope(() =>
      useHomePageSources({
        mode: "navigation",
        fetchBanners,
        fetchHotTop,
        fetchLatest,
      }),
    );

    const settled = await Promise.race([pendingSources, blockedAfter()]);
    expect(settled).not.toBe("blocked");
    const sources = settled as Awaited<typeof pendingSources>;

    // 并行的证据：最早发起的那一个还没返回，另外两个已经在路上（串行实现这里只会有一项）。
    expect(starts.slice().sort()).toEqual(["banners", "hot-top", "latest"]);
    expect(fetchBanners).toHaveBeenCalledTimes(1);
    expect(fetchHotTop).toHaveBeenCalledTimes(1);
    expect(fetchLatest).toHaveBeenCalledTimes(1);

    // 骨架状态：轮播与最新列表都还是"数据未到"。
    expect(sources.bannerPending.value).toBe(true);
    expect(sources.latestPager.pending.value).toBe(true);
    expect(sources.latestPager.currentItems.value).toEqual([]);

    banners.resolve({ list: [], has_more: false });
    hotTop.resolve({ list: [], count: 0 });
    latest.resolve({ items: [{ id: "a1" } as SearchArticleItem], hasMore: false });
    await flushPromises();

    expect(sources.bannerData.value).toEqual({ list: [], has_more: false });
    expect(sources.hotTopData.value).toEqual({ list: [], count: 0 });
    expect(sources.latestPager.currentItems.value).toEqual([{ id: "a1" }]);
    expect(sources.bannerPending.value).toBe(false);
    expect(sources.latestPager.pending.value).toBe(false);
  });

  it("服务端：三个数据源都落定才返回（含后台模式的分页首屏），保证 SSR 的 HTML 里有内容", async () => {
    installAwaitableAsyncData();

    const banners = deferred<BannerListData>();
    const hotTop = deferred<ArticleHotTopListData>();
    const latest = deferred<{ items: SearchArticleItem[]; hasMore: boolean }>();
    const fetchLatest = vi.fn(() => latest.promise);

    const pendingSources = runInScope(() =>
      useHomePageSources({
        mode: "server",
        fetchBanners: () => banners.promise,
        fetchHotTop: () => hotTop.promise,
        fetchLatest,
      }),
    );

    let resolved = false;
    pendingSources.then(() => {
      resolved = true;
    });

    latest.resolve({ items: [], hasMore: false });
    banners.resolve({ list: [], has_more: false });
    await flushPromises();
    expect(resolved).toBe(false);

    hotTop.resolve({ list: [], count: 0 });
    await flushPromises();
    expect(resolved).toBe(true);

    const sources = await pendingSources;
    expect(sources.bannerData.value).toEqual({ list: [], has_more: false });
    expect(sources.hotTopData.value).toEqual({ list: [], count: 0 });
  });

  it("首屏 hydration：分页首屏等齐（不带 payload），异步源直接复用 payload 不拦 setup", async () => {
    installAwaitableAsyncData();

    const banners = deferred<BannerListData>();
    const hotTop = deferred<ArticleHotTopListData>();
    const latest = deferred<{ items: SearchArticleItem[]; hasMore: boolean }>();

    const pendingSources = runInScope(() =>
      useHomePageSources({
        mode: "hydration",
        fetchBanners: () => banners.promise,
        fetchHotTop: () => hotTop.promise,
        fetchLatest: () => latest.promise,
      }),
    );

    let resolved = false;
    pendingSources.then(() => {
      resolved = true;
    });

    latest.resolve({ items: [], hasMore: false });
    await flushPromises();
    expect(resolved).toBe(true);
  });

  // ---- 30s SWR（票 #85 验收项） ----
  // 观众路径：首页 → 文章 → 返回首页。30 秒内三个数据源都不该再发请求；
  // 超过 30 秒则旧值先上屏、后台静默刷新（界面不闪）。
  it("30 秒内返回首页：三个数据源全部命中缓存，不再发请求", async () => {
    installAwaitableAsyncData();
    const cache = createReaderSwrCache({ now: () => 1_000 });
    const bannerPayload: BannerListData = { list: [], has_more: false };
    const hotTopPayload: ArticleHotTopListData = { list: [], count: 0 };
    const latestPayload = {
      items: [{ id: "cached-article" } as SearchArticleItem],
      hasMore: false,
    };

    // 第一次进首页：三个源都真的取一次，把数据写进共享缓存。
    await runInScope(() =>
      useHomePageSources({
        mode: "navigation",
        swrCache: cache,
        fetchBanners: async () => bannerPayload,
        fetchHotTop: async () => hotTopPayload,
        fetchLatest: async () => latestPayload,
      }),
    );
    await flushPromises();
    await flushPromises();

    const fetchBanners = vi.fn(async () => ({ list: [], has_more: false }));
    const fetchHotTop = vi.fn(async () => ({ list: [], count: 0 }));
    const fetchLatest = vi.fn(async () => ({ items: [], hasMore: false }));
    const second = await runInScope(() =>
      useHomePageSources({
        mode: "navigation",
        swrCache: cache,
        fetchBanners,
        fetchHotTop,
        fetchLatest,
      }),
    );
    await flushPromises();

    expect(fetchBanners).not.toHaveBeenCalled();
    expect(fetchHotTop).not.toHaveBeenCalled();
    expect(fetchLatest).not.toHaveBeenCalled();
    expect(second.bannerData.value).toEqual(bannerPayload);
    expect(second.hotTopData.value).toEqual(hotTopPayload);
    expect(second.latestPager.currentItems.value).toEqual(latestPayload.items);
    expect(second.latestPager.pending.value).toBe(false);
  });

  it("超过 30 秒返回首页：旧值先渲染，后台静默刷新（不闪）", async () => {
    installAwaitableAsyncData();
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    const bannerPayload: BannerListData = { list: [], has_more: false };
    const hotTopPayload: ArticleHotTopListData = { list: [], count: 0 };
    const latestPayload = {
      items: [{ id: "old-article" } as SearchArticleItem],
      hasMore: false,
    };

    await runInScope(() =>
      useHomePageSources({
        mode: "navigation",
        swrCache: cache,
        fetchBanners: async () => bannerPayload,
        fetchHotTop: async () => hotTopPayload,
        fetchLatest: async () => latestPayload,
      }),
    );
    await flushPromises();
    await flushPromises();

    clock = 1_000 + 30_001;
    const banners = deferred<BannerListData>();
    const hotTop = deferred<ArticleHotTopListData>();
    const latest = deferred<{ items: SearchArticleItem[]; hasMore: boolean }>();
    const fetchBanners = vi.fn(() => banners.promise);
    const fetchHotTop = vi.fn(() => hotTop.promise);
    const fetchLatest = vi.fn(() => latest.promise);

    const second = await runInScope(() =>
      useHomePageSources({
        mode: "navigation",
        swrCache: cache,
        fetchBanners,
        fetchHotTop,
        fetchLatest,
      }),
    );
    await flushPromises();

    // 旧值已经上屏；静默刷新在后台跑，pending 保持 false。
    expect(second.bannerData.value).toEqual(bannerPayload);
    expect(second.hotTopData.value).toEqual(hotTopPayload);
    expect(second.latestPager.currentItems.value).toEqual(latestPayload.items);
    expect(second.latestPager.pending.value).toBe(false);
    expect(second.latestPager.revalidating.value).toBe(true);
    expect(fetchBanners).toHaveBeenCalledTimes(1);
    expect(fetchHotTop).toHaveBeenCalledTimes(1);
    expect(fetchLatest).toHaveBeenCalledTimes(1);

    const newBanner: BannerListData = { list: [], has_more: false };
    const newHotTop: ArticleHotTopListData = { list: [], count: 0 };
    const newLatest = { items: [{ id: "new-article" } as SearchArticleItem], hasMore: false };
    banners.resolve(newBanner);
    hotTop.resolve(newHotTop);
    latest.resolve(newLatest);
    await flushPromises();

    expect(second.bannerData.value).toEqual(newBanner);
    expect(second.latestPager.currentItems.value).toEqual(newLatest.items);
    expect(second.latestPager.revalidating.value).toBe(false);
  });
});
