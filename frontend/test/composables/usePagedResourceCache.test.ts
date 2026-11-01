import { effectScope, nextTick, shallowRef } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { usePagedResourceCache } from "~/composables/usePagedResourceCache";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import { withoutPrivateFieldsFromItems } from "~/utils/readerPrivateFields";

const scopes: Array<ReturnType<typeof effectScope>> = [];

async function runInScope<T>(factory: () => Promise<T>) {
  const scope = effectScope();
  scopes.push(scope);
  return await scope.run(factory);
}

describe("usePagedResourceCache", () => {
  afterEach(() => {
    while (scopes.length) {
      scopes.pop()?.stop();
    }
  });

  it("loads the initial page and caches previously loaded pages", async () => {
    const fetchPage = vi.fn(async (page: number, limit: number) => ({
      items: [`page-${page}`, `limit-${limit}`],
      hasMore: page < 2,
    }));

    const cache = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "articles",
        pageSize: 9,
        fetchPage,
      }),
    );

    expect(cache.currentItems.value).toEqual(["page-1", "limit-9"]);
    expect(cache.hasNextPage.value).toBe(true);

    await cache.goToNextPage();
    expect(cache.currentPage.value).toBe(2);
    expect(cache.currentItems.value).toEqual(["page-2", "limit-9"]);

    await cache.goToPage(1);
    expect(cache.currentItems.value).toEqual(["page-1", "limit-9"]);
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });

  it("resets and reloads when the cache key changes", async () => {
    const cacheKey = shallowRef("comments");
    const fetchPage = vi.fn(async (page: number) => ({
      items: [`${cacheKey.value}-${page}`],
      hasMore: false,
    }));

    const cache = await runInScope(() =>
      usePagedResourceCache({
        cacheKey,
        pageSize: 5,
        fetchPage,
      }),
    );

    expect(cache.currentItems.value).toEqual(["comments-1"]);

    cacheKey.value = "global";
    await nextTick();
    await flushPromises();

    expect(cache.currentPage.value).toBe(1);
    expect(cache.currentItems.value).toEqual(["global-1"]);
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });

  // 首页与文章评论区要在客户端路由切换时"先出骨架"：初始化取数不能再挡住 setup。
  // background = 立刻给句柄（pending 已置真、骨架可观察），首屏请求在后台跑完，
  // `ready` 是给服务端用的"首屏已落定"信号（SSR 的 HTML 里必须有内容）。
  it("background 初始化：立即返回句柄，pending 可观察，ready 在首屏落定后才解析", async () => {
    let releaseFirstPage!: (page: { items: string[]; hasMore: boolean }) => void;
    const fetchPage = vi.fn(
      () =>
        new Promise<{ items: string[]; hasMore: boolean }>((resolve) => {
          releaseFirstPage = resolve;
        }),
    );

    const settled = await Promise.race([
      runInScope(() =>
        usePagedResourceCache({
          cacheKey: "home-background",
          pageSize: 9,
          fetchPage,
          initialLoad: "background",
        }),
      ),
      new Promise<"blocked">((resolve) => setTimeout(() => resolve("blocked"), 200)),
    ]);

    expect(settled).not.toBe("blocked");
    const cache = settled as Awaited<ReturnType<typeof usePagedResourceCache<string>>>;
    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(cache.pending.value).toBe(true);
    expect(cache.currentItems.value).toEqual([]);

    let readyResolved = false;
    const whenReady = cache.ready.then(() => {
      readyResolved = true;
    });
    expect(readyResolved).toBe(false);

    releaseFirstPage({ items: ["first"], hasMore: true });
    await flushPromises();
    await whenReady;

    expect(readyResolved).toBe(true);
    expect(cache.currentItems.value).toEqual(["first"]);
    expect(cache.pending.value).toBe(false);
  });

  it("keeps the current page unchanged when trying to move before page one", async () => {
    const fetchPage = vi.fn(async () => ({
      items: ["only-page"],
      hasMore: false,
    }));

    const cache = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "single",
        pageSize: 5,
        fetchPage,
      }),
    );

    await expect(cache.goToPreviousPage()).resolves.toBeNull();
    expect(cache.currentPage.value).toBe(1);
    expect(cache.hasPreviousPage.value).toBe(false);
  });

  // ---- 跨导航 30s SWR（票 #85） ----
  // 观众路径：首页 → 文章 → 返回首页，30 秒内首屏不该再走一遍请求。
  // 这里用注入的假时钟把时间推过 30 秒，不真的 sleep。
  function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((done) => {
      resolve = done;
    });
    return { promise, resolve };
  }

  it("跨导航命中：30 秒内重新实例化直接命中，不再请求", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    const firstFetch = vi.fn(async (page: number) => ({
      items: [`page-${page}`],
      hasMore: page < 2,
    }));

    const first = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: firstFetch,
        swrCache: cache,
      }),
    );
    expect(first.currentItems.value).toEqual(["page-1"]);

    const secondFetch = vi.fn(async () => ({ items: ["network"], hasMore: false }));
    const second = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: secondFetch,
        swrCache: cache,
      }),
    );

    expect(secondFetch).not.toHaveBeenCalled();
    expect(second.currentItems.value).toEqual(["page-1"]);
    expect(second.pending.value).toBe(false);
    expect(second.revalidating.value).toBe(false);
  });

  it("超过 30 秒：旧值先上屏，后台静默刷新期间 pending 保持 false（不闪）", async () => {
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: async () => ({ items: ["旧页面"], hasMore: false }),
        swrCache: cache,
      }),
    );

    clock = 1_000 + 30_001;
    const refresh = deferred<{ items: string[]; hasMore: boolean }>();
    const secondFetch = vi.fn(() => refresh.promise);
    const second = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: secondFetch,
        swrCache: cache,
      }),
    );

    // 命中旧值立刻能渲染；重验证在后台跑，不进入 pending（界面不闪）。
    expect(second.currentItems.value).toEqual(["旧页面"]);
    expect(second.pending.value).toBe(false);
    expect(second.revalidating.value).toBe(true);
    expect(secondFetch).toHaveBeenCalledTimes(1);

    refresh.resolve({ items: ["新页面"], hasMore: false });
    await flushPromises();

    expect(second.currentItems.value).toEqual(["新页面"]);
    expect(second.revalidating.value).toBe(false);
  });

  it("缓存键含路由参数：不同文章 id 不串数据", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "article-comments:a1",
        pageSize: 7,
        fetchPage: async () => ({ items: ["a1 的评论"], hasMore: false }),
        swrCache: cache,
      }),
    );

    const secondFetch = vi.fn(async () => ({ items: ["a2 的评论"], hasMore: false }));
    const second = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "article-comments:a2",
        pageSize: 7,
        fetchPage: secondFetch,
        swrCache: cache,
      }),
    );

    expect(secondFetch).toHaveBeenCalledTimes(1);
    expect(second.currentItems.value).toEqual(["a2 的评论"]);
  });

  it("缓存键含页码：返回后翻过的第 2 页也能命中", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    let fetched = 0;
    const first = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: async (page: number) => {
          fetched += 1;
          return { items: [`page-${page}`], hasMore: page < 2 };
        },
        swrCache: cache,
      }),
    );
    await first.goToNextPage();
    expect(first.currentItems.value).toEqual(["page-2"]);

    const secondFetch = vi.fn(async () => ({ items: ["network"], hasMore: false }));
    const second = await runInScope(() =>
      usePagedResourceCache({
        cacheKey: "home-latest-articles",
        pageSize: 9,
        fetchPage: secondFetch,
        swrCache: cache,
      }),
    );
    await second.goToNextPage();

    expect(secondFetch).not.toHaveBeenCalled();
    expect(second.currentPage.value).toBe(2);
    expect(second.currentItems.value).toEqual(["page-2"]);
    expect(fetched).toBe(2);
  });

  it("键变化期间迟到的旧请求不会写进新键的缓存", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    const cacheKey = shallowRef("search:甲");
    const slowOldKey = deferred<{ items: string[]; hasMore: boolean }>();
    const fetchPage = vi.fn((_page: number) =>
      cacheKey.value === "search:甲"
        ? slowOldKey.promise
        : Promise.resolve({ items: ["乙的结果"], hasMore: false }),
    );

    const pager = await runInScope(() =>
      usePagedResourceCache({
        cacheKey,
        pageSize: 9,
        fetchPage,
        initialLoad: "background",
        swrCache: cache,
      }),
    );
    await flushPromises();
    // 第一个请求还悬着，搜索词已经切到"乙"。
    cacheKey.value = "search:乙";
    await nextTick();
    await flushPromises();
    expect(pager.currentItems.value).toEqual(["乙的结果"]);

    slowOldKey.resolve({ items: ["甲的结果"], hasMore: false });
    await flushPromises();

    // 迟到的旧结果既不覆盖当前页面，也没污染新键的缓存。
    expect(pager.currentItems.value).toEqual(["乙的结果"]);
    const newKey = cache.read<{ items: string[] }>("search:乙#1");
    expect(newKey.state !== "miss" ? newKey.value.items : []).toEqual(["乙的结果"]);
    expect(cache.read("search:甲#1")).toEqual({ state: "miss" });
  });

  it("写缓存前剔除私有字段：本次渲染仍是完整数据，缓存里没有 is_digg / relation", async () => {
    interface PrivateItem {
      id: string;
      is_digg: boolean;
      relation: number;
    }

    const cache = createReaderSwrCache({ now: () => 1_000 });
    const pager = await runInScope(() =>
      usePagedResourceCache<PrivateItem>({
        cacheKey: "article-comments:a1",
        pageSize: 7,
        fetchPage: async () => ({
          items: [{ id: "c1", is_digg: true, relation: 1 }],
          hasMore: false,
        }),
        swrCache: cache,
        sanitizeForCache: (page) => ({
          ...page,
          items: withoutPrivateFieldsFromItems(page.items),
        }),
      }),
    );

    expect(pager.currentItems.value[0]).toMatchObject({ id: "c1", is_digg: true });
    const cached = cache.read<{ items: Array<Record<string, unknown>> }>("article-comments:a1#1");
    expect(cached.state).toBe("fresh");
    expect(cached.state !== "miss" ? cached.value.items[0] : null).not.toHaveProperty("is_digg");
    expect(cached.state !== "miss" ? cached.value.items[0] : null).not.toHaveProperty("relation");
  });
});
