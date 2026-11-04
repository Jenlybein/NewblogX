<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NInput, useDialog, useMessage } from "naive-ui";
import { IconSearch, IconTrash } from "@tabler/icons-vue";
import ArticleCover from "~/components/article/ArticleCover.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { clearHistoryArticles, deleteHistoryArticles, getHistoryArticles } from "~/services/history";
import { formatDateTimeLabel } from "~/utils/format";
import { errorMessage } from "~/utils/errorMessage";
import { cursorOfPage, groupHistoryItems, type HistoryCursor } from "~/utils/historyGroups";
import type { HistoryArticleItem } from "~/types/api";

/**
 * 浏览历史独立页（路由 `/history`）。
 *
 * 三件事决定了这一页的形状：
 *   1. **分组按日历天**（今天 / 昨天 / 近一周 / 更早）→ 分页必须是游标：
 *      offset 分页在删除或重复浏览把某条顶到最前之后会让下一页整体位移，
 *      同一分组会被切成两段（判据抽在 utils/historyGroups.ts）；
 *   2. 卡片只放封面 + 两行标题 + 浏览时间：后端有昵称/头像字段，但列表页不展示
 *      （不要把历史页做成第二份文章卡）；
 *   3. 清空不可逆 → 二次确认；批量管理是显式多选态（勾选框 + 已选 N 条），
 *      不在每张卡片上常驻第二个删除动作。
 *
 * 本页 `ssr: false`：历史是私人数据，不索引、也不进匿名 SSR 缓存
 * （渲染边界表 shared/readerRenderingBoundary.ts）。
 */

definePageMeta({
  middleware: "auth",
});

useSeoMeta({
  title: "浏览历史",
  description: "按浏览时间回看你读过的文章。",
});

const PAGE_SIZE = 20;

const message = useMessage();
const dialog = useDialog();

const items = ref<HistoryArticleItem[]>([]);
const hasMore = ref(false);
const cursor = ref<HistoryCursor | null>(null);
const loading = ref(true);
const loadingMore = ref(false);
const busy = ref(false);
const error = ref<unknown>(null);

const keyword = ref("");
const committedKeyword = ref("");

const batchMode = ref(false);
const selected = ref<string[]>([]);

const groups = computed(() => groupHistoryItems(items.value));
const selectedCount = computed(() => selected.value.length);

function currentKeyword() {
  return committedKeyword.value.trim() || undefined;
}

function applyList(list: HistoryArticleItem[], more: boolean) {
  items.value = list;
  hasMore.value = more;
  cursor.value = cursorOfPage(list);
  // 列表换了一轮（搜索/重载）之后，选中列表里已经不存在的条目没有意义。
  selected.value = selected.value.filter((id) => list.some((item) => item.article_id === id));
}

async function loadFirstPage() {
  loading.value = true;
  error.value = null;
  try {
    const page = await getHistoryArticles({ keyword: currentKeyword(), limit: PAGE_SIZE });
    applyList(page.list, page.has_more);
  } catch (reason) {
    // 失败态与空态必须分开：失败时落进「还没有浏览历史」会让用户以为记录没了。
    error.value = reason;
    items.value = [];
    hasMore.value = false;
    cursor.value = null;
  } finally {
    loading.value = false;
  }
}

async function submitSearch() {
  committedKeyword.value = keyword.value.trim();
  exitBatchMode();
  await loadFirstPage();
}

async function loadMore() {
  const next = cursor.value;
  if (!next || !hasMore.value || loading.value || loadingMore.value) return;

  loadingMore.value = true;
  try {
    const page = await getHistoryArticles({
      keyword: currentKeyword(),
      beforeUpdatedAt: next.beforeUpdatedAt,
      beforeId: next.beforeId,
      limit: PAGE_SIZE,
    });
    // 追加而不是替换：分组由 append 后的完整列表重新算，所以同一天的两页会并进
    // 同一个分组（这就是"分组跨页连续"的实现方式）。
    items.value = [...items.value, ...page.list];
    hasMore.value = page.has_more;
    cursor.value = cursorOfPage(page.list) ?? next;
  } catch (reason) {
    message.error(errorMessage(reason, "加载更多失败，请稍后再试"));
  } finally {
    loadingMore.value = false;
  }
}

function toggleSelect(articleId: string) {
  selected.value = selected.value.includes(articleId)
    ? selected.value.filter((id) => id !== articleId)
    : [...selected.value, articleId];
}

function enterBatchMode() {
  batchMode.value = true;
  selected.value = [];
}

function exitBatchMode() {
  batchMode.value = false;
  selected.value = [];
}

async function removeItems(idList: string[], successMessage: string) {
  if (busy.value || idList.length === 0) return;

  busy.value = true;
  try {
    await deleteHistoryArticles(idList);
    items.value = items.value.filter((item) => !idList.includes(item.article_id));
    selected.value = selected.value.filter((id) => !idList.includes(id));
    // 删空之后不再有"下一页"：留着 has_more=true 会让空列表下面继续挂一个按钮。
    if (items.value.length === 0) {
      hasMore.value = false;
      cursor.value = null;
    }
    message.success(successMessage);
  } catch (reason) {
    message.error(errorMessage(reason, "删除浏览历史失败"));
  } finally {
    busy.value = false;
  }
}

async function removeOne(articleId: string) {
  await removeItems([articleId], "已移出浏览历史");
}

async function removeSelected() {
  const idList = [...selected.value];
  await removeItems(idList, `已移出 ${idList.length} 条浏览历史`);
  if (!busy.value) exitBatchMode();
}

function confirmClearHistory() {
  dialog.warning({
    title: "清空浏览历史",
    content: "清空后不可恢复：全部浏览记录会立刻从列表消失。确定清空全部浏览历史吗？",
    positiveText: "清空",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await clearHistoryArticles();
        items.value = [];
        hasMore.value = false;
        cursor.value = null;
        exitBatchMode();
        message.success("已清空浏览历史");
      } catch (reason) {
        message.error(errorMessage(reason, "清空浏览历史失败"));
      }
    },
  });
}

onMounted(() => {
  void loadFirstPage();
});
</script>

<template>
  <div class="page-stack history-page">
    <section class="history-toolbar">
      <div>
        <div class="eyebrow">History</div>
        <h1 class="section-title mt-2">浏览历史</h1>
        <p class="history-toolbar__hint">按浏览时间分组；同一篇只保留最近一次。</p>
      </div>

      <div class="history-toolbar__ops">
        <div class="history-search">
          <NInput
            v-model:value="keyword"
            type="text"
            clearable
            placeholder="搜索标题"
            aria-label="搜索浏览历史标题"
            @keyup.enter="submitSearch"
          />
          <NButton quaternary aria-label="搜索" @click="submitSearch">
            <template #icon>
              <IconSearch :size="16" aria-hidden="true" />
            </template>
            搜索
          </NButton>
        </div>

        <div v-if="batchMode" class="history-batch">
          <span class="history-selected">已选 {{ selectedCount }} 条</span>
          <NButton size="small" :disabled="!selectedCount || busy" @click="removeSelected">删除</NButton>
          <NButton size="small" quaternary @click="exitBatchMode">取消</NButton>
        </div>
        <NButton v-else quaternary :disabled="!items.length" @click="enterBatchMode">批量管理</NButton>

        <NButton quaternary :disabled="!items.length || busy" @click="confirmClearHistory">清空历史</NButton>
      </div>
    </section>

    <div v-if="loading" class="history-loading" data-state="loading">
      <LoadingSkeleton variant="card" :rows="4" label="正在读取浏览历史" />
    </div>

    <section v-else-if="error" class="history-error" role="alert" data-state="error">
      <p class="history-error__title">浏览历史暂时打不开</p>
      <p class="history-error__hint">
        可能是网络或服务波动，你的访问记录不会丢。稍后重试即可。
      </p>
      <NButton quaternary size="small" @click="loadFirstPage">重新加载</NButton>
    </section>

    <ProfileEmptyState
      v-else-if="!items.length"
      :title="committedKeyword ? '没有匹配的历史记录' : '还没有浏览历史'"
      :description="
        committedKeyword
          ? '换个关键词再试，或清空搜索看全部记录。'
          : '去首页或搜索页读几篇文章，访问记录会自动出现在这里。'
      "
    />

    <div v-else class="history-timeline">
      <section v-for="group in groups" :key="group.key" class="history-group">
        <div class="history-group__axis">
          <span class="history-group__dot" aria-hidden="true" />
          <span class="history-group__label">{{ group.label }}</span>
        </div>

        <div class="article-history-grid">
          <article v-for="item in group.items" :key="item.article_id" class="article-history-card">
            <label v-if="batchMode" class="article-history-card__select">
              <input
                type="checkbox"
                :checked="selected.includes(item.article_id)"
                :aria-label="`选择「${item.title}」`"
                @change="toggleSelect(item.article_id)"
              />
            </label>

            <NuxtLink
              v-if="!batchMode"
              :to="`/article/${item.article_id}`"
              class="article-history-card__cover"
              :aria-label="`查看《${item.title}》`"
            >
              <ArticleCover :src="item.cover" :title="item.title" />
            </NuxtLink>
            <div v-else class="article-history-card__cover" aria-hidden="true">
              <ArticleCover :src="item.cover" :title="item.title" />
            </div>

            <div class="article-history-card__title-row">
              <NuxtLink
                v-if="!batchMode"
                :to="`/article/${item.article_id}`"
                class="article-history-card__title"
              >
                {{ item.title }}
              </NuxtLink>
              <span v-else class="article-history-card__title">{{ item.title }}</span>

              <button
                v-if="!batchMode"
                type="button"
                class="article-history-card__remove"
                aria-label="移出浏览历史"
                :disabled="busy"
                @click="removeOne(item.article_id)"
              >
                <IconTrash :size="14" aria-hidden="true" />
              </button>
            </div>

            <span class="article-history-card__time">
              浏览于 {{ formatDateTimeLabel(item.updated_at) }}
            </span>
          </article>
        </div>
      </section>
    </div>

    <div v-if="!loading && !error && items.length" class="history-footer">
      <NButton v-if="hasMore" class="history-more" quaternary :loading="loadingMore" @click="loadMore">
        加载更多
      </NButton>
      <span v-else class="history-end">没有更多了</span>
    </div>
  </div>
</template>

<style scoped>
/*
 * 版式：左侧时间轴（竖线 + 圆点 + 分组标签）、右侧卡片栅格。
 * 色值全部走语义 token（surface-raised / line-faint / line / ink），不写裸色值。
 */
.history-page {
  padding-block: 8px 24px;
}

.history-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding: 20px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
}

.history-toolbar__hint {
  margin: 6px 0 0;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.history-toolbar__ops {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.history-search {
  display: flex;
  align-items: center;
  gap: 8px;
}

.history-search :deep(.n-input) {
  min-width: 200px;
}

.history-batch {
  display: flex;
  align-items: center;
  gap: 10px;
}

.history-selected {
  color: var(--color-ink);
  font-size: 0.8125rem;
  font-weight: 600;
}

.history-loading {
  padding-block: 12px;
}

.history-error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  padding: 16px;
  border: 1px dashed var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-line-faint);
}

.history-error__title {
  margin: 0;
  color: var(--color-ink);
  font-size: 0.9375rem;
  font-weight: 600;
}

.history-error__hint {
  margin: 0 0 4px;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.6;
}

/* ── 时间轴 ── */

.history-timeline {
  position: relative;
  padding-inline-start: 4px;
}

/* 整条竖线只画一次（而不是每个分组各画一段拼起来）：分组之间被切开的缝会
   让"时间轴"看起来断断续续。圆点由每个分组绝对定位到这条线上。 */
.history-timeline::before {
  content: "";
  position: absolute;
  inset-block: 8px;
  inset-inline-start: 10px;
  width: 1px;
  background: var(--color-line-faint);
}

.history-group {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr);
  column-gap: 24px;
  padding-block: 2px 28px;
}

.history-group__axis {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-inline-start: 20px;
}

.history-group__dot {
  position: absolute;
  inset-inline-start: 5px;
  inset-block-start: 2px;
  width: 10px;
  height: 10px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  background: var(--color-surface-raised);
}

.history-group__label {
  color: var(--color-ink);
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.08em;
}

/* ── 卡片栅格（4–5 列自适应） ── */

.article-history-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

@media (min-width: 768px) {
  .article-history-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .article-history-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1440px) {
  .article-history-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}

.article-history-card {
  position: relative;
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.article-history-card:hover {
  border-color: var(--color-line-strong);
  transform: translateY(-1px);
}

/* 封面锁 16:9（与列表 / 详情同一个形状），宽度随列宽伸缩。 */
.article-history-card__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--color-line-faint);
}

.article-history-card__select {
  position: absolute;
  z-index: 2;
  inset-block-start: 6px;
  inset-inline-start: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-sm);
  background: var(--color-surface-raised);
}

.article-history-card__select input {
  width: 14px;
  height: 14px;
  margin: 0;
  accent-color: var(--color-ink);
  cursor: pointer;
}

.article-history-card__title-row {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 4px;
}

.article-history-card__title {
  display: -webkit-box;
  overflow: hidden;
  flex: 1 1 auto;
  color: var(--color-ink);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1.4;
  text-decoration: none;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.article-history-card__title:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.article-history-card__remove {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  background: var(--color-surface-raised);
  color: var(--color-ink-muted);
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.article-history-card__remove:hover,
.article-history-card__remove:focus-visible {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
}

.article-history-card__remove:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.article-history-card__time {
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.04em;
}

/* ── 底部：加载更多 / 没有更多 ── */

.history-footer {
  display: flex;
  justify-content: center;
  padding-block: 8px;
}

.history-end {
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
}

/* 窄屏：时间轴收窄，删除按钮给更大的触摸留白（形状不变）。 */
@media (max-width: 768px) {
  .history-toolbar {
    align-items: stretch;
  }

  .history-group {
    grid-template-columns: 64px minmax(0, 1fr);
    column-gap: 12px;
  }

  .article-history-card__remove {
    padding: 6px;
  }
}
</style>
