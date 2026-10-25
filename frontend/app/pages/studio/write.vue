<script setup lang="ts">
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import type { StudioAiAction } from "~/composables/useStudioAiSelection";
import { computed, nextTick, reactive, ref, watch } from "vue";
import { useDebounce } from "@vueuse/core";
import { useMessage } from "naive-ui";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import StudioAiAssistPanel from "~/components/studio/StudioAiAssistPanel.vue";
import StudioSelectionAiToolbar from "~/components/studio/StudioSelectionAiToolbar.vue";
import StudioWritePreview from "~/components/studio/StudioWritePreview.vue";
import StudioWritePublishDialog from "~/components/studio/StudioWritePublishDialog.vue";
import StudioWriteToc from "~/components/studio/StudioWriteToc.vue";
import StudioWriteHeader from "~/components/studio/StudioWriteHeader.vue";
import { useStudioAiSelection } from "~/composables/useStudioAiSelection";
import { useStudioEditorCommands } from "~/composables/useStudioEditorCommands";
import { useStudioArticle } from "~/composables/useStudioArticle";
import { useStudioMarkdownTheme } from "~/composables/useStudioMarkdownTheme";
import { useStudioPublish } from "~/composables/useStudioPublish";
import { useStudioScore } from "~/composables/useStudioScore";
import { useStudioToolbar } from "~/composables/useStudioToolbar";
import { useTextareaSelectionOverlay } from "~/composables/useTextareaSelectionOverlay";
import { updateArticle } from "~/services/article";
import type { getArticleScoreDetail } from "~/services/ai";
import { generateArticleMetainfo, regenerateArticleScore } from "~/services/ai";
import { resolveAvatarInitial } from "~/utils/avatar";

definePageMeta({
  layout: "write",
  middleware: "auth",
});


const router = useRouter();
const authStore = useAuthStore();
const message = useMessage();
const editorTextareaRef = ref<HTMLTextAreaElement | null>(null);
const shadowPreviewRef = ref<{ scrollToHeading: (id: string) => boolean } | null>(null);
const previewHeadings = ref<ArticleHeadingAnchor[]>([]);

if (!authStore.profileId) {
  await authStore.fetchCurrentUser();
}

const pendingState = reactive({
  draft: false,
  publish: false,
  ai: false,
  cover: false,
});

// 文章数据域见 composable：表单、编辑态回填、tag/分类选项、封面上传、提交载荷。
//
// 这些状态**只能一起搬**：hydrate 写 form、封面写 form.cover_image_id、
// 提交读整个 form。留在页面里时，"谁改了这个字段"要靠通读 1200 行才能回答。
const {
  form,
  articleTitleModel,
  articleContentModel,
  editArticleId,
  isEditMode,
  hydratedArticleId,
  tagOptions,
  categoryOptions,
  editingArticle,
  coverPreviewUrl,
  coverUploadStage,
  coverDirty,
  clearCoverSelection,
  handleCoverFileChange,
  buildArticlePayload,
  canSubmit,
  contentStats,
  tocItems,
  canOpenArticleScore,
  currentArticlePublishStatus,
  draftActionLabel,
  publishActionLabel,
  isPublishingLocked,
} = await useStudioArticle({ pendingState, message, previewHeadings });

const articleScoreModalOpen = ref(false);
const articleScoreLoading = ref(false);
const articleScoreRefreshing = ref(false);
const articleScoreData = ref<Awaited<ReturnType<typeof getArticleScoreDetail>> | null>(null);

const showToc = ref(true);
const viewMode = ref<"split" | "editor" | "preview">("split");
const publishVisible = ref(false);
// Markdown 主题（12 套 CSS 的映射表）见 composable。
const markdownTheme = useStudioMarkdownTheme();
const { visibilityOptions, selectedThemeLabel, activeMarkdownThemeHref, markdownSupportStyleHrefs } = markdownTheme;


const currentUserInitial = computed(() => resolveAvatarInitial(authStore.profileName, "我"));



watch(editArticleId, () => {
  articleScoreData.value = null;
  articleScoreModalOpen.value = false;
});

const debouncedContent = useDebounce(computed(() => form.content), 300);

const writeBodyClass = computed(() => ({
  "write-page__body--hide-toc": !showToc.value,
  "write-page__body--editor-only": viewMode.value === "editor",
  "write-page__body--preview-only": viewMode.value === "preview",
}));



const editorCommands = useStudioEditorCommands({ form, textareaRef: editorTextareaRef });
// 页面自己只用这两个：@keydown 的处理器与"聚焦到底部"。
// 其余命令由 useStudioToolbar 拿去接线，页面不需要知道它们存在。
const { focusEditor, handleEditorShortcuts } = editorCommands;



const {
  panelVisible: aiPanelVisible,
  currentAction: aiPanelAction,
  styleInstruction: aiStyleInstruction,
  selectionContext: aiSelectionContext,
  overwriteResult: aiOverwriteResult,
  diagnoseResult: aiDiagnoseResult,
  errorMessage: aiErrorMessage,
  isBusy: aiPanelBusy,
  canSubmit: aiCanSubmit,
  openPanel: openAiPanel,
  closePanel: closeAiPanel,
  runCurrentAction: runAiPanelAction,
  applyOverwriteResult: applyAiOverwriteResult,
  insertOverwriteBelow: insertAiOverwriteBelow,
  copyOverwriteResult: copyAiOverwriteResult,
} = useStudioAiSelection({
  content: articleContentModel,
  title: articleTitleModel,
  textareaRef: editorTextareaRef,
  focusEditor,
});

const {
  selectionOverlay,
  updateSelectionOverlay,
  hideSelectionOverlay,
} = useTextareaSelectionOverlay(editorTextareaRef, {
  hideWhen: () => aiPanelVisible.value || publishVisible.value || viewMode.value === "preview",
  ignoreSelectors: [".studio-selection-ai-toolbar", ".studio-ai-modal"],
});

watch([aiPanelVisible, publishVisible, viewMode], ([isAiOpen, isPublishOpen, mode]) => {
  if (isAiOpen || isPublishOpen || mode === "preview") {
    hideSelectionOverlay();
    return;
  }

  nextTick(() => {
    updateSelectionOverlay();
  });
});

watch(
  () => form.content,
  () => {
    nextTick(() => {
      updateSelectionOverlay();
    });
  },
);


async function handleAiToolOpen(action: StudioAiAction) {
  try {
    hideSelectionOverlay();
    await openAiPanel(action);
  } catch (error) {
    message.warning(error instanceof Error ? error.message : "AI 辅助暂时不可用");
  }
}

function handleSelectionToolbarAction(action: StudioAiAction) {
  void handleAiToolOpen(action);
}

function handleEditorSelectionChange() {
  updateSelectionOverlay();
}

async function handleAiPanelRun() {
  try {
    await runAiPanelAction();
  } catch (error) {
    if (error instanceof Error && error.message) {
      message.error(error.message);
      return;
    }
    message.error("AI 辅助执行失败");
  }
}

function handleAiReplace() {
  try {
    applyAiOverwriteResult();
    message.success("改写结果已替换到正文中");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "替换选区失败");
  }
}

function handleAiInsertBelow() {
  try {
    insertAiOverwriteBelow();
    message.success("改写结果已插入到选区下方");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "插入正文失败");
  }
}

async function handleAiCopyResult() {
  try {
    await copyAiOverwriteResult();
    message.success("改写结果已复制");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "复制失败");
  }
}

function handleAiPanelVisibility(nextVisible: boolean) {
  if (nextVisible) {
    aiPanelVisible.value = true;
    hideSelectionOverlay();
    return;
  }

  closeAiPanel();
  nextTick(() => {
    updateSelectionOverlay();
  });
}

function matchOptionByText(options: Array<{ label: string; value: string }>, target?: { id?: string; title?: string } | null) {
  if (!target) return null;
  const byId = options.find((item) => item.value === target.id);
  if (byId) return byId.value;
  const normalizedTitle = target.title?.trim().toLowerCase();
  const byTitle = options.find((item) => item.label.trim().toLowerCase() === normalizedTitle);
  return byTitle?.value ?? null;
}

async function handleAiAssist() {
  if (!form.content.trim()) {
    message.warning("先写一点正文，再让 AI 帮你补标题和摘要。");
    return;
  }

  pendingState.ai = true;
  try {
    const result = await generateArticleMetainfo(form.content);
    form.title = result.title?.trim() || form.title;
    form.abstract = result.abstract?.trim() || form.abstract;

    const matchedCategoryId = matchOptionByText(categoryOptions.value || [], result.category);
    if (matchedCategoryId) {
      form.category_id = matchedCategoryId;
    }

    const nextTagIds = (result.tags || [])
      .map((item) => matchOptionByText(tagOptions.value || [], item))
      .filter((item): item is string => Boolean(item));

    if (nextTagIds.length) {
      form.tag_ids = Array.from(new Set(nextTagIds));
    }

    message.success("AI 已根据正文补全元信息建议");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "AI 元信息生成失败");
  } finally {
    pendingState.ai = false;
  }
}



// 评分弹窗的数据加载见 composable（三个入口共用一套 loading 状态）。
const { handleOpenArticleScore } = useStudioScore({
  modalOpen: articleScoreModalOpen,
  loading: articleScoreLoading,
  refreshing: articleScoreRefreshing,
  data: articleScoreData,
  articleId: editArticleId,
  message,
});

// handleRefreshArticleScore 留在页面而不是 composable：它跨两个域 ——
// 先"必要时把当前内容存成草稿"（文章域 + 发布域），再请求重新评分（评分域）。
// 强行注入 5 个依赖塞进 useStudioScore，会让那个模块变成什么都懂的上帝对象。
async function handleRefreshArticleScore() {
  if (!editArticleId.value) {
    message.info("请先保存文章后再重新获取评分。");
    return;
  }

  articleScoreRefreshing.value = true;
  try {
    if (currentArticlePublishStatus.value !== 1) {
      await updateArticle(editArticleId.value, buildArticlePayload(1));
      if (editingArticle.value) {
        editingArticle.value = {
          ...editingArticle.value,
          title: form.title.trim(),
          abstract: form.abstract.trim(),
          content: form.content,
          comments_toggle: form.comments_toggle,
          category_id: form.category_id ?? null,
          tag_ids: [...form.tag_ids],
          cover_image_id: form.cover_image_id,
          cover: coverPreviewUrl.value || editingArticle.value.cover,
          publish_status: 1,
          status: 1,
          visibility_status: form.visibility_status,
        };
      }
      hydratedArticleId.value = editArticleId.value;
      coverDirty.value = false;
      message.success("当前文章已自动保存为草稿");
    }

    articleScoreData.value = await regenerateArticleScore({
      article_id: editArticleId.value,
      title: form.title.trim() || undefined,
      content: form.content.trim() || undefined,
    });
    message.success("已重新获取最新评分");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "重新获取评分失败");
  } finally {
    articleScoreRefreshing.value = false;
  }
}

// 提交流程见 composable（新建/编辑 × 草稿/发布 的 6 条分支集中在一处）。
const { submitArticle } = useStudioPublish({
  canSubmit,
  pendingState,
  isEditMode,
  editArticleId,
  publishVisible,
  visibilityStatus: () => form.visibility_status,
  buildPayload: buildArticlePayload,
  message,
  router,
});

// 工具栏分发见 composable（按钮 key → 插片段 / 改视图 / 开 AI）。
const {
  handleToolOption,
  isToolOptionActive,
  handleToolClick,
  handleRightToolClick,
  handleTocJump,
  handlePreviewHeadingsChange,
} = useStudioToolbar({
  editor: editorCommands,
  theme: markdownTheme,
  showToc,
  viewMode,
  previewRef: shadowPreviewRef,
  previewHeadings,
  openAi: (action) => void handleAiToolOpen(action),
});

useSeoMeta({
  title: "创作系统 - 写文章",
  description: "对齐原型工作台结构的正式创作页面。",
});
</script>

<template>
  <div class="write-page">
    <StudioWriteHeader
      v-model:title="form.title"
      :draft-pending="pendingState.draft"
      :draft-label="draftActionLabel"
      :is-edit-mode="isEditMode"
      :can-open-score="canOpenArticleScore"
      :show-toc="showToc"
      :view-mode="viewMode"
      :profile-id="authStore.profileId || ''"
      :profile-avatar="authStore.profileAvatar"
      :profile-name="authStore.profileName"
      :profile-initial="currentUserInitial"
      :is-option-active="isToolOptionActive"
      @open-score="handleOpenArticleScore"
      @save-draft="submitArticle(1)"
      @open-publish="publishVisible = true"
      @tool-click="handleToolClick"
      @tool-option="handleToolOption"
      @right-tool="handleRightToolClick" />

    <main class="write-page__body" :class="writeBodyClass">
      <StudioWriteToc
        :visible="showToc"
        :items="tocItems"
        @jump="handleTocJump" />

      <section class="write-page__pane write-page__pane--editor">
        <textarea
ref="editorTextareaRef" v-model="form.content" class="write-page__textarea"
          placeholder="从这里开始写作，把 API 能力、页面结构和组件关系整理清楚。"
          @keydown="handleEditorShortcuts"
          @keyup="handleEditorSelectionChange"
          @mouseup="handleEditorSelectionChange"
          @select="handleEditorSelectionChange"
          @scroll="handleEditorSelectionChange" />
      </section>

      <StudioWritePreview
        ref="shadowPreviewRef"
        :empty="!form.content.trim()"
        :source="debouncedContent"
        :theme-href="activeMarkdownThemeHref"
        :extra-style-hrefs="markdownSupportStyleHrefs"
        :theme-label="selectedThemeLabel"
        @headings-change="handlePreviewHeadingsChange" />
    </main>

    <footer class="write-page__statusbar">
      <span>字符数: {{ contentStats.raw }}</span>
      <span>正文字符数: {{ contentStats.pure }}</span>
      <span>行数: {{ contentStats.lines }}</span>
      <span>目录项: {{ previewHeadings.length }}</span>
      <span>视图: {{ viewMode === 'split' ? '双栏' : viewMode === 'editor' ? '仅编辑区' : '仅预览区' }}</span>
    </footer>

    <StudioSelectionAiToolbar
      :show="selectionOverlay.visible"
      :top="selectionOverlay.top"
      :left="selectionOverlay.left"
      @action="handleSelectionToolbarAction" />

    <StudioAiAssistPanel
      :show="aiPanelVisible"
      :action="aiPanelAction"
      :selection="aiSelectionContext"
      :style-instruction="aiStyleInstruction"
      :overwrite-result="aiOverwriteResult"
      :diagnose-result="aiDiagnoseResult"
      :pending="aiPanelBusy"
      :error-message="aiErrorMessage"
      :can-submit="aiCanSubmit"
      @update:show="handleAiPanelVisibility"
      @update:style-instruction="aiStyleInstruction = $event"
      @run="handleAiPanelRun"
      @replace="handleAiReplace"
      @insert-below="handleAiInsertBelow"
      @copy="handleAiCopyResult" />

    <ArticleScoreModal
      v-model:show="articleScoreModalOpen"
      title="文章质量评分"
      :loading="articleScoreLoading"
      :refreshing="articleScoreRefreshing"
      :data="articleScoreData"
      :allow-refresh="Boolean(editArticleId)"
      refresh-label="重新获取（自动保存文章为草稿）"
      empty-text="当前还没有历史评分缓存，可以点击“重新获取”生成最新评分。"
      @refresh="handleRefreshArticleScore" />

    <StudioWritePublishDialog
      v-model:show="publishVisible"
      v-model:category-id="form.category_id"
      v-model:tag-ids="form.tag_ids"
      v-model:abstract-text="form.abstract"
      v-model:comments-toggle="form.comments_toggle"
      v-model:visibility="form.visibility_status"
      :locked="isPublishingLocked"
      :draft-pending="pendingState.draft"
      :publish-pending="pendingState.publish"
      :ai-pending="pendingState.ai"
      :cover-uploading="pendingState.cover"
      :cover-preview-url="coverPreviewUrl"
      :cover-stage="coverUploadStage"
      :category-options="categoryOptions || []"
      :tag-options="tagOptions || []"
      :visibility-options="visibilityOptions"
      :draft-label="draftActionLabel"
      :publish-label="publishActionLabel"
      :on-cover-change="handleCoverFileChange"
      :on-clear="clearCoverSelection"
      @submit="submitArticle"
      @ai-fill="handleAiAssist" />
  </div>
</template>

<style scoped>
.write-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.write-page__body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr) minmax(340px, 1fr);
}

.write-page__body--hide-toc {
  grid-template-columns: minmax(0, 1fr) minmax(340px, 1fr);
}

.write-page__body--editor-only {
  grid-template-columns: 200px minmax(0, 1fr);
}

.write-page__body--editor-only.write-page__body--hide-toc,
.write-page__body--preview-only.write-page__body--hide-toc {
  grid-template-columns: minmax(0, 1fr);
}

.write-page__body--preview-only {
  grid-template-columns: 200px minmax(0, 1fr);
}

.write-page__pane {
  min-height: 0;
  overflow: auto;
}

.write-page__toc {
  border-right: 1px solid rgba(217, 226, 236, 0.92);
  background:
    linear-gradient(180deg, rgba(245, 248, 252, 0.96) 0%, rgba(241, 245, 249, 0.86) 100%);
}

:deep(.write-page__toc-header) {
  padding: 18px 16px 10px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #475569;
}

:deep(.write-page__toc-list) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 10px 14px;
}

:deep(.write-page__toc-item) {
  border: 1px solid transparent;
  border-radius: 10px;
  padding: 8px 10px;
  text-align: left;
  color: #334155;
  background: rgba(255, 255, 255, 0.62);
  cursor: pointer;
  font-size: 13px;
  line-height: 1.35;
  transition:
    background 0.18s ease,
    color 0.18s ease,
    border-color 0.18s ease,
    transform 0.18s ease;
}

:deep(.write-page__toc-item:hover) {
  transform: translateX(1px);
  background: rgba(15, 118, 110, 0.1);
  border-color: rgba(15, 118, 110, 0.18);
  color: #0f766e;
}

:deep(.write-page__toc-item--level-2) {
  padding-left: 24px;
  color: #475569;
}

:deep(.write-page__toc-item--level-3) {
  padding-left: 36px;
  color: #64748b;
}

:deep(.write-page__toc-item--level-4) {
  padding-left: 48px;
  color: #94a3b8;
}

.write-page__pane--editor {
  border-right: 1px solid rgba(217, 226, 236, 0.92);
}

.write-page__body--editor-only .write-page__pane--preview,
.write-page__body--preview-only .write-page__pane--editor {
  display: none;
}

.write-page__textarea {
  width: 100%;
  height: 100%;
  min-height: calc(100vh - 154px);
  border: 0;
  resize: none;
  padding: 24px 32px 40px;
  font: inherit;
  font-size: 17px;
  line-height: 1.9;
  color: #102031;
  background: transparent;
  outline: none;
}

:deep(.write-page__preview-shell) {
  min-height: 100%;
  padding: 32px 36px 48px;
}

.write-page__theme-indicator {
  display: inline-flex;
  align-items: center;
  margin-bottom: 14px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  color: #0f766e;
  background: rgba(15, 118, 110, 0.08);
}

.write-page__markdown {
  max-width: none;
  min-height: 100%;
  box-sizing: border-box;
}

:deep(.write-page__preview-empty) {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #6b7f95;
}

:deep(.write-page__preview-empty p) {
  margin: 0;
  font-size: 18px;
  color: #334155;
}

.write-page__statusbar {
  justify-content: flex-start;
  gap: 18px;
  color: #6b7f95;
  font-size: 13px;
  background: rgba(255, 251, 245, 0.84);
}

.publish-modal-shell {
  width: min(760px, calc(100vw - 96px));
  margin: 0 auto;
}

.publish-modal-card {
  border-radius: 20px;
}

.publish-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.publish-form__row {
  display: grid;
  grid-template-columns: 84px minmax(0, 1fr);
  align-items: center;
  gap: 18px;
}

.publish-form__row--top {
  align-items: flex-start;
}

.publish-form__label {
  font-size: 14px;
  color: #334155;
}

.publish-form__control {
  width: 100%;
}

.publish-cover {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.publish-cover__file-input {
  display: none;
}

.publish-cover__picker {
  width: 194px;
  height: 130px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 1px dashed rgba(148, 163, 184, 0.7);
  border-radius: 16px;
  color: #64748b;
  background: rgba(248, 250, 252, 0.8);
  cursor: pointer;
  overflow: hidden;
}

.publish-cover__picker--busy {
  opacity: 0.72;
  cursor: wait;
}

.publish-cover__preview-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.publish-cover__plus {
  font-size: 28px;
  line-height: 1;
}

.publish-cover__actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.publish-cover__stage {
  margin: 0;
  font-size: 12px;
  color: #0f766e;
}

.publish-summary {
  position: relative;
}

.publish-summary__count {
  position: absolute;
  right: 12px;
  bottom: 10px;
  font-size: 12px;
  color: #f97316;
}

.publish-form__switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid rgba(217, 226, 236, 0.82);
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.8);
}

.publish-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@media (max-width: 1120px) {

  .write-page__topbar,
  .write-page__toolbar,
  .write-page__statusbar {
    padding: 12px 16px;
  }

  .write-page__actions {
    gap: 10px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .write-page__toolbar-group--right,
  .write-page__toc,
  .write-page__pane--preview {
    display: none !important;
  }

  .write-page__body,
  .write-page__body--hide-toc,
  .write-page__body--editor-only,
  .write-page__body--preview-only,
  .write-page__body--editor-only.write-page__body--hide-toc,
  .write-page__body--preview-only.write-page__body--hide-toc {
    grid-template-columns: 1fr !important;
  }

  .write-page__pane--editor {
    border-right: 0;
    border-bottom: 0;
  }

  .write-page__textarea {
    min-height: 420px;
    padding: 20px 18px 28px;
  }

  .write-page__preview-shell {
    padding: 24px 18px 32px;
  }
}

@media (max-width: 720px) {
  .write-title-input :deep(.n-input__input-el) {
    font-size: 20px;
  }

  .write-page__actions .muted {
    display: none;
  }

  .publish-form__row {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .publish-modal__footer {
    flex-wrap: wrap;
  }

  .publish-modal-shell {
    width: calc(100vw - 28px) !important;
  }
}
.write-page__statusbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 14px 24px;
  border-top: 1px solid rgba(217, 226, 236, 0.72);
}
</style>
