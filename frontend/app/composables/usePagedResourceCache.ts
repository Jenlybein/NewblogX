import { computed, ref, shallowRef, toRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

export interface PagedResourcePage<TItem> {
  items: TItem[];
  hasMore: boolean;
}

/**
 * 初始化取数的调度方式（ADR-0019 决策 3：页面 setup 不得被取数挡住）。
 *
 * - `block`（默认）：等首屏取数回来才返回句柄 —— 既有调用方（首页 / 评论区）一直如此，
 *   保持向后兼容；服务端渲染也靠它保证 HTML 里有首屏内容。
 * - `background`：立刻返回句柄，首屏请求在后台跑完；`pending` 从调用瞬间就是 true，
 *   页面可以先把骨架画出来。`ready` 是"首屏已落定"的信号，服务端用它决定 SSR 要不要等。
 */
export type PagedResourceInitialLoad = "block" | "background";

interface UsePagedResourceCacheOptions<TItem> {
  cacheKey: MaybeRefOrGetter<string>;
  pageSize: MaybeRefOrGetter<number>;
  initialPage?: MaybeRefOrGetter<number>;
  immediate?: boolean;
  initialLoad?: MaybeRefOrGetter<PagedResourceInitialLoad>;
  fetchPage: (page: number, limit: number) => Promise<PagedResourcePage<TItem>>;
}

export async function usePagedResourceCache<TItem>(options: UsePagedResourceCacheOptions<TItem>) {
  const cacheKeyRef = toRef(() => toValue(options.cacheKey));
  const pageSizeRef = toRef(() => toValue(options.pageSize));
  const initialPageRef = toRef(() => toValue(options.initialPage) || 1);
  const immediateRef = toRef(() => toValue(options.immediate) ?? true);
  const initialLoadRef = toRef(() => toValue(options.initialLoad) ?? "block");

  const currentPage = ref(initialPageRef.value);
  const pages = shallowRef<Record<number, PagedResourcePage<TItem>>>({});
  const pending = ref(false);
  const error = shallowRef<unknown>(null);

  function reset(page = initialPageRef.value) {
    currentPage.value = page;
    pages.value = {};
    error.value = null;
  }

  function isPageLoaded(page: number) {
    return Boolean(pages.value[page]);
  }

  async function loadPage(page = currentPage.value, force = false) {
    if (!force && isPageLoaded(page)) {
      currentPage.value = page;
      return pages.value[page];
    }

    pending.value = true;
    error.value = null;

    try {
      const payload = await options.fetchPage(page, pageSizeRef.value);
      pages.value = {
        ...pages.value,
        [page]: payload,
      };
      currentPage.value = page;
      return payload;
    } catch (requestError) {
      error.value = requestError;
      throw requestError;
    } finally {
      pending.value = false;
    }
  }

  async function goToPage(page: number) {
    if (page < 1) return null;
    return loadPage(page);
  }

  async function goToPreviousPage() {
    if (currentPage.value <= 1) return null;
    return goToPage(currentPage.value - 1);
  }

  async function goToNextPage() {
    const current = pages.value[currentPage.value];
    if (!current) {
      const payload = await loadPage(currentPage.value);
      if (!payload?.hasMore) return null;
    } else if (!current.hasMore && !isPageLoaded(currentPage.value + 1)) {
      return null;
    }
    return goToPage(currentPage.value + 1);
  }

  async function refreshCurrentPage() {
    return loadPage(currentPage.value, true);
  }

  watch(
    [cacheKeyRef, pageSizeRef],
    async ([nextKey, nextSize], [prevKey, prevSize]) => {
      if (nextKey === prevKey && nextSize === prevSize) {
        return;
      }
      reset();
      if (immediateRef.value) {
        await loadPage(initialPageRef.value, true);
      }
    },
    { flush: "post" },
  );

  // 首屏是"先取再返回"还是"先返回再取"：默认 block（旧行为），页面可显式要 background。
  let initialReady: Promise<unknown> = Promise.resolve();
  if (immediateRef.value) {
    if (initialLoadRef.value === "background") {
      // 后台模式下没人 await 这个请求：错误已记进 error ref，这里兜住免得变成未处理 rejection。
      initialReady = loadPage(currentPage.value, true).catch(() => null);
    } else {
      initialReady = loadPage(currentPage.value, true);
      await initialReady;
    }
  }

  const currentItems = computed(() => pages.value[currentPage.value]?.items || []);
  const hasPreviousPage = computed(() => currentPage.value > 1);
  const hasNextPage = computed(() => {
    if (isPageLoaded(currentPage.value + 1)) return true;
    return Boolean(pages.value[currentPage.value]?.hasMore);
  });

  return {
    currentPage,
    currentItems,
    pages,
    pending,
    error,
    ready: initialReady,
    hasPreviousPage,
    hasNextPage,
    isPageLoaded,
    reset,
    loadPage,
    goToPage,
    goToPreviousPage,
    goToNextPage,
    refreshCurrentPage,
  };
}
