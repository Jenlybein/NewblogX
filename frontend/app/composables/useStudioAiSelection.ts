import type { Ref } from "vue";
import type { AiDiagnoseResponseData, AiOverwriteMode } from "~/types/api";
import { computed, reactive, shallowRef } from "vue";
import { streamAiDiagnose, streamAiOverwrite } from "~/services/ai";

export type StudioAiAction = AiOverwriteMode | "diagnose";

export interface StudioAiSelectionContext {
  articleTitle: string;
  selectionText: string;
  prefixText: string;
  suffixText: string;
  selectionStart: number;
  selectionEnd: number;
  sourceContent: string;
}

interface UseStudioAiSelectionOptions {
  content: Ref<string>;
  title: Ref<string>;
  textareaRef: Ref<HTMLTextAreaElement | null>;
  focusEditor: (selectionStart?: number, selectionEnd?: number) => void;
}

const MAX_CONTEXT_LENGTH = 800;
const MIN_SELECTION_LENGTH = 25;
const MAX_SELECTION_LENGTH = 3500;

function resolveActionMode(action: StudioAiAction): AiOverwriteMode | null {
  if (action === "diagnose") {
    return null;
  }
  return action;
}

export function useStudioAiSelection(options: UseStudioAiSelectionOptions) {
  const panelVisible = shallowRef(false);
  const currentAction = shallowRef<StudioAiAction>("polish");
  const styleInstruction = shallowRef("");
  const selectionContext = shallowRef<StudioAiSelectionContext | null>(null);
  const overwriteResult = shallowRef("");
  const diagnoseResult = shallowRef<AiDiagnoseResponseData | null>(null);
  const errorMessage = shallowRef("");
  const pending = reactive({
    overwrite: false,
    diagnose: false,
  });

  let abortController: AbortController | null = null;

  const isDiagnoseAction = computed(() => currentAction.value === "diagnose");
  const isStyleTransformAction = computed(() => currentAction.value === "style_transform");
  const isBusy = computed(() => pending.overwrite || pending.diagnose);
  const hasOverwriteResult = computed(() => Boolean(overwriteResult.value.trim()));
  const hasDiagnoseResult = computed(() => Boolean(diagnoseResult.value));
  const canSubmit = computed(() => {
    if (!selectionContext.value || isBusy.value) {
      return false;
    }

    if (isStyleTransformAction.value) {
      return Boolean(styleInstruction.value.trim());
    }

    return true;
  });

  function resetResults() {
    overwriteResult.value = "";
    diagnoseResult.value = null;
    errorMessage.value = "";
  }

  function abortCurrentRequest() {
    abortController?.abort();
    abortController = null;
    pending.overwrite = false;
    pending.diagnose = false;
  }

  function closePanel() {
    abortCurrentRequest();
    panelVisible.value = false;
    selectionContext.value = null;
    resetResults();
    styleInstruction.value = "";
  }

  function captureSelectionContext() {
    const textarea = options.textareaRef.value;
    if (!textarea) {
      throw new Error("编辑器尚未就绪，请稍后重试。");
    }

    const articleTitle = options.title.value.trim();
    if (!articleTitle) {
      throw new Error("请先填写文章标题，再使用 AI 辅助。");
    }

    const content = options.content.value;
    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const selectionText = content.slice(selectionStart, selectionEnd);
    const trimmedLength = selectionText.trim().length;

    if (!trimmedLength) {
      throw new Error("请先在编辑区选中一段正文。");
    }

    if (trimmedLength < MIN_SELECTION_LENGTH) {
      throw new Error(`选中内容至少需要 ${MIN_SELECTION_LENGTH} 个字后再交给 AI 处理。`);
    }

    if (trimmedLength > MAX_SELECTION_LENGTH) {
      throw new Error(`选中内容请控制在 ${MAX_SELECTION_LENGTH} 字以内，避免生成不稳定。`);
    }

    return {
      articleTitle,
      selectionText,
      prefixText: content.slice(Math.max(0, selectionStart - MAX_CONTEXT_LENGTH), selectionStart),
      suffixText: content.slice(selectionEnd, selectionEnd + MAX_CONTEXT_LENGTH),
      selectionStart,
      selectionEnd,
      sourceContent: content,
    } satisfies StudioAiSelectionContext;
  }

  async function runCurrentAction() {
    const context = selectionContext.value;
    if (!context) {
      throw new Error("当前没有可用的选区。");
    }

    abortCurrentRequest();
    resetResults();

    const nextAbortController = new AbortController();
    abortController = nextAbortController;

    try {
      if (isDiagnoseAction.value) {
        pending.diagnose = true;
        diagnoseResult.value = await streamAiDiagnose(
          {
            selection_text: context.selectionText,
            prefix_text: context.prefixText,
            suffix_text: context.suffixText,
            article_title: context.articleTitle,
          },
          {
            signal: nextAbortController.signal,
            onResult(result) {
              diagnoseResult.value = result;
            },
          },
        );
        return;
      }

      const mode = resolveActionMode(currentAction.value);
      if (!mode) {
        throw new Error("当前 AI 模式无效。");
      }

      if (mode === "style_transform" && !styleInstruction.value.trim()) {
        throw new Error("请输入目标风格后再开始改写。");
      }

      pending.overwrite = true;
      overwriteResult.value = await streamAiOverwrite(
        {
          mode,
          selection_text: context.selectionText,
          prefix_text: context.prefixText,
          suffix_text: context.suffixText,
          article_title: context.articleTitle,
          target_style: mode === "style_transform" ? styleInstruction.value.trim() : undefined,
        },
        {
          signal: nextAbortController.signal,
          onChunk(chunk) {
            overwriteResult.value += chunk;
          },
        },
      );
    } catch (error) {
      if (nextAbortController.signal.aborted) {
        return;
      }

      errorMessage.value = error instanceof Error ? error.message : "AI 辅助执行失败";
      throw error;
    } finally {
      if (abortController === nextAbortController) {
        abortController = null;
      }
      pending.overwrite = false;
      pending.diagnose = false;
    }
  }

  async function openPanel(action: StudioAiAction) {
    selectionContext.value = captureSelectionContext();
    currentAction.value = action;
    panelVisible.value = true;
    resetResults();

    if (action !== "style_transform") {
      await runCurrentAction();
    }
  }

  function applyOverwriteResult() {
    const context = selectionContext.value;
    const nextContent = overwriteResult.value;
    if (!context || !nextContent.trim()) {
      throw new Error("当前没有可替换的改写结果。");
    }

    if (options.content.value !== context.sourceContent) {
      throw new Error("正文已发生变化，请重新生成后再替换。");
    }

    options.content.value =
      context.sourceContent.slice(0, context.selectionStart) + nextContent + context.sourceContent.slice(context.selectionEnd);
    panelVisible.value = false;
    options.focusEditor(context.selectionStart, context.selectionStart + nextContent.length);
    closePanel();
  }

  function insertOverwriteBelow() {
    const context = selectionContext.value;
    const nextContent = overwriteResult.value;
    if (!context || !nextContent.trim()) {
      throw new Error("当前没有可插入的改写结果。");
    }

    if (options.content.value !== context.sourceContent) {
      throw new Error("正文已发生变化，请重新生成后再插入。");
    }

    const before = context.sourceContent.slice(0, context.selectionEnd);
    const after = context.sourceContent.slice(context.selectionEnd);
    const prefixGap = before.endsWith("\n") ? "\n" : "\n\n";
    const suffixGap = after.startsWith("\n") ? "" : "\n";
    const insertion = `${prefixGap}${nextContent}${suffixGap}`;
    const cursorStart = context.selectionEnd + prefixGap.length;

    options.content.value = `${before}${insertion}${after}`;
    panelVisible.value = false;
    options.focusEditor(cursorStart, cursorStart + nextContent.length);
    closePanel();
  }

  async function copyOverwriteResult() {
    if (!overwriteResult.value.trim()) {
      throw new Error("当前没有可复制的改写结果。");
    }

    if (!import.meta.client || !navigator.clipboard) {
      throw new Error("当前环境不支持剪贴板复制。");
    }

    await navigator.clipboard.writeText(overwriteResult.value);
  }

  return {
    panelVisible,
    currentAction,
    styleInstruction,
    selectionContext,
    overwriteResult,
    diagnoseResult,
    errorMessage,
    pending,
    isBusy,
    isDiagnoseAction,
    isStyleTransformAction,
    hasOverwriteResult,
    hasDiagnoseResult,
    canSubmit,
    openPanel,
    closePanel,
    runCurrentAction,
    applyOverwriteResult,
    insertOverwriteBelow,
    copyOverwriteResult,
  };
}
