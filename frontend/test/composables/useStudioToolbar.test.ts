import { ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useStudioToolbar } from "~/composables/useStudioToolbar";

// 工具栏分发：按钮 key → 做什么。
//
// 这段逻辑以前埋在页面里，改一个按钮的插入内容没有任何测试兜底 ——
// 而它产出的是**直接写进正文的文本**（代码块、表格、mermaid 图），
// 写错了用户看到的就是坏掉的 Markdown。
//
// 这里钉三件事：插入内容正确、主题只改主题、未知 key 不乱动正文。

function harness() {
  const insertSnippet = vi.fn();
  const applyBold = vi.fn();
  const setTheme = ref("github");

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
    theme: {
      selectedMarkdownTheme: setTheme,
      markdownThemeOptions: [
        { key: "github", label: "GitHub Light" },
        { key: "shanyue", label: "Shanyue" },
      ],
    } as never,
    showToc: ref(true),
    viewMode: ref("split" as const),
    previewRef: ref(null),
    previewHeadings: ref([]),
    openAi: vi.fn(),
  });

  return { toolbar, insertSnippet, applyBold, setTheme };
}

describe("useStudioToolbar", () => {
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

  it("主题只改主题，不碰正文", () => {
    const { toolbar, insertSnippet, setTheme } = harness();

    toolbar.handleToolOption("theme", "shanyue");

    expect(setTheme.value).toBe("shanyue");
    expect(insertSnippet).not.toHaveBeenCalled();
  });

  it("未知主题回落到 github", () => {
    const { toolbar, setTheme } = harness();

    toolbar.handleToolOption("theme", "no-such-theme");

    expect(setTheme.value).toBe("github");
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

  it("isToolOptionActive 只对主题返回 true", () => {
    const { toolbar } = harness();

    expect(toolbar.isToolOptionActive("theme", "github")).toBe(true);
    expect(toolbar.isToolOptionActive("theme", "shanyue")).toBe(false);
    expect(toolbar.isToolOptionActive("title", "h1")).toBe(false);
  });

  it("handleToolClick 转发到对应的编辑器命令", () => {
    const { toolbar, applyBold } = harness();

    toolbar.handleToolClick({ key: "bold", label: "加粗" } as never);

    expect(applyBold).toHaveBeenCalledTimes(1);
  });
});
