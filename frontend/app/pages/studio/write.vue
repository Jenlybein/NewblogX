<script setup lang="ts">
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { computed, reactive, ref, watch } from "vue";
import { useDebounce } from "@vueuse/core";
import { useMessage } from "naive-ui";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import StudioAiAssistPanel from "~/components/studio/StudioAiAssistPanel.vue";
import StudioSelectionAiToolbar from "~/components/studio/StudioSelectionAiToolbar.vue";
import StudioWritePreview from "~/components/studio/StudioWritePreview.vue";
import StudioWritePublishDialog from "~/components/studio/StudioWritePublishDialog.vue";
import StudioWriteToc from "~/components/studio/StudioWriteToc.vue";
import StudioWriteHeader from "~/components/studio/StudioWriteHeader.vue";
import { useStudioEditorCommands } from "~/composables/useStudioEditorCommands";
import { useStudioAiAssist } from "~/composables/useStudioAiAssist";
import { useStudioArticle } from "~/composables/useStudioArticle";
import { useStudioMarkdownTheme } from "~/composables/useStudioMarkdownTheme";
import { useStudioPublish } from "~/composables/useStudioPublish";
import { useStudioScore } from "~/composables/useStudioScore";
import { useStudioToolbar } from "~/composables/useStudioToolbar";
import { updateArticle } from "~/services/article";
import type { getArticleScoreDetail } from "~/services/ai";
import { regenerateArticleScore } from "~/services/ai";
import { resolveAvatarInitial } from "~/utils/avatar";
import { DRAFT_DISABLED_NOTICE } from "~/utils/writePolicy";

definePageMeta({
  layout: "write",
  middleware: "auth",
});


const router = useRouter();
const authStore = useAuthStore();
const message = useMessage();
// 站点「发布与展示」开关（ADR-0017 决策 2）。服务端判定是唯一边界，
// 这里只做体验：发布关闭时说明原因、草稿关闭时编辑器只读。
const {
  policy: writePolicy,
  readOnly: writeReadOnly,
  notices: writePolicyNotices,
} = useArticleWritePolicy();
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
  categoryCreating,
  createCategoryInline,
  isLoggedIn,
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

// AI 面板（选区浮层 + 动作执行 + 元信息补全）见 composable。
const {
  aiPanelVisible,
  aiPanelAction,
  aiStyleInstruction,
  aiSelectionContext,
  aiOverwriteResult,
  aiDiagnoseResult,
  aiErrorMessage,
  aiPanelBusy,
  aiCanSubmit,
  handleAiToolOpen,
  handleSelectionToolbarAction,
  handleEditorSelectionChange,
  handleAiPanelRun,
  handleAiReplace,
  handleAiInsertBelow,
  handleAiCopyResult,
  handleAiPanelVisibility,
  handleAiAssist,
  selectionOverlay,
} = useStudioAiAssist({
  content: articleContentModel,
  title: articleTitleModel,
  textareaRef: editorTextareaRef,
  focusEditor,
  form,
  pendingState,
  publishVisible,
  viewMode,
  categoryOptions,
  tagOptions,
  message,
});

// 评分弹窗的数据加载见 composable（三个入口共用一套 loading 状态）。
const { handleOpenArticleScore } = useStudioScore({
  modalOpen: articleScoreModalOpen,
  loading: articleScoreLoading,
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

  // 自动保存草稿这一步也要尊重站点开关：草稿关闭时不能借"重新评分"偷偷保存。
  // 文案复用编辑器顶部那条站点策略说明，避免同一件事两套说法。
  if (writeReadOnly.value) {
    message.warning(DRAFT_DISABLED_NOTICE);
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
  // 站点策略：草稿/发布开关关闭时在这里拦一道，避免"点了才知道不行"。
  policy: writePolicy,
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
      :draft-disabled="!writePolicy.allowDraft"
      :publish-disabled="!writePolicy.allowPublish"
      :title-readonly="writeReadOnly"
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

    <!-- 站点策略说明：发布/草稿开关关闭时，先说清"为什么不能写"。
         放在顶栏之下、正文之上，直接访问编辑器（入口已隐藏）时才看得到。 -->
    <div v-if="writePolicyNotices.length" class="write-page__policy" role="status">
      <p v-for="notice in writePolicyNotices" :key="notice">{{ notice }}</p>
    </div>

    <main class="write-page__body" :class="writeBodyClass">
      <StudioWriteToc
        :visible="showToc"
        :items="tocItems"
        @jump="handleTocJump" />

      <section class="write-page__pane write-page__pane--editor">
        <textarea
ref="editorTextareaRef" v-model="form.content" class="write-page__textarea"
          :readonly="writeReadOnly"
          :aria-readonly="writeReadOnly"
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
      :draft-disabled="!writePolicy.allowDraft"
      :publish-disabled="!writePolicy.allowPublish"
      :ai-pending="pendingState.ai"
      :cover-uploading="pendingState.cover"
      :cover-preview-url="coverPreviewUrl"
      :cover-stage="coverUploadStage"
      :category-options="categoryOptions || []"
      :tag-options="tagOptions || []"
      :visibility-options="visibilityOptions"
      :can-create-category="isLoggedIn"
      :category-creating="categoryCreating"
      :on-create-category="createCategoryInline"
      :draft-label="draftActionLabel"
      :publish-label="publishActionLabel"
      :on-cover-change="handleCoverFileChange"
      :on-clear="clearCoverSelection"
      @submit="submitArticle"
      @ai-fill="handleAiAssist" />
  </div>
</template>

<style scoped src="../../assets/css/studio-write.css"></style>
