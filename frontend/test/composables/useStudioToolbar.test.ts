import { ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { LEFT_TOOLS } from "~/constants/studioToolbar";
import { useStudioToolbar } from "~/composables/useStudioToolbar";

// 工具栏分发：按钮 key → 做什么。
//
// 这段逻辑以前埋在页面里，改一个按钮的插入内容没有任何测试兜底 ——
// 而它产出的是**直接写进正文的文本**（代码块、表格、mermaid 图），
// 写错了用户看到的就是坏掉的 Markdown。
//
// 这里钉：插入内容正确、已退场的"预览样式"下拉不再保留任何状态通道、
// 未知 key 不乱动正文。

function harness() {
  const insertSnippet = vi.fn();
  const applyBold = vi.fn();

  const toolbar = useStudioToolbar({
    editor: {
      insertSnippet,
      focusEditor: vi.fn(),
      applyBoldShortcut: applyBold,
      applyItalicShortcut: vi.fn(),
      applyQuoteShortcut: vi.fn(),
      applyLinkShortcut: vi.fn(),
      applyImageShortcut: vi.fn(),
      applyInlineCodeShortcut: vi.fn(),
      applyCodeBlockShortcut: vi.fn(),
      applyUnorderedListShortcut: vi.fn(),
      applyOrderedListShortcut: vi.fn(),
      applyHeadingShortcut: vi.fn(),
      applyTableShortcut: vi.fn(),
      applyStrikeShortcut: vi.fn(),
    } as never,
    showToc: ref(true),
    viewMode: ref("split" as const),
    previewRef: ref(null),
    previewHeadings: ref([]),
    openAi: vi.fn(),
  });

  return { toolbar, insertSnippet, applyBold };
}

describe("useStudioToolbar", () => {
  it("工具栏没有预览主题下拉（13 套预览样式已退场）", () => {
    expect(LEFT_TOOLS.map((tool) => tool.key)).not.toContain("theme");
    expect(LEFT_TOOLS.map((tool) => tool.label)).not.toContain("预览样式");
  });

  it("工具栏不再暴露选项选中态（只服务主题下拉的通道已退场）", () => {
    const { toolbar } = harness();

    expect((toolbar as Record<string, unknown>).isToolOptionActive).toBeUndefined();
  });

  it("已退场的 theme 下拉 key 不碰正文，也不抛错", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("theme", "shanyue");

    expect(insertSnippet).not.toHaveBeenCalled();
  });

  it("标题级别按 option key 生成对应数量的 #", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("title", "h3");

    expect(insertSnippet).toHaveBeenCalledWith("### 标题\n");
  });

  it("标题级别非法时回落到二级（不产出 ####### 这种坏 Markdown）", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("title", "hx");

    expect(insertSnippet).toHaveBeenCalledWith("## 标题\n");
  });

  it("mermaid 插入的是带语言标记的围栏块", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("mermaid", "flowchart");

    const snippet = insertSnippet.mock.calls[0][0] as string;
    expect(snippet.startsWith("```mermaid\n")).toBe(true);
    expect(snippet.endsWith("```\n")).toBe(true);
    expect(snippet).toContain("flowchart TD");
  });

  it("未知 mermaid 类型回落到 flowchart（而不是插入空块）", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("mermaid", "no-such-diagram");

    const snippet = insertSnippet.mock.calls[0][0] as string;
    expect(snippet).toContain("flowchart TD");
  });

  it("对齐选项生成 div 包裹", () => {
    const { toolbar, insertSnippet } = harness();

    toolbar.handleToolOption("align", "align-center");

    expect(insertSnippet).toHaveBeenCalledWith('<div align="center">\n内容\n</div>\n');
  });

  it("handleToolClick 转发到对应的编辑器命令", () => {
    const { toolbar, applyBold } = harness();

    toolbar.handleToolClick({ key: "bold", label: "加粗" } as never);

    expect(applyBold).toHaveBeenCalledTimes(1);
  });
});
