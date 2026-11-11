<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NCard, NEmpty, NInput, NModal, NSkeleton, useMessage } from "naive-ui";
import type {
  ArticleFavoriteRemoveResult,
  ArticleFavoriteResult,
  FavoriteFolderItem,
} from "~/types/api";
import { createFavoriteFolder, getOwnFavoriteFolders, removeFavoriteFolderArticles } from "~/services/favorite";
import { favoriteArticle } from "~/services/article";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { errorMessage } from "~/utils/errorMessage";

const props = defineProps<{
  show: boolean;
  articleId: string;
  articleTitle: string;
}>();

// 弹窗是厚玻璃浮层：指针镜面高光由统一 composable 写入 --mx/--my（移动端 / 减少动态关闭）。
const { handlePointerMove } = usePointerSpecular();

const emit = defineEmits<{
  "update:show": [value: boolean];
  /**
   * 收藏写响应里的权威状态与计数（ADR-0024 决策 4/6）。
   *
   * `null` 表示这次响应**不可用**（没有 data，或字段缺失）：页面据此跳过 patch，
   * 不再让 `payload.is_favor` 直接抛 TypeError（评审票 248 / P0-2）。
   */
  updated: [payload: ArticleFavoriteResult | null];
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

/**
 * 把写响应交给页面（或交一个明确的 `null`）。
 *
 * 为什么不能直接 `emit("updated", result)`：失败分支历史上返回过空 data，
 * 页面读 `payload.is_favor` 会直接 TypeError（评审票 248 / P0-2）。这里做空值防御，
 * 形状不对就当"这次没有可用载荷"，由页面跳过 patch、只失效缓存。
 */
function emitFavoriteResult(
  result: ArticleFavoriteResult | ArticleFavoriteRemoveResult | null | undefined,
) {
  const available =
    result &&
    typeof result.is_favor === "boolean" &&
    typeof result.favor_count === "number"
      ? result
      : null;
  emit("updated", available);
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
    message.error(errorMessage(error, "加载收藏夹失败"));
  } finally {
    pending.value = false;
  }
}

async function handleToggleFolder(folder: FavoriteFolderItem) {
  actionPendingId.value = folder.id;
  try {
    const wasIncluded = folder.has_article;
    // 关键：已收藏时走 DELETE（移除），不是再 POST 一次。
    // 旧实现两个分支都发 POST，于是"移出收藏夹"必然撞 ErrItemExists。
    const result = wasIncluded
      ? await removeFavoriteFolderArticles({ favoriteId: folder.id, articleIds: [props.articleId] })
      : await favoriteArticle(props.articleId, folder.id);
    await loadFolders();
    // 批量接口会带回 `failed_count`：单篇场景全失败已由后端返回非 0 业务码进 catch，
    // 这里只有当"部分失败"时才不能再宣称全部成功（评审票 248 / P0-2）。
    if (wasIncluded) {
      const removeResult = result as ArticleFavoriteRemoveResult;
      if (removeResult.failed_count > 0) {
        message.warning(`已移出 ${removeResult.removed_count} 篇，${removeResult.failed_count} 篇未移除`);
      } else {
        message.success(`已从“${folder.title}”移除`);
      }
    } else {
      message.success(`已加入“${folder.title}”`);
    }
    emitFavoriteResult(result);
  } catch (error) {
    message.error(errorMessage(error, "收藏操作失败"));
  } finally {
    actionPendingId.value = null;
  }
}

async function handleAddToDefaultFolder() {
  actionPendingId.value = "__default__";
  try {
    const result = await favoriteArticle(props.articleId);
    await loadFolders();
    message.success("已加入默认收藏夹");
    emitFavoriteResult(result);
  } catch (error) {
    message.error(errorMessage(error, "收藏操作失败"));
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
    // 后端 `PUT /articles/favorite` 只返回消息、**不返回新收藏夹的 id**
    // （注解是 `Envelope[EmptyData]`，handler 走 `OkWithMsg`）。
    // 所以必须重新拉一次列表再按标题找回来——之前这里读 `created.id`，
    // 拿到的永远是 undefined，于是"创建并收藏"实际收藏进了**默认收藏夹**。
    await createFavoriteFolder({
      title,
      abstract: newFolderAbstract.value.trim() || `${title} 收藏夹`,
    });
    await loadFolders();
    const created = folders.value.find((item) => item.title === title);
    const result = created ? await favoriteArticle(props.articleId, created.id) : null;
    resetCreateForm();
    message.success(created ? `已创建“${title}”并完成收藏` : `已创建“${title}”`);
    if (result) {
      emitFavoriteResult(result);
    }
  } catch (error) {
    message.error(errorMessage(error, "创建收藏夹失败"));
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
    :mask-closable="!pending && !createPending"
    @update:show="modalOpen = $event"
  >
    <NCard
      title="选择收藏夹"
      :bordered="false"
      closable
      class="glass-thick"
      :style="{
        '--n-color': 'var(--glass-thick-fill)',
        '--n-border-radius': 'var(--radius-xl)',
        maxWidth: '640px',
        width: 'calc(100% - 32px)',
        margin: '0 auto',
      }"
      @close="closeModal"
      @pointermove="handlePointerMove"
    >
      <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="text-sm muted">当前文章</div>
          <div class="mt-1 text-base font-semibold">{{ props.articleTitle }}</div>
        </div>
        <div class="glass-badge">{{ folderCountText }}</div>
      </div>

      <div class="max-h-[360px] overflow-y-auto rounded-card border border-line px-2 pt-2 pr-1">
        <template v-if="pending">
          <div v-for="index in 3" :key="index" class="surface-section mb-3 p-4 last:mb-0">
            <NSkeleton text :repeat="2" />
          </div>
        </template>

        <template v-else-if="folders.length">
          <!-- 两行紧凑行（票 345）：名称 + 篇数徽标在第一行，简介一行在第二行；
               右侧是圆形 ＋ / ✓ 按钮 —— 一屏能看十几个夹子，点一下即收 / 移。
               之前是 640px 大卡 + 文字按钮，一屏只看得到三四个。 -->
          <div
            v-for="folder in sortedFolders"
            :key="folder.id"
            class="favorite-chooser-row"
            :class="{ 'is-included': folder.has_article }"
            :data-role="`favorite-folder-row-${folder.id}`"
          >
            <div class="favorite-chooser-row__body">
              <div class="favorite-chooser-row__line">
                <span class="favorite-chooser-row__title">{{ folder.title }}</span>
                <span v-if="folder.is_default" class="favorite-chooser-row__badge">默认</span>
                <span class="favorite-chooser-row__count" :data-role="`favorite-folder-count-${folder.id}`">
                  {{ folder.article_count ?? 0 }} 篇
                </span>
              </div>
              <p class="favorite-chooser-row__abstract">{{ folder.abstract || "还没有简介" }}</p>
            </div>

            <button
              type="button"
              class="favorite-chooser-row__toggle"
              :class="{ 'is-included': folder.has_article }"
              :disabled="actionPendingId === folder.id"
              :aria-pressed="folder.has_article"
              :aria-label="folder.has_article ? `从「${folder.title}」移出` : `收藏到「${folder.title}」`"
              :data-role="`favorite-folder-toggle-${folder.id}`"
              @click="handleToggleFolder(folder)">
              {{ actionPendingId === folder.id ? "…" : folder.has_article ? "✓" : "＋" }}
            </button>
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
