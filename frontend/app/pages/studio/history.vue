<script setup lang="ts">
import { IconX } from "@tabler/icons-vue";
import { NButton, useMessage } from "naive-ui";
import ArticleCover from "~/components/article/ArticleCover.vue";
import { deleteHistoryArticles, getHistoryArticles } from "~/services/studio";
import { formatDateTimeLabel } from "~/utils/format";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const page = shallowRef(1);

/**
 * 浏览历史只关心"点回去接着读"：短卡片用 16:9 封面 + 标题 + 一行浏览时间，
 * 不渲染摘要与浏览者昵称 —— 因此后端历史接口的 7 个字段够用，不需要扩契约。
 *
 * 接口失败与"确实没有记录"是两种状态：前者由独立的失败态承载（带重试），
 * 不能再落进「还没有浏览历史」的空态里。
 */
const { data, pending, error, refresh } = await useAsyncData(
  () => `studio-history:${page.value}`,
  () => getHistoryArticles({ page: page.value, limit: 20 }),
  { watch: [page] },
);

async function removeOne(id: string) {
  try {
    await deleteHistoryArticles([id]);
    message.success("已移出浏览历史");
    await refresh();
    if (!data.value?.list.length && page.value > 1) {
      page.value -= 1;
    }
  } catch (reason) {
    message.error(reason instanceof Error ? reason.message : "删除历史失败");
  }
}

function previousPage() {
  if (page.value <= 1) return;
  page.value -= 1;
}

function nextPage() {
  if (!data.value?.has_more) return;
  page.value += 1;
}

useSeoMeta({
  title: "个人中心 - 浏览历史",
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <div class="studio-toolbar">
        <div class="section-title">浏览历史</div>
        <NButton quaternary @click="refresh()">刷新记录</NButton>
      </div>

      <div v-if="data?.list.length" class="article-history-grid">
        <article
          v-for="item in data?.list"
          :key="`${item.article_id}-${item.updated_at}`"
          class="article-history-card"
        >
          <NuxtLink
            :to="`/article/${item.article_id}`"
            class="article-history-card__cover"
            :aria-label="`查看《${item.title}》`"
          >
            <ArticleCover :src="item.cover" :title="item.title" label="History" />
          </NuxtLink>

          <button
            type="button"
            class="article-history-card__remove"
            aria-label="移出历史"
            @click="removeOne(item.article_id)"
          >
            <IconX :size="14" aria-hidden="true" />
          </button>

          <NuxtLink :to="`/article/${item.article_id}`" class="article-history-card__title">
            {{ item.title }}
          </NuxtLink>
          <span class="article-history-card__time">
            浏览于 {{ formatDateTimeLabel(item.updated_at) }}
          </span>
        </article>
      </div>

      <div v-else-if="error" class="article-history-error" role="alert" data-state="error">
        <p class="article-history-error__title">浏览历史暂时打不开</p>
        <p class="article-history-error__hint">
          可能是网络或服务波动，你的访问记录没有丢。稍后重试即可。
        </p>
        <NButton quaternary size="small" @click="refresh()">重新加载</NButton>
      </div>

      <StudioEmptyState
        v-else
        title="还没有浏览历史"
        :description="pending ? '正在读取历史记录…' : '先去首页或搜索页看看文章，访问记录会自动出现在这里。'"
      />

      <div v-if="data?.list.length" class="mt-5 flex flex-wrap items-center justify-between gap-3 text-sm muted">
        <span>第 {{ page }} 页</span>
        <div class="flex items-center gap-3">
          <NButton quaternary size="small" :disabled="page <= 1 || pending" @click="previousPage()">上一页</NButton>
          <NButton quaternary size="small" :disabled="!data?.has_more || pending" @click="nextPage()">下一页</NButton>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* 短卡片栅格：2 → 3 → 4 列随视口变宽，卡片宽度完全由列宽决定。
   与合集卡片（20rem 起、带两行简介）相比，这里没有摘要、字号更小、内边距更紧，
   视觉体量明显小一档。 */
.article-history-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

@media (min-width: 768px) {
  .article-history-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .article-history-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
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
  background: var(--color-surface);
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.article-history-card:hover {
  border-color: var(--color-line-strong);
  transform: translateY(-1px);
}

/* 封面锁 16:9（与列表 / 详情页同一个形状），宽度随栅格列宽伸缩。 */
.article-history-card__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--color-bg-subtle);
}

.article-history-card__title {
  display: -webkit-box;
  overflow: hidden;
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

.article-history-card__time {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.04em;
}

/* 移出入口：卡片右上角悬浮图标按钮，悬浮 / 键盘聚焦时显现，窄屏常显。 */
.article-history-card__remove {
  position: absolute;
  top: 12px;
  right: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-pill);
  background: var(--color-surface);
  color: var(--color-ink);
  cursor: pointer;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.article-history-card:hover .article-history-card__remove,
.article-history-card:focus-within .article-history-card__remove {
  opacity: 1;
  pointer-events: auto;
}

.article-history-card__remove:hover {
  background: var(--color-line-faint);
}

.article-history-card__remove:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  opacity: 1;
}

/* 手感设备没有 hover：窄屏直接常显，别让"移出"藏在双击里。 */
@media (max-width: 768px) {
  .article-history-card__remove {
    opacity: 1;
    pointer-events: auto;
  }
}

.article-history-error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  margin-top: 16px;
  padding: 16px;
  border: 1px dashed var(--color-line-strong);
  border-radius: var(--radius-md);
  background: var(--color-line-faint);
}

.article-history-error__title {
  margin: 0;
  color: var(--color-ink);
  font-size: 0.9375rem;
  font-weight: 600;
}

.article-history-error__hint {
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.6;
}
</style>
