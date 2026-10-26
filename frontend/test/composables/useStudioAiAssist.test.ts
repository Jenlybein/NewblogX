import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, reactive, ref } from "vue";

import { useStudioAiAssist } from "~/composables/useStudioAiAssist";

// useStudioAiAssist 的用例。
//
// 这个模块本身不含 AI 逻辑（那是 useStudioAiSelection 的事），它是**编排层**：
// 把选区浮层、AI 面板、表单三者接起来。所以这里钉的是"接线"和"兜底"，
// 尤其是两条容易漏的：
//
//   1. **失败要出声**。8 个 handle* 函数每一个都有 try/catch，但凡有一处忘了
//      message.error，用户点了按钮会看到"什么都没发生"。逐条钉住。
//   2. **AI 返回的分类/标签是"名字"，要映射成 id**。名字对不上就只能丢弃，
//      绝不能把"后端"这个字符串当 id 写进表单——那会存出一堆脏数据。
//      这里把"按 id 命中 / 按名字命中 / 命中不了"三条路都测到。

const useStudioAiSelectionMock = vi.hoisted(() => vi.fn());
const useTextareaSelectionOverlayMock = vi.hoisted(() => vi.fn());
const generateArticleMetainfoMock = vi.hoisted(() => vi.fn());

vi.mock("~/composables/useStudioAiSelection", () => ({ useStudioAiSelection: useStudioAiSelectionMock }));
vi.mock("~/composables/useTextareaSelectionOverlay", () => ({
  useTextareaSelectionOverlay: useTextareaSelectionOverlayMock,
}));
vi.mock("~/services/ai", () => ({ generateArticleMetainfo: generateArticleMetainfoMock }));

type SelectionApi = {
  panelVisible: ReturnType<typeof ref<boolean>>;
  currentAction: ReturnType<typeof ref<string | null>>;
  styleInstruction: ReturnType<typeof ref<string>>;
  selectionContext: ReturnType<typeof ref<unknown>>;
  overwriteResult: ReturnType<typeof ref<unknown>>;
  diagnoseResult: ReturnType<typeof ref<unknown>>;
  errorMessage: ReturnType<typeof ref<string>>;
  isBusy: ReturnType<typeof ref<boolean>>;
  canSubmit: ReturnType<typeof ref<boolean>>;
  openPanel: ReturnType<typeof vi.fn>;
  closePanel: ReturnType<typeof vi.fn>;
  runCurrentAction: ReturnType<typeof vi.fn>;
  applyOverwriteResult: ReturnType<typeof vi.fn>;
  insertOverwriteBelow: ReturnType<typeof vi.fn>;
  copyOverwriteResult: ReturnType<typeof vi.fn>;
};

/** 造一套可控的 useStudioAiSelection 返回值。 */
function makeSelectionApi(): SelectionApi {
  return {
    panelVisible: ref(false) as ReturnType<typeof ref<boolean>>,
    currentAction: ref(null) as ReturnType<typeof ref<string | null>>,
    styleInstruction: ref("") as ReturnType<typeof ref<string>>,
    selectionContext: ref(null),
    overwriteResult: ref(null),
    diagnoseResult: ref(null),
    errorMessage: ref("") as ReturnType<typeof ref<string>>,
    isBusy: ref(false) as ReturnType<typeof ref<boolean>>,
    canSubmit: ref(false) as ReturnType<typeof ref<boolean>>,
    openPanel: vi.fn(async () => undefined),
    closePanel: vi.fn(),
    runCurrentAction: vi.fn(async () => undefined),
    applyOverwriteResult: vi.fn(),
    insertOverwriteBelow: vi.fn(),
    copyOverwriteResult: vi.fn(async () => undefined),
  };
}

function makeMessage() {
  return { success: vi.fn(), warning: vi.fn(), info: vi.fn(), error: vi.fn() };
}

type HarnessOptions = {
  content?: string;
  title?: string;
  publishVisible?: boolean;
  viewMode?: "split" | "editor" | "preview";
  categoryOptions?: { label: string; value: string }[];
  tagOptions?: { label: string; value: string }[];
};

function harness(options: HarnessOptions = {}) {
  const selection = makeSelectionApi();
  const overlay = {
    selectionOverlay: ref(null),
    updateSelectionOverlay: vi.fn(),
    hideSelectionOverlay: vi.fn(),
  };

  useStudioAiSelectionMock.mockReturnValue(selection);
  useTextareaSelectionOverlayMock.mockReturnValue(overlay);

  const message = makeMessage();
  // form 必须是 reactive：本模块靠 watch(() => form.content) 重算浮层位置，
  // 普通对象的属性变化不会被追踪（真实页面里 form 来自 useStudioArticle 的 reactive）。
  const form = reactive({
    title: options.title ?? "",
    abstract: "",
    content: options.content ?? "",
    category_id: null as string | null,
    tag_ids: [] as string[],
  });
  const pendingState = { ai: false };
  const publishVisible = ref(options.publishVisible ?? false);
  const viewMode = ref(options.viewMode ?? "split");
  const categoryOptions = ref(options.categoryOptions ?? [{ label: "后端", value: "cat-1" }]);
  const tagOptions = ref(options.tagOptions ?? [
    { label: "Go", value: "tag-1" },
    { label: "Vue", value: "tag-2" },
  ]);

  const api = useStudioAiAssist({
    content: ref(form.content) as never,
    title: ref(form.title) as never,
    textareaRef: ref(null),
    focusEditor: vi.fn(),
    form,
    pendingState,
    publishVisible,
    viewMode,
    categoryOptions,
    tagOptions,
    message,
  });

  return { api, form, message, pendingState, selection, overlay, publishVisible, viewMode };
}

beforeEach(() => {
  useStudioAiSelectionMock.mockReset();
  useTextareaSelectionOverlayMock.mockReset();
  generateArticleMetainfoMock.mockReset();
  generateArticleMetainfoMock.mockResolvedValue({ title: "", abstract: "" });
});

describe("useStudioAiAssist · 浮层与面板的互斥", () => {
  it("AI 面板打开时隐藏选区浮层（否则两个浮层叠在一起）", async () => {
    const { overlay, selection } = harness();

    selection.panelVisible.value = true;
    await nextTick();

    expect(overlay.hideSelectionOverlay).toHaveBeenCalled();
  });

  it("发布弹窗打开时隐藏选区浮层", async () => {
    const { overlay, publishVisible } = harness();

    publishVisible.value = true;
    await nextTick();

    expect(overlay.hideSelectionOverlay).toHaveBeenCalled();
  });

  it("切到纯预览模式时隐藏选区浮层（预览区不是 textarea，没有选区）", async () => {
    const { overlay, viewMode } = harness();

    viewMode.value = "preview";
    await nextTick();

    expect(overlay.hideSelectionOverlay).toHaveBeenCalled();
  });

  it("面板关掉后重新计算浮层位置", async () => {
    const { overlay, selection } = harness();
    selection.panelVisible.value = true;
    await nextTick();
    overlay.updateSelectionOverlay.mockClear();

    selection.panelVisible.value = false;
    await nextTick();
    await nextTick();

    expect(overlay.updateSelectionOverlay).toHaveBeenCalled();
  });

  it("正文变化后重算浮层位置", async () => {
    const { form, overlay } = harness({ content: "旧内容" });
    overlay.updateSelectionOverlay.mockClear();

    form.content = "新内容";
    await nextTick();
    await nextTick();

    expect(overlay.updateSelectionOverlay).toHaveBeenCalled();
  });
});

describe("useStudioAiAssist · 动作兜底", () => {
  it("打开面板前先收起选区浮层", async () => {
    const { api, overlay, selection } = harness();

    await api.handleAiToolOpen("polish" as never);

    expect(overlay.hideSelectionOverlay).toHaveBeenCalled();
    expect(selection.openPanel).toHaveBeenCalledWith("polish");
  });

  it("打开面板失败时给出提示，而不是静默", async () => {
    const { api, message, selection } = harness();
    selection.openPanel.mockRejectedValue(new Error("AI 服务未配置"));

    await api.handleAiToolOpen("polish" as never);

    expect(message.warning).toHaveBeenCalledWith("AI 服务未配置");
  });

  it("执行动作失败：带错误文案就用原文案", async () => {
    const { api, message, selection } = harness();
    selection.runCurrentAction.mockRejectedValue(new Error("内容过长"));

    await api.handleAiPanelRun();

    expect(message.error).toHaveBeenCalledWith("内容过长");
  });

  it("执行动作失败：错误没有文案时兜底成通用提示", async () => {
    const { api, message, selection } = harness();
    selection.runCurrentAction.mockRejectedValue(new Error(""));

    await api.handleAiPanelRun();

    expect(message.error).toHaveBeenCalledWith("AI 辅助执行失败");
  });

  it("替换选区成功与失败分别给出对应提示", async () => {
    const success = harness();
    success.api.handleAiReplace();
    expect(success.message.success).toHaveBeenCalledWith("改写结果已替换到正文中");

    const failed = harness();
    failed.selection.applyOverwriteResult.mockImplementation(() => {
      throw new Error("选区已失效");
    });
    failed.api.handleAiReplace();
    expect(failed.message.error).toHaveBeenCalledWith("选区已失效");
  });

  it("插入到选区下方成功与失败分别给出对应提示", async () => {
    const success = harness();
    success.api.handleAiInsertBelow();
    expect(success.message.success).toHaveBeenCalledWith("改写结果已插入到选区下方");

    const failed = harness();
    failed.selection.insertOverwriteBelow.mockImplementation(() => {
      throw new Error("插入位置无效");
    });
    failed.api.handleAiInsertBelow();
    expect(failed.message.error).toHaveBeenCalledWith("插入位置无效");
  });

  it("复制结果失败时提示，成功时不报错", async () => {
    const success = harness();
    await success.api.handleAiCopyResult();
    expect(success.message.success).toHaveBeenCalledWith("改写结果已复制");

    const failed = harness();
    failed.selection.copyOverwriteResult.mockRejectedValue(new Error("剪贴板不可用"));
    await failed.api.handleAiCopyResult();
    expect(failed.message.error).toHaveBeenCalledWith("剪贴板不可用");
  });

  it("面板显隐切换：打开时收浮层，关闭时重算浮层", async () => {
    const { api, overlay, selection } = harness();

    api.handleAiPanelVisibility(true);
    expect(selection.panelVisible.value).toBe(true);
    expect(overlay.hideSelectionOverlay).toHaveBeenCalled();

    overlay.updateSelectionOverlay.mockClear();
    api.handleAiPanelVisibility(false);
    await nextTick();
    await nextTick();

    expect(selection.closePanel).toHaveBeenCalled();
    expect(overlay.updateSelectionOverlay).toHaveBeenCalled();
  });
});

describe("useStudioAiAssist · 元信息补全", () => {
  it("正文为空时直接拦下，不请求 AI", async () => {
    const { api, message } = harness({ content: "   " });

    await api.handleAiAssist();

    expect(message.warning).toHaveBeenCalledWith("先写一点正文，再让 AI 帮你补标题和摘要。");
    expect(generateArticleMetainfoMock).not.toHaveBeenCalled();
  });

  it("AI 返回标题摘要时写回表单", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ title: " AI 标题 ", abstract: "AI 摘要" });
    const { api, form, message, pendingState } = harness({ content: "正文内容" });

    await api.handleAiAssist();

    expect(form.title).toBe("AI 标题");
    expect(form.abstract).toBe("AI 摘要");
    expect(message.success).toHaveBeenCalledWith("AI 已根据正文补全元信息建议");
    expect(pendingState.ai).toBe(false);
  });

  it("AI 返回空标题时保留用户已写的标题（不能被清空）", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ title: "  ", abstract: "" });
    const { api, form } = harness({ content: "正文", title: "我自己写的标题" });

    await api.handleAiAssist();

    expect(form.title).toBe("我自己写的标题");
  });

  it("分类按 id 命中", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ category: { id: "cat-1", title: "不存在的名字" } });
    const { api, form } = harness({ content: "正文" });

    await api.handleAiAssist();

    expect(form.category_id).toBe("cat-1");
  });

  it("分类 id 对不上时按名字命中，且忽略大小写与首尾空白", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ category: { id: "unknown", title: "  后端  " } });
    const { api, form } = harness({ content: "正文" });

    await api.handleAiAssist();

    expect(form.category_id).toBe("cat-1");
  });

  it("分类命中不了时保持原值，绝不把名字当 id 写进去", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ category: { id: "x", title: "前端" } });
    const { api, form } = harness({ content: "正文" });
    form.category_id = "cat-1";

    await api.handleAiAssist();

    expect(form.category_id).toBe("cat-1");
  });

  it("标签按名字映射，并去重", async () => {
    generateArticleMetainfoMock.mockResolvedValue({
      tags: [{ title: "Go" }, { title: "go" }, { title: "Vue" }, { title: "不存在的标签" }],
    });
    const { api, form } = harness({ content: "正文" });

    await api.handleAiAssist();

    expect(form.tag_ids).toEqual(["tag-1", "tag-2"]);
  });

  it("一个标签都命中不了时不改动已有标签", async () => {
    generateArticleMetainfoMock.mockResolvedValue({ tags: [{ title: "Redis" }] });
    const { api, form } = harness({ content: "正文" });
    form.tag_ids = ["tag-1"];

    await api.handleAiAssist();

    expect(form.tag_ids).toEqual(["tag-1"]);
  });

  it("请求失败时提示并复位进行中标记", async () => {
    generateArticleMetainfoMock.mockRejectedValue(new Error("AI 配额已用尽"));
    const { api, message, pendingState } = harness({ content: "正文" });

    await api.handleAiAssist();

    expect(message.error).toHaveBeenCalledWith("AI 配额已用尽");
    expect(pendingState.ai).toBe(false);
  });

  it("请求抛出的不是 Error 时兜底成通用提示", async () => {
    generateArticleMetainfoMock.mockRejectedValue("boom");
    const { api, message, pendingState } = harness({ content: "正文" });

    await api.handleAiAssist();

    expect(message.error).toHaveBeenCalledWith("AI 元信息生成失败");
    expect(pendingState.ai).toBe(false);
  });
});
