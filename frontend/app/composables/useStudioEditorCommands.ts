import { nextTick, type Ref } from "vue";

// 写作页的编辑器命令：选区替换、插入片段、Markdown 快捷键。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。抽的理由不是"文件太长"，
// 而是这些函数**只依赖两样东西**（正文内容 + textarea 引用）：
// 它们和文章元信息、发布流程、AI 面板毫无关系，混在一个 2000 行的文件里
// 才需要靠"读整页"才能确认这一点。
//
// 纯函数的部分（findWordRangeAtCursor / getShortcutRange）可以直接单测，
// 这也是抽出来的直接收益 —— 快捷键映射错了以前只能靠手点。

// StudioEditorText 是本 composable 对表单的最小要求。
//
// 只声明 content 而不是整个表单类型：这个模块不该知道"文章有没有封面"。
export type StudioEditorText = {
  content: string;
};

// StudioEditorCommandsOptions 是依赖注入。
export type StudioEditorCommandsOptions = {
  /** 正文所在的表单对象（原地修改 content）。 */
  form: StudioEditorText;
  /** 编辑区 textarea 的引用。 */
  textareaRef: Ref<HTMLTextAreaElement | null>;
};

/**
 * useStudioEditorCommands 提供一组"在光标处改正文"的命令。
 *
 * 所有命令都遵循同一套约定：改完正文后把光标放到**合理的位置**
 * （插在选区外而不是原地），否则连续按两次快捷键会得到嵌套的标记。
 */
export function useStudioEditorCommands(opts: StudioEditorCommandsOptions) {
  const { form, textareaRef: editorTextareaRef } = opts;

  function focusEditor(selectionStart?: number, selectionEnd?: number) {
    nextTick(() => {
      const textarea = editorTextareaRef.value;
      if (!textarea) {
        return;
      }
      textarea.focus();
      if (typeof selectionStart === "number" && typeof selectionEnd === "number") {
        textarea.setSelectionRange(selectionStart, selectionEnd);
      }
    });
  }

  function replaceSelection(transform: (selectedText: string) => { content: string; start: number; end: number }) {
    const textarea = editorTextareaRef.value;
    const fallbackSelection = form.content.length;
    const selectionStart = textarea?.selectionStart ?? fallbackSelection;
    const selectionEnd = textarea?.selectionEnd ?? fallbackSelection;
    const selectedText = form.content.slice(selectionStart, selectionEnd);
    const nextSelection = transform(selectedText);

    form.content = `${form.content.slice(0, selectionStart)}${nextSelection.content}${form.content.slice(selectionEnd)}`;
    focusEditor(selectionStart + nextSelection.start, selectionStart + nextSelection.end);
  }

  function insertSnippet(snippet: string) {
    replaceSelection((selectedText) => {
      const content = selectedText ? `${snippet}${selectedText}` : snippet;
      const offset = content.length;
      return { content, start: offset, end: offset };
    });
  }

  function findWordRangeAtCursor(content: string, cursor: number) {
    const isWordChar = (char: string) => /[A-Za-z0-9_\u4e00-\u9fff]/.test(char);
    let start = cursor;
    let end = cursor;

    while (start > 0 && isWordChar(content[start - 1] || "")) {
      start -= 1;
    }

    while (end < content.length && isWordChar(content[end] || "")) {
      end += 1;
    }

    if (start === end) {
      return null;
    }

    return { start, end };
  }

  function getShortcutRange() {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return null;
    }

    let selectionStart = textarea.selectionStart ?? 0;
    let selectionEnd = textarea.selectionEnd ?? selectionStart;

    if (selectionStart === selectionEnd) {
      const wordRange = findWordRangeAtCursor(form.content, selectionStart);
      if (!wordRange) {
        return null;
      }
      selectionStart = wordRange.start;
      selectionEnd = wordRange.end;
    }

    return { selectionStart, selectionEnd };
  }

  function toggleInlineWrapper(prefix: string, suffix: string) {
    const range = getShortcutRange();
    if (!range) {
      return;
    }

    const { selectionStart, selectionEnd } = range;
    const content = form.content;
    const hasWrappedSelection =
      selectionStart >= prefix.length &&
      selectionEnd + suffix.length <= content.length &&
      content.slice(selectionStart - prefix.length, selectionStart) === prefix &&
      content.slice(selectionEnd, selectionEnd + suffix.length) === suffix;

    if (hasWrappedSelection) {
      const nextContent =
        content.slice(0, selectionStart - prefix.length) +
        content.slice(selectionStart, selectionEnd) +
        content.slice(selectionEnd + suffix.length);
      form.content = nextContent;
      focusEditor(selectionStart - prefix.length, selectionEnd - prefix.length);
      return;
    }

    const selectedText = content.slice(selectionStart, selectionEnd);
    form.content = `${content.slice(0, selectionStart)}${prefix}${selectedText}${suffix}${content.slice(selectionEnd)}`;
    focusEditor(selectionStart + prefix.length, selectionEnd + prefix.length);
  }

  function applyBoldShortcut() {
    toggleInlineWrapper("**", "**");
  }

  function applyItalicShortcut() {
    toggleInlineWrapper("*", "*");
  }

  function applyUnderlineShortcut() {
    toggleInlineWrapper("++", "++");
  }

  function applyInlineCodeShortcut() {
    toggleInlineWrapper("`", "`");
  }

  function applyImageShortcut() {
    insertSnippet("![图片描述](https://example.com/image.png)\n");
  }

  function applyStrikeShortcut() {
    toggleInlineWrapper("~~", "~~");
  }

  function applyTableShortcut() {
    insertSnippet("| 列1 | 列2 |\n| --- | --- |\n| 内容 | 内容 |\n");
  }

  function applyHeadingShortcut(level: number) {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }

    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const content = form.content;
    const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
    let lineEnd = content.indexOf("\n", selectionEnd);
    if (lineEnd === -1) {
      lineEnd = content.length;
    }

    const lineBlock = content.slice(lineStart, lineEnd);
    const nextPrefix = `${"#".repeat(level)} `;
    const transformedBlock = lineBlock
      .split("\n")
      .map((line) => {
        if (!line.trim()) {
          return line;
        }
        const stripped = line.replace(/^\s{0,3}#{1,6}\s+/, "");
        return `${nextPrefix}${stripped}`;
      })
      .join("\n");

    form.content = `${content.slice(0, lineStart)}${transformedBlock}${content.slice(lineEnd)}`;
    focusEditor(lineStart, lineStart + transformedBlock.length);
  }

  function replaceCurrentLineBlock(transformLine: (line: string, index: number) => string) {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }

    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const content = form.content;
    const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
    let lineEnd = content.indexOf("\n", selectionEnd);
    if (lineEnd === -1) {
      lineEnd = content.length;
    }

    const lineBlock = content.slice(lineStart, lineEnd);
    const transformedBlock = lineBlock
      .split("\n")
      .map((line, index) => transformLine(line, index))
      .join("\n");

    form.content = `${content.slice(0, lineStart)}${transformedBlock}${content.slice(lineEnd)}`;
    focusEditor(lineStart, lineStart + transformedBlock.length);
  }

  function applyQuoteShortcut() {
    replaceCurrentLineBlock((line) => {
      if (!line.trim()) {
        return line;
      }
      if (/^\s{0,3}>\s?/.test(line)) {
        return line.replace(/^\s{0,3}>\s?/, "");
      }
      return `> ${line}`;
    });
  }

  function applyUnorderedListShortcut() {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }

    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const content = form.content;
    const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
    let lineEnd = content.indexOf("\n", selectionEnd);
    if (lineEnd === -1) {
      lineEnd = content.length;
    }
    const lineBlock = content.slice(lineStart, lineEnd);
    const lines = lineBlock.split("\n");
    const nonEmptyLines = lines.filter((line) => line.trim());
    const isAllBulleted = nonEmptyLines.length > 0 && nonEmptyLines.every((line) => /^\s*-\s+/.test(line));

    replaceCurrentLineBlock((line) => {
      if (!line.trim()) {
        return line;
      }
      if (isAllBulleted) {
        return line.replace(/^\s*-\s+/, "");
      }
      return `- ${line}`;
    });
  }

  function applyOrderedListShortcut() {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }

    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const content = form.content;
    const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
    let lineEnd = content.indexOf("\n", selectionEnd);
    if (lineEnd === -1) {
      lineEnd = content.length;
    }
    const lineBlock = content.slice(lineStart, lineEnd);
    const lines = lineBlock.split("\n");
    const nonEmptyLines = lines.filter((line) => line.trim());
    const isAllOrdered = nonEmptyLines.length > 0 && nonEmptyLines.every((line) => /^\s*\d+\.\s+/.test(line));
    let order = 0;

    replaceCurrentLineBlock((line) => {
      if (!line.trim()) {
        return line;
      }
      if (isAllOrdered) {
        return line.replace(/^\s*\d+\.\s+/, "");
      }
      order += 1;
      return `${order}. ${line}`;
    });
  }

  function applyCodeBlockShortcut() {
    const textarea = editorTextareaRef.value;
    const fallbackSelection = form.content.length;
    const selectionStart = textarea?.selectionStart ?? fallbackSelection;
    const selectionEnd = textarea?.selectionEnd ?? fallbackSelection;
    const selectedText = form.content.slice(selectionStart, selectionEnd);
    const match = selectedText.match(/^```([\w-]*)\n([\s\S]*?)\n```$/);

    if (match) {
      const inner = match[2] || "";
      form.content = `${form.content.slice(0, selectionStart)}${inner}${form.content.slice(selectionEnd)}`;
      focusEditor(selectionStart, selectionStart + inner.length);
      return;
    }

    const code = selectedText || "const answer = true;";
    const wrapped = `\`\`\`ts\n${code}\n\`\`\``;
    form.content = `${form.content.slice(0, selectionStart)}${wrapped}${form.content.slice(selectionEnd)}`;
    focusEditor(selectionStart + 6, selectionStart + 6 + code.length);
  }

  function applyLinkShortcut() {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }

    const fallbackSelection = form.content.length;
    let selectionStart = textarea.selectionStart ?? fallbackSelection;
    let selectionEnd = textarea.selectionEnd ?? fallbackSelection;

    if (selectionStart === selectionEnd) {
      const wordRange = findWordRangeAtCursor(form.content, selectionStart);
      if (wordRange) {
        selectionStart = wordRange.start;
        selectionEnd = wordRange.end;
      }
    }

    const selectedText = form.content.slice(selectionStart, selectionEnd);
    const linkMatch = selectedText.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
    const defaultText = linkMatch?.[1] || selectedText || "链接文本";
    const defaultUrl = linkMatch?.[2] || "https://example.com";
    const url = import.meta.client ? window.prompt("请输入链接 URL", defaultUrl) : defaultUrl;

    if (!url) {
      return;
    }

    const nextContent = `[${defaultText}](${url.trim()})`;
    form.content = `${form.content.slice(0, selectionStart)}${nextContent}${form.content.slice(selectionEnd)}`;
    focusEditor(selectionStart + 1, selectionStart + 1 + defaultText.length);
  }

  function handleEditorShortcuts(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey)) {
      return;
    }

    if (event.altKey) {
      if (event.key.toLowerCase() === "t") {
        event.preventDefault();
        applyTableShortcut();
      }
      return;
    }

    if (event.key.toLowerCase() === "b") {
      event.preventDefault();
      applyBoldShortcut();
      return;
    }

    if (event.shiftKey && event.key.toLowerCase() === "i") {
      event.preventDefault();
      applyImageShortcut();
      return;
    }

    if (event.key.toLowerCase() === "i") {
      event.preventDefault();
      applyItalicShortcut();
      return;
    }

    if (event.key.toLowerCase() === "u") {
      event.preventDefault();
      applyUnderlineShortcut();
      return;
    }

    if (event.shiftKey && event.key.toLowerCase() === "k") {
      event.preventDefault();
      applyCodeBlockShortcut();
      return;
    }

    if (event.shiftKey && event.key.toLowerCase() === "l") {
      event.preventDefault();
      applyOrderedListShortcut();
      return;
    }

    if (event.shiftKey && (event.key === "~" || event.code === "Backquote")) {
      event.preventDefault();
      applyInlineCodeShortcut();
      return;
    }

    if (event.key.toLowerCase() === "q") {
      event.preventDefault();
      applyQuoteShortcut();
      return;
    }

    if (event.key.toLowerCase() === "k") {
      event.preventDefault();
      applyLinkShortcut();
      return;
    }

    if (event.key.toLowerCase() === "l") {
      event.preventDefault();
      applyUnorderedListShortcut();
      return;
    }

    if (event.key.toLowerCase() === "d") {
      event.preventDefault();
      applyStrikeShortcut();
      return;
    }

    if (/^[1-6]$/.test(event.key)) {
      event.preventDefault();
      applyHeadingShortcut(Number(event.key));
    }
  }

  return {
    focusEditor,
    replaceSelection,
    insertSnippet,
    findWordRangeAtCursor,
    getShortcutRange,
    toggleInlineWrapper,
    applyBoldShortcut,
    applyItalicShortcut,
    applyUnderlineShortcut,
    applyInlineCodeShortcut,
    applyImageShortcut,
    applyStrikeShortcut,
    applyTableShortcut,
    applyHeadingShortcut,
    replaceCurrentLineBlock,
    applyQuoteShortcut,
    applyUnorderedListShortcut,
    applyOrderedListShortcut,
    applyCodeBlockShortcut,
    applyLinkShortcut,
    handleEditorShortcuts,
  };
}
