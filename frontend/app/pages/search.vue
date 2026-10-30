<script setup lang="ts">
import { computed, ref, shallowRef, watch } from "vue";
import { NButton, NInput } from "naive-ui";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { getTagOptions, searchArticles, searchArticlesWithAi } from "~/services/search";
import {
  SEARCH_ACTION_LABELS,
  resolveSearchEmptyState,
  type SearchAction,
} from "~/utils/searchState";

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
const currentSearchPage = computed(() => searchPager.currentPage.value);
const isAiMode = computed(() => mode.value === "ai");
const searchPlaceholder = computed(() =>
  isAiMode.value
    ? "直接通过对话形式，输入你想要找怎么样的文章"
    : "输入关键字搜索文章标题、摘要或内容…",
);
const searchModeToggleLabel = computed(() => (isAiMode.value ? "切换到普通搜索" : "切换到智能搜索"));
const resultTitle = computed(() => (isAiMode.value ? "智能搜索结果" : "搜索结果"));
// 空态 / 错误态统一走纯函数（utils/searchState）：文案与"下一步该做什么"只写一处，
// 页面只负责把 actions 渲染成按钮。
const emptyState = computed(() =>
  resolveSearchEmptyState({
    mode: mode.value,
    key: committedQuery.value.key,
    tagId: committedQuery.value.tagId,
    pending: pending.value,
    failed: Boolean(requestError.value),
  }),
);
const emptyActions = computed(() => emptyState.value.actions);
const suggestionTags = computed(() => (tagOptions.value || []).slice(0, 6));

async function handleEmptyAction(action: SearchAction) {
  if (action === "retry") {
    try {
      await searchPager.refreshCurrentPage();
    } catch {
      // 失败状态已在 fetchPage 中收集，这里不重复抛出。
    }
    return;
  }
  if (action === "clear-filters") {
    handleReset();
    return;
  }
  if (action === "switch-mode") {
    handleToggleSearchMode();
    return;
  }
  await navigateTo("/");
}

function handleSuggestionTag(value: string) {
  tagId.value = value;
  key.value = "";
  handleSearch();
}

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
        <p class="mt-2 text-sm leading-6 text-ink-muted">
          关键词搜索标题、摘要与正文；也可以按标签筛选，或切到智能搜索用一句话描述。
        </p>
      </div>

      <div class="space-y-4">
        <NInput
          v-model:value="key"
          class="search-field"
          name="article-search"
          autocomplete="off"
          aria-label="搜索文章关键字"
          clearable
          :placeholder="searchPlaceholder"
          @keydown.enter.prevent="handleSearch"
        />

        <div v-if="!isAiMode" class="grid gap-4 md:grid-cols-2">
          <label class="flex items-center">
            <span class="sr-only">标签筛选</span>
            <select v-model="tagId" name="tag_ids" class="search-select" aria-label="标签筛选">
              <option value="">全部标签</option>
              <option v-for="option in tagOptions || []" :key="option.value" :value="String(option.value)">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="flex items-center">
            <span class="sr-only">排序方式</span>
            <select v-model="sort" name="sort" class="search-select" aria-label="排序方式">
              <option v-for="option in sortOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3">
          <NButton secondary attr-type="button" @click="handleToggleSearchMode">{{ searchModeToggleLabel }}</NButton>
          <div class="flex items-center gap-3 blueprint-actions">
            <NButton quaternary attr-type="button" @click="handleReset">重置</NButton>
            <NButton type="primary" attr-type="button" @click="handleSearch">搜索</NButton>
          </div>
        </div>
      </div>
    </section>

    <section class="blueprint-panel">
      <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="eyebrow">Results</div>
          <h2 class="section-title mt-2">{{ resultTitle }}</h2>
        </div>
        <div class="text-sm text-ink-muted">
          <template v-if="isAiMode">共 {{ totalResults }} 条结果。</template>
          <template v-else>
            共 {{ totalResults }} 条结果，每页 9 篇，当前第 {{ currentSearchPage }} / {{ Math.max(totalPages, 1) }} 页
          </template>
        </div>
      </div>

      <div v-if="articles.length" class="space-y-4">
        <ArticleFeedItem
          v-for="article in articles"
          :key="article.id"
          :article="article"
          variant="search"
        />
      </div>

      <ProfileEmptyState
        v-else
        size="lg"
        :pending="pending"
        :title="emptyState.title"
        :description="emptyState.description"
        :hints="emptyState.hints"
      >
        <button
          v-for="action in emptyActions"
          :key="action"
          type="button"
          class="blueprint-tab"
          @click="handleEmptyAction(action)"
        >
          {{ SEARCH_ACTION_LABELS[action] }}
        </button>
      </ProfileEmptyState>

      <div v-if="suggestionTags.length && emptyState.kind === 'idle'" class="mt-5 border-t border-line-faint pt-4">
        <p class="text-sm text-ink-muted">从标签开始浏览：</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-for="tag in suggestionTags"
            :key="tag.value"
            type="button"
            class="blueprint-tab"
            @click="handleSuggestionTag(String(tag.value))"
          >
            {{ tag.label }}
          </button>
        </div>
      </div>

      <div v-if="showPager" class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-line-faint pt-5 text-sm text-ink-muted">
        <span>未刷新前，已访问页会直接复用缓存。</span>
        <div class="flex items-center gap-3 blueprint-actions">
          <NButton quaternary :disabled="!searchPager.hasPreviousPage" attr-type="button" @click="handlePageChange(currentSearchPage - 1)">
            上一页
          </NButton>
          <NButton
            type="primary"
            ghost
            :disabled="!searchPager.hasNextPage || currentSearchPage >= totalPages"
            :loading="pending"
            attr-type="button"
            @click="handlePageChange(currentSearchPage + 1)"
          >
            下一页
          </NButton>
        </div>
      </div>
    </section>
  </div>
</template>
