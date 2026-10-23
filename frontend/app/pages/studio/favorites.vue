<script setup lang="ts">
import { computed, watch } from "vue";
import { NButton, NInput, NModal, NSkeleton, useMessage } from "naive-ui";
import ArticleCover from "~/components/article/ArticleCover.vue";
import {
  createFavoriteFolder,
  deleteFavoriteFolders,
  getFavoriteFolderArticles,
  getOwnFavoriteFolders,
  removeFavoriteFolderArticles,
  updateFavoriteFolder,
} from "~/services/favorite";
import { formatDateTimeLabel } from "~/utils/format";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const activeFolderId = shallowRef<string>("");
const contentPage = shallowRef(1);
const editModalVisible = shallowRef(false);
const editForm = reactive({
  title: "",
  abstract: "",
});

const { data: folders, pending: foldersPending, refresh: refreshFolders } = await useAsyncData(
  "studio-favorite-folders",
  () => getOwnFavoriteFolders(),
);
const { data: contents, pending: contentsPending, refresh: refreshContents } = await useAsyncData(
  () => `studio-favorite-contents:${activeFolderId.value}:${contentPage.value}`,
  () =>
    activeFolderId.value
      ? getFavoriteFolderArticles({ favoriteId: activeFolderId.value, page: contentPage.value, limit: 30 })
      : Promise.resolve({ list: [], has_more: false }),
  {
    watch: [activeFolderId, contentPage],
  },
);

watch(
  () => folders.value?.list,
  (list) => {
    if (!list?.length) {
      activeFolderId.value = "";
      editForm.title = "";
      editForm.abstract = "";
      return;
    }

    if (!activeFolderId.value || !list.some((item) => item.id === activeFolderId.value)) {
      activeFolderId.value = list[0]?.id ?? "";
    }
    contentPage.value = 1;
  },
  { immediate: true },
);

const currentFolder = computed(() => folders.value?.list.find((item) => item.id === activeFolderId.value));

watch(
  () => activeFolderId.value,
  () => {
    contentPage.value = 1;
  },
);

async function handleCreateFolder() {
  try {
    await createFavoriteFolder({
      title: "新的收藏夹",
      abstract: "为稍后阅读保留一组主题内容。",
    });
    message.success("收藏夹已创建");
    await refreshFolders();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "创建收藏夹失败");
  }
}

async function handleUpdateFolder() {
  if (!activeFolderId.value) return;
  try {
    await updateFavoriteFolder({
      id: activeFolderId.value,
      title: editForm.title.trim(),
      abstract: editForm.abstract.trim(),
    });
    message.success("收藏夹已更新");
    editModalVisible.value = false;
    await refreshFolders();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "更新收藏夹失败");
  }
}

function openEditModal() {
  if (!currentFolder.value) return;
  editForm.title = currentFolder.value.title ?? "";
  editForm.abstract = currentFolder.value.abstract ?? "";
  editModalVisible.value = true;
}

async function handleDeleteFolder() {
  if (!activeFolderId.value) return;
  try {
    await deleteFavoriteFolders([activeFolderId.value]);
    message.success("收藏夹已删除");
    await refreshFolders();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "删除收藏夹失败");
  }
}

async function removeArticle(articleId: string) {
  if (!activeFolderId.value) return;
  try {
    await removeFavoriteFolderArticles({
      favoriteId: activeFolderId.value,
      articleIds: [articleId],
    });
    message.success("文章已移出收藏夹");
    await refreshContents();
    await refreshFolders();
    if (!contents.value?.list.length && contentPage.value > 1) {
      contentPage.value -= 1;
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : "移除收藏内容失败");
  }
}

function previousContentPage() {
  if (contentPage.value <= 1) return;
  contentPage.value -= 1;
}

function nextContentPage() {
  if (!contents.value?.has_more) return;
  contentPage.value += 1;
}

useSeoMeta({
  title: "个人中心 - 收藏夹",
});
</script>

<template>
  <div class="page-stack">
    <section class="surface-card studio-inbox-card">
      <div class="studio-inbox-grid">
        <aside class="studio-inbox-grid__aside">
          <div class="studio-toolbar studio-toolbar--stack">
            <div>
              <h2 class="section-title">收藏夹分组</h2>
            </div>
            <NButton type="primary" @click="handleCreateFolder()">新建收藏夹</NButton>
          </div>

          <div class="mt-4 space-y-2">
            <template v-if="foldersPending">
              <div
                v-for="idx in 4"
                :key="`folder-skeleton-${idx}`"
                class="rounded-2xl border border-slate-200/70 bg-white/75 px-4 py-3"
              >
                <NSkeleton text width="70%" />
                <NSkeleton text width="50%" class="mt-2" />
              </div>
            </template>
            <template v-else>
              <button
                v-for="folder in folders?.list || []"
                :key="folder.id"
                type="button"
                class="studio-filter-chip studio-filter-chip--stack"
                :class="{ 'is-active': activeFolderId === folder.id }"
                @click="activeFolderId = folder.id"
              >
                <span class="studio-filter-chip__main">
                  <strong>{{ folder.title }}</strong>
                  <small>{{ folder.abstract || "暂未填写简介" }}</small>
                </span>
                <span class="studio-sidebar__badge">{{ folder.article_count }}</span>
              </button>
            </template>
          </div>
        </aside>

        <div class="studio-inbox-grid__main space-y-5">
          <section class="surface-section p-4 md:p-5">
            <div class="studio-toolbar">
              <div>
                <template v-if="foldersPending">
                  <NSkeleton text width="224px" height="28px" />
                  <NSkeleton text width="288px" class="mt-2" />
                </template>
                <template v-else>
                  <h2 class="section-title">{{ currentFolder?.title || "未选择收藏夹" }}</h2>
                  <p class="muted">{{ currentFolder?.abstract || "暂未填写简介" }}</p>
                </template>
              </div>
              <div class="flex flex-wrap gap-2">
                <NButton quaternary @click="openEditModal()" :disabled="!activeFolderId">修改资料</NButton>
                <NButton quaternary type="error" @click="handleDeleteFolder()" :disabled="!activeFolderId">删除分组</NButton>
              </div>
            </div>
          </section>

          <section class="studio-list-card !p-0">
            <div class="studio-toolbar border-b border-slate-200/70 px-5 py-4 dark:border-slate-700/70">
              <div>
                <h2 class="section-title">分组内容</h2>
                <p class="muted">当前第 {{ contentPage }} 页</p>
              </div>
            </div>

            <div v-if="contents?.list.length" class="favorite-content-list">
              <article v-for="item in contents?.list" :key="item.article_id" class="favorite-content-card">
                <NuxtLink :to="`/article/${item.article_id}`" class="favorite-content-card__cover-wrap" aria-label="查看文章详情">
                  <ArticleCover :src="item.cover" :title="item.title" />
                </NuxtLink>

                <div class="favorite-content-card__main">
                  <NuxtLink :to="`/article/${item.article_id}`" class="favorite-content-card__title">
                    {{ item.title }}
                  </NuxtLink>
                  <p class="favorite-content-card__abstract">
                    {{ item.abstract || "这篇文章暂无摘要，点击查看原文获取完整内容。" }}
                  </p>
                  <div class="favorite-content-card__meta">
                    <span>{{ item.user_nickname }}</span>
                    <span>{{ item.view_count }} 阅读</span>
                    <span>收藏于 {{ formatDateTimeLabel(item.favorited_at) }}</span>
                  </div>
                </div>

                <div class="favorite-content-card__actions">
                  <NuxtLink :to="`/article/${item.article_id}`" class="glass-badge">查看原文</NuxtLink>
                  <NButton quaternary size="small" @click="removeArticle(item.article_id)">移出</NButton>
                </div>
              </article>
            </div>
            <div v-else-if="contentsPending" class="favorite-content-list">
              <article v-for="idx in 4" :key="`content-skeleton-${idx}`" class="favorite-content-card">
                <NSkeleton class="favorite-content-card__cover-skeleton" />
                <div class="favorite-content-card__main">
                  <NSkeleton text width="75%" height="24px" />
                  <NSkeleton text :repeat="2" class="mt-2" />
                  <NSkeleton text width="66%" class="mt-2" />
                </div>
                <div class="favorite-content-card__actions">
                  <NSkeleton round height="32px" width="64px" />
                  <NSkeleton round height="32px" width="48px" />
                </div>
              </article>
            </div>
            <StudioEmptyState
              v-else
              title="这个收藏夹还是空的"
              :description="'先去文章详情页收藏内容，或者切换到另一个分组继续整理。'"
              class="m-5"
            />

            <div v-if="contents?.list.length" class="px-5 pb-5 flex flex-wrap items-center justify-between gap-3 text-sm muted">
              <span>第 {{ contentPage }} 页</span>
              <div class="flex items-center gap-3">
                <NButton quaternary size="small" :disabled="contentPage <= 1 || contentsPending" @click="previousContentPage()">上一页</NButton>
                <NButton quaternary size="small" :disabled="!contents?.has_more || contentsPending" @click="nextContentPage()">下一页</NButton>
              </div>
            </div>
          </section>
        </div>
      </div>
    </section>

    <NModal v-model:show="editModalVisible" preset="card" title="修改收藏夹资料" style="max-width: 560px" :mask-closable="false">
      <div class="grid gap-4 md:grid-cols-2">
        <label class="space-y-2">
          <span class="text-sm font-medium">收藏夹名称</span>
          <NInput v-model:value="editForm.title" maxlength="40" />
        </label>
        <label class="space-y-2 md:col-span-2">
          <span class="text-sm font-medium">收藏夹简介</span>
          <NInput v-model:value="editForm.abstract" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" maxlength="120" />
        </label>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton quaternary @click="editModalVisible = false">取消</NButton>
          <NButton type="primary" @click="handleUpdateFolder()" :disabled="!activeFolderId">保存修改</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.favorite-content-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
}

.favorite-content-card {
  display: grid;
  grid-template-columns: 176px minmax(0, 1fr) auto;
  gap: 14px;
  align-items: stretch;
  border: 1px solid rgba(217, 226, 236, 0.8);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.84);
  padding: 10px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
}

.favorite-content-card:hover {
  border-color: rgba(15, 118, 110, 0.24);
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.08);
  transform: translateY(-1px);
}

.favorite-content-card__cover-wrap {
  border-radius: 14px;
  overflow: hidden;
  display: block;
  height: 100%;
  min-height: 112px;
}

.favorite-content-card__cover {
  width: 100%;
  height: 100%;
  min-height: 112px;
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
}

.favorite-content-card__cover-empty {
  width: 100%;
  height: 112px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed rgba(148, 163, 184, 0.65);
  border-radius: 14px;
  color: #94a3b8;
  font-size: 12px;
  letter-spacing: 0.14em;
}

.favorite-content-card__cover-skeleton {
  width: 100%;
  height: 100%;
  min-height: 112px;
  border-radius: 14px;
}

.favorite-content-card__main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.favorite-content-card__title {
  color: #1e293b;
  font-size: 18px;
  line-height: 1.35;
  font-weight: 700;
  text-decoration: none;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.favorite-content-card__title:hover {
  color: #0f766e;
}

.favorite-content-card__abstract {
  margin: 0;
  color: #64748b;
  font-size: 14px;
  line-height: 1.65;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.favorite-content-card__meta {
  margin-top: auto;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  color: #64748b;
  font-size: 13px;
}

.favorite-content-card__actions {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  justify-content: center;
  gap: 10px;
  min-width: 88px;
}

@media (max-width: 980px) {
  .favorite-content-card {
    grid-template-columns: 144px minmax(0, 1fr);
    gap: 12px;
  }

  .favorite-content-card__actions {
    grid-column: 1 / -1;
    flex-direction: row;
    justify-content: flex-end;
    padding-top: 4px;
  }

  .favorite-content-card__cover,
  .favorite-content-card__cover-empty {
    min-height: 98px;
  }
}

@media (max-width: 640px) {
  .favorite-content-list {
    padding: 10px;
    gap: 10px;
  }

  .favorite-content-card {
    grid-template-columns: 1fr;
    padding: 10px;
  }

  .favorite-content-card__cover,
  .favorite-content-card__cover-empty {
    min-height: 154px;
  }

  .favorite-content-card__actions {
    justify-content: flex-start;
  }
}
</style>
