import type { Ref } from "vue";

import type { ToolItem } from "~/constants/studioToolbar";
import type { StudioAiAction } from "~/composables/useStudioAiSelection";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import type { useStudioEditorCommands } from "~/composables/useStudioEditorCommands";
import type { useStudioMarkdownTheme } from "~/composables/useStudioMarkdownTheme";

// 工具栏的点击分发。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。它是一张"按钮 key → 做什么"的分发表：
// 二十来个按钮，每个要么插一段 Markdown 片段、要么改一个视图状态、要么打开 AI 面板。
//
// 抽出来的理由：这段代码不改任何业务数据，只是"把点击翻译成命令"。
// 留在页面里时，它和 hydrate、提交、封面混在一起，很难看出"改正文"只有两条来源
// （编辑器命令 + 这里的片段插入）。
//
// 依赖全部注入：编辑器命令对象、主题、视图状态都由页面持有。

export type StudioToolbarDeps = {
  /** 编辑器命令（插入片段是这里唯一用到的能力）。 */
  editor: ReturnType<typeof useStudioEditorCommands>;
  /** 主题状态与选项（"预览样式"下拉要用）。 */
  theme: ReturnType<typeof useStudioMarkdownTheme>;
  /** 目录栏开关与视图模式。 */
  showToc: Ref<boolean>;
  viewMode: Ref<"split" | "editor" | "preview">;
  /** 预览区引用（点目录要滚动）。 */
  previewRef: Ref<{ scrollToHeading: (id: string) => boolean } | null>;
  /** 预览区上报的锚点。 */
  previewHeadings: Ref<ArticleHeadingAnchor[]>;
  /** 打开 AI 面板。 */
  openAi: (action: StudioAiAction) => void;
};

export function useStudioToolbar(deps: StudioToolbarDeps) {
  const {
    editor,
    theme,
    showToc,
    viewMode,
    previewRef: shadowPreviewRef,
    previewHeadings,
    openAi: handleAiToolOpen,
  } = deps;

  // 只解构本模块真正用到的命令：多解构会让"这个模块能做什么"变得模糊。
  const {
    insertSnippet,
    focusEditor,
    applyBoldShortcut,
    applyItalicShortcut,
    applyQuoteShortcut,
    applyLinkShortcut,
    applyImageShortcut,
    applyInlineCodeShortcut,
    applyCodeBlockShortcut,
    applyUnorderedListShortcut,
    applyOrderedListShortcut,
    applyTableShortcut,
    applyStrikeShortcut,
  } = editor;
  const { selectedMarkdownTheme, markdownThemeOptions } = theme;

  function handleToolOption(toolKey: string, optionKey: string) {
    if (toolKey === "ai") {
      void handleAiToolOpen(optionKey as StudioAiAction);
      return;
    }

    if (toolKey === "theme") {
      selectedMarkdownTheme.value =
        (markdownThemeOptions.find((option) => option.key === optionKey)?.key as typeof selectedMarkdownTheme.value) ||
        "github";
      return;
    }

    if (toolKey === "title") {
      const level = Number(optionKey.slice(1)) || 2;
      insertSnippet(`${"#".repeat(level)} 标题\n`);
      return;
    }

    if (toolKey === "align") {
      const alignMap = {
        "align-left": "<div align=\"left\">\n内容\n</div>\n",
        "align-center": "<div align=\"center\">\n内容\n</div>\n",
        "align-right": "<div align=\"right\">\n内容\n</div>\n",
      } as const;

      insertSnippet(alignMap[optionKey as keyof typeof alignMap] || "<div align=\"left\">\n内容\n</div>\n");
      return;
    }

    if (toolKey === "mermaid") {
      const mermaidMap = {
        flowchart: "flowchart TD\n  A[开始] --> B[继续创作]",
        sequence: "sequenceDiagram\n  participant U as User\n  participant A as API\n  U->>A: 请求数据\n  A-->>U: 返回结果",
        class: "classDiagram\n  class Article {\n    +string title\n    +string content\n  }",
        state: "stateDiagram-v2\n  [*] --> Draft\n  Draft --> Published",
        er: "erDiagram\n  USER ||--o{ ARTICLE : writes",
        journey: "journey\n  title 创作流程\n  section 写作\n    起草: 5: 作者\n    发布: 4: 作者",
        gantt: "gantt\n  title 发布计划\n  dateFormat  YYYY-MM-DD\n  section Draft\n  写初稿 :2026-04-14, 2d",
        pie: "pie title 标签占比\n  \"Nuxt\" : 40\n  \"OpenAPI\" : 30\n  \"TypeScript\" : 30",
      } as const;

      insertSnippet(`\`\`\`mermaid\n${mermaidMap[optionKey as keyof typeof mermaidMap] || mermaidMap.flowchart}\n\`\`\`\n`);
    }
  }

  function isToolOptionActive(toolKey: string, optionKey: string) {
    if (toolKey === "theme") {
      return selectedMarkdownTheme.value === optionKey;
    }
    return false;
  }

  function handleToolClick(tool: ToolItem) {
    switch (tool.key) {
      case "bold":
        applyBoldShortcut();
        break;
      case "italic":
        applyItalicShortcut();
        break;
      case "quote":
        applyQuoteShortcut();
        break;
      case "link":
        applyLinkShortcut();
        break;
      case "image":
        applyImageShortcut();
        break;
      case "highlight":
        applyInlineCodeShortcut();
        break;
      case "code":
        applyCodeBlockShortcut();
        break;
      case "ul":
        applyUnorderedListShortcut();
        break;
      case "ol":
        applyOrderedListShortcut();
        break;
      case "strike":
        applyStrikeShortcut();
        break;
      case "table":
        applyTableShortcut();
        break;
      default:
        break;
    }
  }

  function handleRightToolClick(key: "toc" | "editor" | "preview") {
    if (key === "toc") {
      showToc.value = !showToc.value;
      return;
    }

    const nextMode = key === "editor" ? "editor" : "preview";
    viewMode.value = viewMode.value === nextMode ? "split" : nextMode;
  }

  function handleTocJump(id: string) {
    if (!id) {
      focusEditor();
      return;
    }

    shadowPreviewRef.value?.scrollToHeading(id);
  }


  function handlePreviewHeadingsChange(nextHeadings: ArticleHeadingAnchor[]) {
    previewHeadings.value = nextHeadings;
  }

  return {
    handleToolOption,
    isToolOptionActive,
    handleToolClick,
    handleRightToolClick,
    handleTocJump,
    handlePreviewHeadingsChange,
  };
}
