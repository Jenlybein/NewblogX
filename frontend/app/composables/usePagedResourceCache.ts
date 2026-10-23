import { computed, ref, shallowRef, toRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

export interface PagedResourcePage<TItem> {
  items: TItem[];
  hasMore: boolean;
}

interface UsePagedResourceCacheOptions<TItem> {
  cacheKey: MaybeRefOrGetter<string>;
  pageSize: MaybeRefOrGetter<number>;
  initialPage?: MaybeRefOrGetter<number>;
  immediate?: boolean;
  fetchPage: (page: number, limit: number) => Promise<PagedResourcePage<TItem>>;
}

export async function usePagedResourceCache<TItem>(options: UsePagedResourceCacheOptions<TItem>) {
  const cacheKeyRef = toRef(() => toValue(options.cacheKey));
  const pageSizeRef = toRef(() => toValue(options.pageSize));
  const initialPageRef = toRef(() => toValue(options.initialPage) || 1);
  const immediateRef = toRef(() => toValue(options.immediate) ?? true);

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

  if (immediateRef.value) {
    await loadPage(currentPage.value, true);
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
