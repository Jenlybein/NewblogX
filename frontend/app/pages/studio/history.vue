<script setup lang="ts">
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

const { data, pending, refresh } = await useAsyncData(
  () => `studio-history:${page.value}`,
  () => getHistoryArticles({ type: 1, page: page.value, limit: 20 }),
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
  } catch (error) {
    message.error(error instanceof Error ? error.message : "删除历史失败");
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
      <div v-if="data?.list.length" class="article-history-list">
        <article v-for="item in data?.list" :key="`${item.article_id}-${item.updated_at}`" class="article-feed-item article-history-card">
          <NuxtLink :to="`/article/${item.article_id}`" class="article-feed-cover article-history-card__cover">
            <ArticleCover :src="item.cover" :title="item.title" label="History" />
          </NuxtLink>

          <div class="article-feed-body">
            <div class="article-feed-meta">
              <span>{{ formatDateTimeLabel(item.updated_at) }}</span>
              <span>{{ item.nickname }}</span>
            </div>
            <NuxtLink :to="`/article/${item.article_id}`" class="article-feed-title article-feed-title--compact">
              {{ item.title }}
            </NuxtLink>
            <p class="article-feed-abstract line-clamp-2">
              上次读到这里，点击继续回到文章详情。
            </p>
            <div class="article-feed-footer">
              <NuxtLink :to="`/article/${item.article_id}`" class="glass-badge">继续阅读</NuxtLink>
              <NButton quaternary size="small" @click="removeOne(item.article_id)">移出历史</NButton>
            </div>
          </div>
        </article>
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
.article-history-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.article-history-card {
  padding: 14px;
}

.article-history-card__cover {
  width: 176px;
  height: 118px;
  border-radius: 18px;
}

@media (max-width: 768px) {
  .article-history-card__cover {
    width: 100%;
    height: 196px;
  }
}
</style>
