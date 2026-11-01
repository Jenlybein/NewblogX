import { computed, ref, shallowRef, toRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";
import { readerSwrCache, type ReaderSwrCache, type ReaderSwrLookup } from "~/utils/readerSwrCache";

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
  /**
   * 跨导航共享缓存（30s SWR，ADR-0019 决策 4）。
   *
   * 不传 = 客户端用模块级共享实例、服务端自动禁用；传 `null` = 完全关闭（服务端渲染 /
   * 单测要隔离时用）；传实例 = 注入（单测用假时钟，不真的 sleep 30 秒）。
   */
  swrCache?: ReaderSwrCache | null;
  /** 写缓存前处理载荷：评论这类带 `is_digg` / `relation` 的列表要在这里剔除私有字段。 */
  sanitizeForCache?: (page: PagedResourcePage<TItem>) => PagedResourcePage<TItem>;
}

export async function usePagedResourceCache<TItem>(options: UsePagedResourceCacheOptions<TItem>) {
  const cacheKeyRef = toRef(() => toValue(options.cacheKey));
  const pageSizeRef = toRef(() => toValue(options.pageSize));
  const initialPageRef = toRef(() => toValue(options.initialPage) || 1);
  const immediateRef = toRef(() => toValue(options.immediate) ?? true);
  const initialLoadRef = toRef(() => toValue(options.initialLoad) ?? "block");
  // 服务端不能读写模块级缓存（进程内 Map 会跨请求串数据），默认只在客户端启用。
  // 选项在 setup 时固定，不需要响应式。
  const sharedCache: ReaderSwrCache | null =
    options.swrCache !== undefined
      ? options.swrCache
      : import.meta.client
        ? readerSwrCache
        : null;

  const currentPage = ref(initialPageRef.value);
  const pages = shallowRef<Record<number, PagedResourcePage<TItem>>>({});
  const pending = ref(false);
  // 后台静默刷新的状态：和 pending 分开，界面在刷新期间继续显示旧数据、不闪骨架。
  const revalidating = ref(false);
  let revalidatingCount = 0;
  const error = shallowRef<unknown>(null);

  function reset(page = initialPageRef.value) {
    currentPage.value = page;
    pages.value = {};
    error.value = null;
  }

  function isPageLoaded(page: number) {
    return Boolean(pages.value[page]);
  }

  // 共享缓存的条目键 = "逻辑资源键#页码"：文章 id / 搜索词由调用方放进 cacheKey，
  // 页码在这里补上，所以"不同文章、不同页码、不同搜索词"天然不会串。
  function sharedEntryKey(page: number) {
    return `${cacheKeyRef.value}#${page}`;
  }

  function readSharedPage(page: number): ReaderSwrLookup<PagedResourcePage<TItem>> {
    if (!sharedCache) {
      return { state: "miss" };
    }
    return sharedCache.read<PagedResourcePage<TItem>>(sharedEntryKey(page));
  }

  function writeSharedPage(page: number, payload: PagedResourcePage<TItem>) {
    if (!sharedCache) {
      return;
    }
    sharedCache.write(sharedEntryKey(page), options.sanitizeForCache ? options.sanitizeForCache(payload) : payload);
  }

  function beginRevalidate() {
    revalidatingCount += 1;
    revalidating.value = true;
  }

  function endRevalidate() {
    revalidatingCount = Math.max(0, revalidatingCount - 1);
    revalidating.value = revalidatingCount > 0;
  }

  // 页数据落地的唯一出口：命中（不写缓存）与首屏/重取（写缓存）都走这里，
  // 避免"写内存表"和"写共享缓存"两条路径各自演进后漂移。
  function commitPage(page: number, payload: PagedResourcePage<TItem>, { writeCache = true } = {}) {
    pages.value = {
      ...pages.value,
      [page]: payload,
    };
    if (writeCache) {
      writeSharedPage(page, payload);
    }
  }

  /**
   * 过期命中后的后台重取：**不置 pending、不清旧数据**，失败也不抛出。
   * 切换 key（换了文章）之后才落地的旧请求直接丢弃，不覆盖新页面的显示。
   */
  async function revalidatePage(page: number, keyAtRequest: string) {
    beginRevalidate();
    try {
      const payload = await options.fetchPage(page, pageSizeRef.value);
      if (cacheKeyRef.value !== keyAtRequest) {
        return null;
      }
      commitPage(page, payload);
      return payload;
    } catch (requestError) {
      error.value = requestError;
      return null;
    } finally {
      endRevalidate();
    }
  }

  async function loadPage(page = currentPage.value, force = false) {
    if (!force && isPageLoaded(page)) {
      currentPage.value = page;
      return pages.value[page];
    }

    if (!force) {
      const shared = readSharedPage(page);
      if (shared.state !== "miss") {
        // 命中即渲染：旧值先上屏，30 秒内不重复请求；过期才在后台静默刷新。
        commitPage(page, shared.value, { writeCache: false });
        currentPage.value = page;
        error.value = null;
        if (shared.state === "stale") {
          void revalidatePage(page, cacheKeyRef.value);
        }
        return shared.value;
      }
    }

    pending.value = true;
    error.value = null;

    // 记下发起时的资源键：等待期间路由参数可能已经变了（搜索词 / 文章 id），
    // 迟到的结果既不能覆盖当前页面，也不能写进新键的缓存（否则会把 A 的数据串给 B）。
    const keyAtRequest = cacheKeyRef.value;
    try {
      const payload = await options.fetchPage(page, pageSizeRef.value);
      if (cacheKeyRef.value !== keyAtRequest) {
        return payload;
      }
      currentPage.value = page;
      commitPage(page, payload);
      return payload;
    } catch (requestError) {
      if (cacheKeyRef.value === keyAtRequest) {
        error.value = requestError;
      }
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
        // 换了资源键（文章 id / 搜索词）也要先看共享缓存：返回同一个搜索词时直接命中。
        await loadPage(initialPageRef.value);
      }
    },
    { flush: "post" },
  );

  // 首屏是"先取再返回"还是"先返回再取"：默认 block（旧行为），页面可显式要 background。
  let initialReady: Promise<unknown> = Promise.resolve();
  if (immediateRef.value) {
    if (initialLoadRef.value === "background") {
      // 后台模式下没人 await 这个请求：错误已记进 error ref，这里兜住免得变成未处理 rejection。
      initialReady = loadPage(currentPage.value).catch(() => null);
    } else {
      initialReady = loadPage(currentPage.value);
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
    revalidating,
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
