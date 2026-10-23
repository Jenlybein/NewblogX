<script setup lang="ts">
import { computed, ref, shallowRef, watch } from "vue";
import { NButton, NInput } from "naive-ui";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import { getTagOptions, searchArticles, searchArticlesWithAi } from "~/services/search";

type SearchMode = "normal" | "ai";

const route = useRoute();
const router = useRouter();

useSeoMeta({
  title: "搜索文章",
  description: "通过关键词筛选或智能语义搜索查找公开文章。",
});

const key = ref("");
const sort = ref("1");
const tagId = ref("");
const mode = shallowRef<SearchMode>("normal");
const page = ref(1);
const totalPages = ref(1);
const totalResults = ref(0);
const requestError = shallowRef<unknown>(null);

function parsePage(value: unknown) {
  const parsed = typeof value === "string" && value ? Number(value) : 1;
  if (!Number.isFinite(parsed) || parsed < 1) {
    return 1;
  }
  return Math.floor(parsed);
}

function resolveSearchMode(value: unknown): SearchMode {
  return value === "ai" ? "ai" : "normal";
}

const committedQuery = computed(() => ({
  mode: resolveSearchMode(route.query.mode),
  key: typeof route.query.key === "string" ? route.query.key : "",
  sort: typeof route.query.sort === "string" ? route.query.sort : "1",
  tagId: typeof route.query.tag_ids === "string" ? route.query.tag_ids : "",
  page: resolveSearchMode(route.query.mode) === "ai" ? 1 : parsePage(route.query.page),
}));

watch(
  () => route.query,
  (query) => {
    const nextMode = resolveSearchMode(query.mode);
    mode.value = nextMode;
    key.value = typeof query.key === "string" ? query.key : "";
    sort.value = nextMode === "ai" ? "1" : typeof query.sort === "string" ? query.sort : "1";
    tagId.value = nextMode === "ai" ? "" : typeof query.tag_ids === "string" ? query.tag_ids : "";
    page.value = nextMode === "ai" ? 1 : parsePage(query.page);
  },
  { immediate: true },
);

const { data: tagOptions } = await useAsyncData("search-tags", () => getTagOptions().catch(() => []));

function formatRequestError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

const searchPager = await usePagedResourceCache({
  cacheKey: () =>
    `search:${JSON.stringify({
      mode: committedQuery.value.mode,
      key: committedQuery.value.key.trim(),
      sort: committedQuery.value.mode === "normal" ? committedQuery.value.sort : undefined,
      tag_ids: committedQuery.value.mode === "normal" ? committedQuery.value.tagId : undefined,
    })}`,
  pageSize: () => 9,
  initialPage: () => committedQuery.value.page,
  fetchPage: async (nextPage, limit) => {
    try {
      requestError.value = null;
      if (committedQuery.value.mode === "ai" && !committedQuery.value.key.trim()) {
        totalPages.value = 1;
        totalResults.value = 0;
        return {
          items: [],
          hasMore: false,
        };
      }

      const payload =
        committedQuery.value.mode === "ai"
          ? await searchArticlesWithAi(committedQuery.value.key.trim())
          : await searchArticles({
              key: committedQuery.value.key || undefined,
              type: 1,
              sort: Number(committedQuery.value.sort || "1") as 1 | 2 | 3 | 4 | 5 | 6,
              tag_ids: committedQuery.value.tagId || undefined,
              page: nextPage,
              limit,
              page_mode: "count",
            });

      if (committedQuery.value.mode === "ai") {
        totalPages.value = 1;
        totalResults.value = payload.pagination.total ?? payload.list.length;
      } else {
        totalPages.value = Math.max(payload.pagination.total_pages ?? 1, 1);
        totalResults.value = Math.max(payload.pagination.total ?? 0, 0);
      }

      return {
        items: payload.list,
        hasMore:
          committedQuery.value.mode === "ai"
            ? false
            : nextPage < Math.max(payload.pagination.total_pages ?? 1, 1),
      };
    } catch (error) {
      requestError.value = error;
      totalPages.value = 1;
      totalResults.value = 0;
      console.warn(`[search-articles] request failed, using fallback: ${formatRequestError(error)}`);
      return {
        items: [],
        hasMore: false,
      };
    }
  },
});

const sortOptions = [
  { label: "综合相关度", value: "1" },
  { label: "最新发布", value: "2" },
  { label: "回复最多", value: "3" },
  { label: "点赞最多", value: "4" },
  { label: "收藏最多", value: "5" },
  { label: "阅读最多", value: "6" },
];

watch(
  page,
  async (nextPage, previousPage) => {
    if (nextPage === previousPage) {
      return;
    }

    try {
      await searchPager.goToPage(nextPage);
    } catch {
      // 错误状态已在 fetchPage 中收集，这里不重复抛出。
    }
  },
  { flush: "post" },
);

const articles = computed(() => searchPager.currentItems.value);
const pending = computed(() => searchPager.pending.value);
const cachedPageCount = computed(() => Object.keys(searchPager.pages.value).length);
const currentSearchPage = computed(() => searchPager.currentPage.value);
const isAiMode = computed(() => mode.value === "ai");
const searchPlaceholder = computed(() =>
  isAiMode.value
    ? "直接通过对话形式，输入你想要找怎么样的文章"
    : "输入关键字搜索文章标题、摘要或内容…",
);
const searchModeToggleLabel = computed(() => (isAiMode.value ? "切换到普通搜索" : "切换到智能搜索"));
const resultTitle = computed(() => (isAiMode.value ? "智能搜索结果" : "搜索结果"));
const emptyText = computed(() => {
  if (pending.value) {
    return isAiMode.value ? "正在理解你的问题并搜索文章..." : "正在搜索中...";
  }
  if (requestError.value) {
    return isAiMode.value ? "智能搜索失败，请稍后重试或切换到普通搜索。" : "文章加载失败，请检查前端 API 地址或测试环境状态。";
  }
  return isAiMode.value ? "没有找到合适的文章，试着换一种更具体的描述。" : "没有匹配结果，换个关键词或筛选条件试试。";
});
const showPager = computed(() => !isAiMode.value && (totalResults.value > 0 || currentSearchPage.value > 1));

function handleReset() {
  key.value = "";
  sort.value = "1";
  tagId.value = "";
  page.value = 1;
  router.push({
    path: "/search",
    query: isAiMode.value ? { mode: "ai" } : {},
  });
}

function handleSearch() {
  page.value = 1;
  router.push({
    path: "/search",
    query: buildQuery(1, mode.value),
  });
}

function buildQuery(targetPage = 1, targetMode: SearchMode = mode.value) {
  if (targetMode === "ai") {
    return {
      mode: "ai",
      ...(key.value.trim() ? { key: key.value.trim() } : {}),
    };
  }

  return {
    ...(key.value ? { key: key.value } : {}),
    ...(sort.value && sort.value !== "1" ? { sort: sort.value } : {}),
    ...(tagId.value ? { tag_ids: tagId.value } : {}),
    ...(targetPage > 1 ? { page: String(targetPage) } : {}),
  };
}

function handleToggleSearchMode() {
  const nextMode: SearchMode = isAiMode.value ? "normal" : "ai";
  page.value = 1;
  if (nextMode === "ai") {
    sort.value = "1";
    tagId.value = "";
  }

  router.push({
    path: "/search",
    query: buildQuery(1, nextMode),
  });
}

function handlePageChange(nextPage: number) {
  const targetPage = Math.min(Math.max(nextPage, 1), Math.max(totalPages.value, 1));
  router.push({
    path: "/search",
    query: buildQuery(targetPage),
  });
}

watch(
  [currentSearchPage, totalPages, pending],
  async ([current, total, isPending]) => {
    if (isPending) {
      return;
    }

    const safeTotalPages = Math.max(total, 1);
    if (current <= safeTotalPages) {
      return;
    }

    await router.replace({
      path: "/search",
      query: buildQuery(safeTotalPages),
    });
  },
  { flush: "post" },
);
</script>

<template>
  <div class="page-stack">
    <section class="filter-card">
      <div class="mb-5">
        <div class="eyebrow">Search</div>
        <h1 class="section-title mt-2">搜索文章</h1>
      </div>

      <div class="space-y-4">
        <NInput
          v-model:value="key"
          name="article-search"
          autocomplete="off"
          aria-label="搜索文章关键字"
          round
          clearable
          :placeholder="searchPlaceholder"
          @keydown.enter.prevent="handleSearch"
        />

        <div v-if="!isAiMode" class="grid gap-4 md:grid-cols-2">
          <label class="flex items-center">
            <select v-model="tagId" name="tag_ids" aria-label="标签筛选" class="h-12 w-full rounded-full border border-white/70 bg-white/78 px-4 text-sm text-slate-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)] backdrop-blur">
              <option value="">全部标签</option>
              <option v-for="option in tagOptions || []" :key="option.value" :value="String(option.value)">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="flex items-center">
            <select v-model="sort" name="sort" aria-label="排序方式" class="h-12 w-full rounded-full border border-white/70 bg-white/78 px-4 text-sm text-slate-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)] backdrop-blur">
              <option v-for="option in sortOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3">
          <NButton secondary round @click="handleToggleSearchMode">{{ searchModeToggleLabel }}</NButton>
          <div class="flex items-center gap-3">
            <NButton quaternary @click="handleReset">重置</NButton>
            <NButton type="primary" @click="handleSearch">搜索</NButton>
          </div>
        </div>
      </div>
    </section>

    <section class="surface-card p-5 md:p-6">
      <div class="mb-5 flex items-center justify-between">
        <div class="section-title">{{ resultTitle }}</div>
        <div class="text-sm muted">
          <template v-if="isAiMode">
            共 {{ totalResults }} 条结果。
          </template>
          <template v-else>
            共 {{ totalResults }} 条结果，每页 9 篇，当前第 {{ currentSearchPage }} / {{ Math.max(totalPages, 1) }} 页，本次已缓存
            {{ cachedPageCount }} 页。
          </template>
        </div>
      </div>

      <div v-if="articles.length" class="space-y-4">
        <ArticleFeedItem
          v-for="article in articles"
          :key="article.id"
          :article="article"
          compact
        />
      </div>

      <div v-else class="surface-section flex min-h-[240px] items-center justify-center p-6 text-sm muted">
        {{ emptyText }}
      </div>

      <div v-if="showPager" class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-white/60 pt-5 text-sm muted">
        <span>未刷新前，已访问页会直接复用缓存。</span>
        <div class="flex items-center gap-3">
          <NButton quaternary :disabled="!searchPager.hasPreviousPage" @click="handlePageChange(currentSearchPage - 1)">
            上一页
          </NButton>
          <NButton
            type="primary"
            ghost
            :disabled="!searchPager.hasNextPage || currentSearchPage >= totalPages"
            :loading="pending"
            @click="handlePageChange(currentSearchPage + 1)"
          >
            下一页
          </NButton>
        </div>
      </div>
    </section>
  </div>
</template>
