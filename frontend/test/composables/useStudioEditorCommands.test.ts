import { nextTick } from "vue";
import { describe, expect, it } from "vitest";

import { useStudioEditorCommands } from "~/composables/useStudioEditorCommands";

// 编辑器命令：光标定位与选区替换。
//
// 这一组是写作页最容易出"看起来对、用起来错"的地方：
// 光标该落在哪、连续按两次快捷键会不会嵌套、中文词边界算不算 ——
// 以前只能靠手点验证。抽出 composable 的直接目的就是让它们可测。

function harness(content = "") {
  const form = { content };
  // textarea 用最小假对象：本组测试只关心选择区间的读写
  const textarea = {
    selectionStart: 0,
    selectionEnd: 0,
    value: content,
    focus: () => undefined,
    setSelectionRange: (start: number, end: number) => {
      textarea.selectionStart = start;
      textarea.selectionEnd = end;
    },
  };
  const textareaRef = { value: textarea as unknown as HTMLTextAreaElement };

  const commands = useStudioEditorCommands({ form, textareaRef });
  return { commands, form, textarea };
}

describe("useStudioEditorCommands / findWordRangeAtCursor", () => {
  it("光标在词中时取出整个词（含中文）", () => {
    const { commands } = harness();
    // "hello 世界" 的第 3 个字符处（l 与 l 之间）
    expect(commands.findWordRangeAtCursor("hello 世界", 3)).toEqual({ start: 0, end: 5 });
    // "世界" 的中间
    expect(commands.findWordRangeAtCursor("hello 世界", 7)).toEqual({ start: 6, end: 8 });
  });

  it("光标紧贴词尾时选中该词（不是返回 null）", () => {
    const { commands } = harness();
    // 索引 5 是第一个空格，左边还是 'o' —— 用户视觉上"光标在词尾"，应当选中这个词
    expect(commands.findWordRangeAtCursor("hello  world", 5)).toEqual({ start: 0, end: 5 });
  });

  it("光标在两个空格之间才返回 null（左右都不是词字符）", () => {
    const { commands } = harness();
    // "hello  world"：索引 6 左右都是空格
    expect(commands.findWordRangeAtCursor("hello  world", 6)).toBeNull();
  });

  it("标点不算词的一部分", () => {
    const { commands } = harness();
    expect(commands.findWordRangeAtCursor("a-b", 2)).toEqual({ start: 2, end: 3 });
  });
});

describe("useStudioEditorCommands / replaceSelection", () => {
  it("替换选中文本并把光标放到插入内容之后（避免二次按快捷键嵌套）", async () => {
    const { commands, form, textarea } = harness("hello world");
    textarea.selectionStart = 0;
    textarea.selectionEnd = 5;

    commands.replaceSelection((selected) => ({ content: `**${selected}**`, start: 2, end: 2 + selected.length }));
    // focusEditor 走 nextTick，光标要在下一个 tick 才写回 textarea
    await nextTick();

    expect(form.content).toBe("**hello** world");
    // 光标落在被包裹的词之后、后缀之前
    expect(textarea.selectionStart).toBe(2);
    expect(textarea.selectionEnd).toBe(7);
  });

  it("没有 textarea 时退化为「追加到末尾」，不抛错", async () => {
    const form = { content: "abc" };
    const commands = useStudioEditorCommands({ form, textareaRef: { value: null } });

    commands.replaceSelection(() => ({ content: "def", start: 0, end: 3 }));
    await nextTick();

    expect(form.content).toBe("abcdef");
  });
});

describe("useStudioEditorCommands / 快捷键", () => {
  it("Ctrl+B 给当前词加粗", async () => {
    const { commands, form, textarea } = harness("hello");
    textarea.selectionStart = 2;
    textarea.selectionEnd = 2;

    commands.applyBoldShortcut();
    await nextTick();

    expect(form.content).toBe("**hello**");
  });

  it("Ctrl+K 生成链接语法", async () => {
    const { commands, form, textarea } = harness("text");
    textarea.selectionStart = 0;
    textarea.selectionEnd = 4;

    commands.applyLinkShortcut();
    await nextTick();

    expect(form.content).toContain("](http");
  });
});
