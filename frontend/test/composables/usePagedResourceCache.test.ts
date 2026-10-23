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
