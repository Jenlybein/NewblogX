<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NCard, NEmpty, NInput, NModal, NSkeleton, useMessage } from "naive-ui";
import type { FavoriteFolderItem } from "~/types/api";
import { createFavoriteFolder, getOwnFavoriteFolders } from "~/services/favorite";
import { favoriteArticle } from "~/services/article";

const props = defineProps<{
  show: boolean;
  articleId: string;
  articleTitle: string;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
  updated: [];
}>();

const folders = ref<FavoriteFolderItem[]>([]);
const pending = ref(false);
const createPending = ref(false);
const actionPendingId = ref<string | null>(null);
const showCreateForm = ref(false);
const newFolderTitle = ref("");
const newFolderAbstract = ref("");

const folderCountText = computed(() => `${folders.value.length} 个收藏夹`);
const sortedFolders = computed(() =>
  [...folders.value].sort((left, right) => {
    if (left.is_default !== right.is_default) {
      return left.is_default ? -1 : 1;
    }
    if (left.has_article !== right.has_article) {
      return left.has_article ? -1 : 1;
    }
    return left.title.localeCompare(right.title, "zh-CN");
  }),
);
const message = useMessage();
const modalOpen = computed({
  get: () => props.show,
  set: (value: boolean) => emit("update:show", value),
});

function closeModal() {
  emit("update:show", false);
}

function resetCreateForm() {
  showCreateForm.value = false;
  newFolderTitle.value = "";
  newFolderAbstract.value = "";
}

async function loadFolders() {
  pending.value = true;
  try {
    const payload = await getOwnFavoriteFolders(props.articleId);
    folders.value = payload.list ?? [];
  } catch (error) {
    message.error(error instanceof Error ? error.message : "加载收藏夹失败");
  } finally {
    pending.value = false;
  }
}

async function handleToggleFolder(folder: FavoriteFolderItem) {
  actionPendingId.value = folder.id;
  try {
    const wasIncluded = folder.has_article;
    await favoriteArticle(props.articleId, folder.id);
    await loadFolders();
    message.success(wasIncluded ? `已从“${folder.title}”移除` : `已加入“${folder.title}”`);
    emit("updated");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "收藏操作失败");
  } finally {
    actionPendingId.value = null;
  }
}

async function handleAddToDefaultFolder() {
  actionPendingId.value = "__default__";
  try {
    await favoriteArticle(props.articleId);
    await loadFolders();
    message.success("已加入默认收藏夹");
    emit("updated");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "收藏操作失败");
  } finally {
    actionPendingId.value = null;
  }
}

async function handleCreateFolder() {
  const title = newFolderTitle.value.trim();
  if (!title) {
    return;
  }

  createPending.value = true;
  try {
    const created = await createFavoriteFolder({
      title,
      abstract: newFolderAbstract.value.trim() || `${title} 收藏夹`,
    });
    await favoriteArticle(props.articleId, created.id);
    resetCreateForm();
    await loadFolders();
    message.success(`已创建“${title}”并完成收藏`);
    emit("updated");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "创建收藏夹失败");
  } finally {
    createPending.value = false;
  }
}

watch(
  () => props.show,
  async (opened) => {
    if (!opened) {
      resetCreateForm();
      return;
    }

    await loadFolders();
  },
);
</script>

<template>
  <NModal
    :show="modalOpen"
    preset="card"
    style="max-width: 640px; margin: 0 auto;"
    :mask-closable="!pending && !createPending"
    @update:show="modalOpen = $event"
  >
    <NCard
      title="选择收藏夹"
      :bordered="false"
      class="surface-card surface-card--strong"
      @close="closeModal"
    >
      <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="text-sm muted">当前文章</div>
          <div class="mt-1 text-base font-semibold">{{ props.articleTitle }}</div>
        </div>
        <div class="glass-badge">{{ folderCountText }}</div>
      </div>

      <div class="max-h-[360px] overflow-y-auto rounded-[22px] border border-slate-200/80 px-2 pt-2 pr-1 dark:border-slate-700/70">
        <template v-if="pending">
          <div v-for="index in 3" :key="index" class="surface-section mb-3 p-4 last:mb-0">
            <NSkeleton text :repeat="2" />
          </div>
        </template>

        <template v-else-if="folders.length">
          <div
            v-for="folder in sortedFolders"
            :key="folder.id"
            class="surface-section mb-3 flex flex-col gap-3 p-4 last:mb-0 md:flex-row md:items-center md:justify-between"
          >
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <div class="font-semibold">{{ folder.title }}</div>
                <span v-if="folder.is_default" class="glass-badge">默认</span>
                <span v-if="folder.has_article" class="glass-badge">已包含当前文章</span>
              </div>
              <p class="mt-2 text-sm leading-6 muted">
                {{ folder.abstract || "这个收藏夹还没有补充说明。" }}
              </p>
              <div class="mt-2 text-xs muted">
                共 {{ folder.article_count }} 篇文章
              </div>
            </div>

            <NButton
              :type="folder.has_article ? 'default' : 'primary'"
              :ghost="!folder.has_article"
              :loading="actionPendingId === folder.id"
              @click="handleToggleFolder(folder)"
            >
              {{ folder.has_article ? "移出收藏夹" : "加入收藏夹" }}
            </NButton>
          </div>
        </template>

        <template v-else>
          <div class="surface-section p-6">
            <NEmpty description="你还没有收藏夹，先创建一个，或者直接放进默认收藏夹。" />
            <div class="mt-4 flex flex-wrap justify-center gap-3">
              <NButton
                type="primary"
                ghost
                :loading="actionPendingId === '__default__'"
                @click="handleAddToDefaultFolder"
              >
                收藏到默认收藏夹
              </NButton>
              <NButton quaternary @click="showCreateForm = true">新建收藏夹</NButton>
            </div>
          </div>
        </template>
      </div>

      <div class="mt-5 flex flex-wrap items-center justify-between gap-3">
        <div class="text-sm muted">收藏操作会按收藏夹维度生效，支持一篇文章被多个收藏夹收录。</div>
        <NButton quaternary @click="showCreateForm = !showCreateForm">
          {{ showCreateForm ? "收起新建" : "新建收藏夹" }}
        </NButton>
      </div>

      <div v-if="showCreateForm" class="surface-section mt-4 space-y-3 p-4">
        <div class="text-sm font-semibold">新建收藏夹</div>
        <NInput
          v-model:value="newFolderTitle"
          maxlength="32"
          show-count
          placeholder="输入收藏夹名称，例如：架构灵感"
        />
        <NInput
          v-model:value="newFolderAbstract"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 4 }"
          maxlength="256"
          show-count
          placeholder="补一句说明，方便后面回看。留空则自动生成默认摘要。"
        />
        <div class="flex justify-end gap-3">
          <NButton quaternary @click="resetCreateForm">取消</NButton>
          <NButton type="primary" :loading="createPending" @click="handleCreateFolder">创建并收藏</NButton>
        </div>
      </div>
    </NCard>
  </NModal>
</template>
