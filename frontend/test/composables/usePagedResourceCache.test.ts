import { effectScope, nextTick, shallowRef } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { usePagedResourceCache } from "~/composables/usePagedResourceCache";

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
});
