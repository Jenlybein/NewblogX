import { computed, shallowRef, toValue } from "vue";
import type { MaybeRefOrGetter } from "vue";
import { searchArticles, type SearchArticlesParams } from "~/services/search";
import type { SearchArticleResponse } from "~/types/api";

interface UseArticleSearchOptions {
  key?: MaybeRefOrGetter<string>;
  fallback?: MaybeRefOrGetter<SearchArticleResponse>;
  immediate?: boolean;
  lazy?: boolean;
  server?: boolean;
  watch?: Array<MaybeRefOrGetter<unknown>>;
}

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
}

function createFallback(params: SearchArticlesParams): SearchArticleResponse {
  const page = params.page ?? 1;
  const limit = params.limit ?? 12;

  return {
    list: [],
    pagination: {
      mode: params.page_mode ?? "count",
      page,
      limit,
      has_more: false,
      total: 0,
      total_pages: 0,
    },
  };
}

export async function useArticleSearch(
  params: MaybeRefOrGetter<SearchArticlesParams>,
  options: UseArticleSearchOptions = {},
) {
  const paramsRef = computed(() => toValue(params));
  const requestFingerprint = computed(() => JSON.stringify(paramsRef.value));
  const fallbackRef = computed(() => toValue(options.fallback) || createFallback(paramsRef.value));
  const externalWatchers = (options.watch || []).map((source) => computed(() => toValue(source)));
  const requestError = shallowRef<unknown>(null);

  const state = await useAsyncData(
    () => toValue(options.key) || `article-search:${requestFingerprint.value}`,
    async () => {
      try {
        requestError.value = null;
        return await searchArticles(paramsRef.value);
      } catch (error) {
        requestError.value = error;
        console.warn(`[useArticleSearch] request failed, using fallback: ${formatRequestError(error)}`, paramsRef.value);
        return fallbackRef.value;
      }
    },
    {
      immediate: options.immediate ?? true,
      lazy: options.lazy ?? false,
      server: options.server ?? true,
      watch: [requestFingerprint, ...externalWatchers],
    },
  );

  const articles = computed(() => state.data.value?.list || fallbackRef.value.list);
  const pagination = computed(() => state.data.value?.pagination || fallbackRef.value.pagination);
  const total = computed(() => pagination.value.total || articles.value.length);

  return {
    ...state,
    articles,
    pagination,
    total,
    requestError,
  };
}
