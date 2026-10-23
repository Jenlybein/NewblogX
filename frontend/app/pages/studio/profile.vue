<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { NButton, NInput, NPagination, NTag, useMessage } from "naive-ui";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import { deleteOwnArticle } from "~/services/article";
import { getTagOptions } from "~/services/search";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

type ArticleStatusFilter = 0 | 1 | 2 | 3 | 4;

const route = useRoute();
const router = useRouter();
const message = useMessage();
const { openWriteEntry } = useWriteEntry();

const key = shallowRef("");
const sort = shallowRef("2");
const tagId = shallowRef("");
const page = shallowRef(1);
const statusFilter = shallowRef<ArticleStatusFilter>(0);

const sortOptions = [
  { label: "最新发布", value: "2" },
  { label: "综合相关度", value: "1" },
  { label: "回复最多", value: "3" },
  { label: "点赞最多", value: "4" },
  { label: "收藏最多", value: "5" },
  { label: "阅读最多", value: "6" },
] as const;

const statusOptions = [
  { label: "全部状态", value: 0 },
  { label: "草稿", value: 1 },
  { label: "待审核", value: 2 },
  { label: "已发布", value: 3 },
  { label: "已拒绝", value: 4 },
] as const;

watch(
  () => route.query,
  (query) => {
    key.value = typeof query.key === "string" ? query.key : "";
    sort.value = typeof query.sort === "string" ? query.sort : "2";
    tagId.value = typeof query.tag_ids === "string" ? query.tag_ids : "";
    page.value = typeof query.page === "string" && query.page ? Math.max(Number(query.page) || 1, 1) : 1;
    statusFilter.value =
      typeof query.status === "string" && ["1", "2", "3", "4"].includes(query.status)
        ? (Number(query.status) as ArticleStatusFilter)
        : 0;
  },
  { immediate: true },
);

const { data: tagOptions } = await useAsyncData("studio-profile-tag-options", () => getTagOptions().catch(() => []));

const articleQuery = computed(() => ({
  type: 4 as const,
  key: key.value.trim() || undefined,
  tag_ids: tagId.value || undefined,
  sort: Number(sort.value || "2") as 1 | 2 | 3 | 4 | 5 | 6,
  status: statusFilter.value || undefined,
  page: page.value,
  limit: 9,
  page_mode: "count" as const,
  auth: true,
}));

const {
  articles,
  pending,
  pagination,
  total,
  refresh,
  requestError,
} = await useArticleSearch(articleQuery, {
  key: computed(
    () =>
      `studio-profile-search:${JSON.stringify({
        key: key.value.trim(),
        sort: sort.value,
        tag_ids: tagId.value,
        status: statusFilter.value,
        page: page.value,
      })}`,
  ),
  server: false,
  lazy: true,
});

const totalPages = computed(() => Math.max(pagination.value.total_pages || 0, 1));

watch(
  () => pagination.value.total_pages,
  (nextTotalPages) => {
    if (nextTotalPages && page.value > nextTotalPages) {
      handlePageChange(nextTotalPages);
    }
  },
);

function buildQuery(targetPage = 1) {
  return {
    ...(key.value.trim() ? { key: key.value.trim() } : {}),
    ...(sort.value !== "2" ? { sort: sort.value } : {}),
    ...(tagId.value ? { tag_ids: tagId.value } : {}),
    ...(statusFilter.value ? { status: String(statusFilter.value) } : {}),
    ...(targetPage > 1 ? { page: String(targetPage) } : {}),
  };
}

function applyFilters(targetPage = 1) {
  void router.push({
    path: "/studio/profile",
    query: buildQuery(targetPage),
  });
}

function handleReset() {
  key.value = "";
  sort.value = "2";
  tagId.value = "";
  statusFilter.value = 0;
  page.value = 1;
  applyFilters(1);
}

function handleSearch() {
  page.value = 1;
  applyFilters(1);
}

function handlePageChange(nextPage: number) {
  page.value = nextPage;
  applyFilters(nextPage);
}

function statusLabel(status: number) {
  return statusOptions.find((item) => item.value === status)?.label ?? `状态 ${status}`;
}

function statusType(status: number) {
  if (status === 3) return "success";
  if (status === 2) return "warning";
  if (status === 4) return "error";
  return "default";
}

async function handleDelete(id: string) {
  try {
    await deleteOwnArticle(id);
    message.success("文章已删除");
    await refresh();

    if (!articles.value.length && page.value > 1) {
      handlePageChange(page.value - 1);
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : "删除文章失败");
  }
}

useSeoMeta({
  title: "个人中心 - 我的文章",
});
</script>

<template>
  <div class="page-stack">
    <section class="surface-card p-5 md:p-6">
      <div class="mb-5 flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="section-title">筛选我的文章</div>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <NuxtLink
            to="/studio/write"
            target="_blank"
            rel="noopener noreferrer"
            class="glass-badge"
            @click.prevent="openWriteEntry()">创作文章</NuxtLink>
          <NButton quaternary @click="refresh()">刷新列表</NButton>
          <div class="text-sm muted">共 {{ total }} 篇，当前第 {{ pagination.page }} / {{ totalPages }} 页</div>
        </div>
      </div>

      <div class="space-y-4">
        <NInput
          v-model:value="key"
          name="my-article-keyword"
          autocomplete="off"
          round
          clearable
          placeholder="按标题、摘要或正文关键字筛选我的文章…"
          @keydown.enter.prevent="handleSearch"
        />

        <div class="grid gap-4 xl:grid-cols-4 md:grid-cols-2">
          <label class="flex items-center">
            <select
              v-model="statusFilter"
              name="status"
              aria-label="文章状态"
              class="h-12 w-full rounded-full border border-white/70 bg-white/78 px-4 text-sm text-slate-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)] backdrop-blur"
            >
              <option v-for="option in statusOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="flex items-center">
            <select
              v-model="tagId"
              name="tag_ids"
              aria-label="标签筛选"
              class="h-12 w-full rounded-full border border-white/70 bg-white/78 px-4 text-sm text-slate-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)] backdrop-blur"
            >
              <option value="">全部标签</option>
              <option v-for="option in tagOptions || []" :key="option.value" :value="String(option.value)">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="flex items-center">
            <select
              v-model="sort"
              name="sort"
              aria-label="排序方式"
              class="h-12 w-full rounded-full border border-white/70 bg-white/78 px-4 text-sm text-slate-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)] backdrop-blur"
            >
              <option v-for="option in sortOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <div class="flex items-center justify-end gap-3">
            <NButton quaternary @click="handleReset">重置</NButton>
            <NButton type="primary" @click="handleSearch">查询</NButton>
          </div>
        </div>
      </div>
    </section>

    <section class="surface-card p-5 md:p-6">
      <div v-if="articles.length" class="space-y-5">
        <article
          v-for="item in articles"
          :key="item.id"
          class="rounded-[30px] border border-white/65 bg-white/72 p-4 shadow-[0_18px_50px_rgba(15,23,42,0.06)] backdrop-blur"
        >
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <NTag :type="statusType(item.publish_status)" size="small">
                {{ statusLabel(item.publish_status) }}
              </NTag>
              <NTag v-if="item.visibility_status !== 'visible'" size="small" type="warning">
                {{
                  item.visibility_status === "user_hidden"
                    ? "仅自己可见"
                    : item.visibility_status === "admin_hidden"
                      ? "管理员隐藏"
                      : item.visibility_status
                }}
              </NTag>
              <NTag v-if="!item.comments_toggle" size="small" type="warning">评论已关闭</NTag>
            </div>

            <div class="flex flex-wrap gap-2">
              <NuxtLink :to="`/article/${item.id}`" class="glass-badge">查看详情</NuxtLink>
              <NuxtLink
                :to="{ path: '/studio/write', query: { article_id: item.id } }"
                target="_blank"
                rel="noopener noreferrer"
                class="glass-badge"
                @click.prevent="openWriteEntry({ articleId: item.id })">编辑</NuxtLink>
              <NButton quaternary size="small" @click="handleDelete(item.id)">删除</NButton>
            </div>
          </div>

          <ArticleFeedItem :article="item" compact :show-author="false" />
        </article>
      </div>

      <StudioEmptyState
        v-else
        title="当前筛选下没有文章"
        :description="
          pending
            ? '正在同步文章列表…'
            : requestError
              ? '文章列表加载失败，请检查登录态、搜索投影或本地 API 连接。'
              : '可以调整关键词、标签、状态或排序方式后再试。'
        "
      />

      <div
        v-if="total > 0"
        class="mt-6 flex flex-wrap items-center justify-between gap-4 border-t border-white/60 pt-5"
      >
        <NPagination
          :page="pagination.page"
          :page-count="totalPages"
          :page-slot="7"
          @update:page="handlePageChange"
        />
      </div>
    </section>
  </div>
</template>
