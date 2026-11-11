<script setup lang="ts">
import { computed, watch } from "vue";
import { NButton, NInput, NModal, NSkeleton, useMessage } from "naive-ui";
import ArticleMiniCard from "~/components/article/ArticleMiniCard.vue";
import {
  createFavoriteFolder,
  deleteFavoriteFolders,
  getFavoriteFolderArticles,
  getOwnFavoriteFolders,
  removeFavoriteFolderArticles,
  updateFavoriteFolder,
} from "~/services/favorite";
import { formatDateTimeLabel } from "~/utils/format";
import { errorMessage } from "~/utils/errorMessage";

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
      ? getFavoriteFolderArticles({
          favoriteId: activeFolderId.value,
          page: contentPage.value,
          limit: 30,
          // 本人读自己的夹子：必须带 token，否则非默认夹会被 403 吞成空态（票 344）。
          auth: true,
        })
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
    message.error(errorMessage(error, "创建收藏夹失败"));
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
    message.error(errorMessage(error, "更新收藏夹失败"));
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
    message.error(errorMessage(error, "删除收藏夹失败"));
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
    message.error(errorMessage(error, "移除收藏内容失败"));
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
useNoIndexPage();
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
                class="rounded-card border border-line bg-surface px-4 py-3"
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
            <div class="studio-toolbar border-b border-line px-5 py-4">
              <div>
                <h2 class="section-title">分组内容</h2>
                <p class="muted">当前第 {{ contentPage }} 页</p>
              </div>
            </div>

            <!-- 卡片本体是共享组件：与浏览历史同一个形状（票 327），差异只有元信息那一行。
                 整张卡（封面与标题）都指向详情页，所以不再单独排一个"查看原文"按钮。 -->
            <div v-if="contents?.list.length" class="favorite-content-grid">
              <ArticleMiniCard
                v-for="item in contents?.list"
                :key="item.article_id"
                :title="item.title"
                :cover="item.cover"
                :to="`/article/${item.article_id}`"
                :meta="`${item.user_nickname} · ${item.view_count} 阅读 · 收藏于 ${formatDateTimeLabel(item.favorited_at)}`"
                removable
                remove-label="移出收藏夹"
                @remove="removeArticle(item.article_id)"
              />
            </div>
            <div v-else-if="contentsPending" class="favorite-content-grid">
              <article v-for="idx in 4" :key="`content-skeleton-${idx}`" class="favorite-content-skeleton">
                <NSkeleton class="favorite-content-skeleton__cover" />
                <NSkeleton text width="75%" height="20px" />
                <NSkeleton text width="60%" />
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
/* 颜色一律消费语义 token。卡片本体在 app/components/article/ArticleMiniCard.vue：
   与浏览历史同一个形状，这里只负责栅格列数与骨架占位。 */

/* 这一栏是个人中心的右半区，容器本来就窄：最多两列，窄屏一列。
   历史页是整页宽度，所以那边能铺到 5 列（同一个卡片、不同的栅格）。 */
.favorite-content-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  padding: 12px;
}

@media (min-width: 720px) {
  .favorite-content-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* 骨架也必须用同一个栅格，否则加载时与加载后的版式会跳一下。 */
.favorite-content-skeleton {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
}

.favorite-content-skeleton__cover {
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: var(--radius-sm);
}
</style>
