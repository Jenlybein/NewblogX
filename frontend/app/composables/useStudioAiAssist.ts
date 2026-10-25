import type { Ref } from "vue";

import { useStudioAiSelection } from "~/composables/useStudioAiSelection";
import { useTextareaSelectionOverlay } from "~/composables/useTextareaSelectionOverlay";
import type { StudioAiAction } from "~/composables/useStudioAiSelection";
import { generateArticleMetainfo } from "~/services/ai";

// 写作页的 AI 面板：选区浮层、动作执行、改写结果落回正文、元信息补全。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。这一块原本就建立在两个
// 现成的 composable 上（useStudioAiSelection / useTextareaSelectionOverlay），
// 页面只是把它们和"表单"接起来。抽出来之后，"页面与 AI 的唯一接触面"
// 就是本模块导出的那几个状态与函数。
//
// 依赖：正文与标题的 v-model、textarea 引用、聚焦函数、表单（补元信息要写回去）。

/** Notifier 是本模块需要的提示能力。 */
export type Notifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  info: (msg: string) => void;
  error: (msg: string) => void;
};

/** 表单里 AI 补元信息会写到的字段。 */
export type AiMetainfoTarget = {
  title: string;
  abstract: string;
  content: string;
  category_id: string | null;
  tag_ids: string[];
};

export type StudioAiDeps = {
  content: Ref<string>;
  title: Ref<string>;
  textareaRef: Ref<HTMLTextAreaElement | null>;
  focusEditor: (start?: number, end?: number) => void;
  /** 表单（AI 补元信息要写回标题/摘要/分类/标签）。 */
  form: AiMetainfoTarget;
  /** 进行中标记（AI 补元信息用 ai 这一位）。 */
  pendingState: { ai: boolean };
  /** 发布弹窗开关与视图模式（决定选区浮层要不要隐藏）。 */
  publishVisible: Ref<boolean>;
  viewMode: Ref<"split" | "editor" | "preview">;
  /** 分类/标签选项（AI 返回的是名字，要映射成 id）。 */
  categoryOptions: Ref<{ label: string; value: string }[] | undefined>;
  tagOptions: Ref<{ label: string; value: string }[] | undefined>;
  message: Notifier;
};

/**
 * useStudioAiAssist 提供 AI 面板与选区浮层。
 *
 * 返回值的命名与页面模板一一对应（aiPanelVisible / aiPanelRun ...），
 * 页面不需要再包一层。
 */
export function useStudioAiAssist(deps: StudioAiDeps) {
  const {
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
  } = deps;


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


  return {
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
    updateSelectionOverlay,
    hideSelectionOverlay,
  };
}
