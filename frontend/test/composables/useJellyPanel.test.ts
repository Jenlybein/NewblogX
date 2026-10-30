import { flushPromises } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import {
  JELLY_PANEL_ERROR_TEXTS,
  JELLY_QUICK_QUESTIONS,
  classifyJellyPanelError,
  useJellyPanel,
  type JellyPanelDeps,
} from "~/composables/useJellyPanel";
import type { JellyAssistantApi } from "~/composables/useJellyAssistant";

// 捏捏 AI 面板（票 #10）的 S1 接缝。
//
// 面板的"网络"全部走注入的假流式服务（真实现是 services/ai.ts 的 streamAiAssistantReply，
// 它自己的单测已钉住 SSE 解析/401 重试）。这里只钉面板自己的行为：
//   - 流式逐块追加 → 完成；中途停止；关闭面板即中断（且不再追加幽灵内容）；
//   - 超时/配额/未启用/未登录四类错误各说各的人话（可行动文案）；
//   - 历史保留与一键清空（关掉再打开不能丢对话）；
//   - 空态快捷问句点击后填进输入框。
//
// 注意：面板的开关**不是**自己的一套 ref，而是共享的 useJellyAssistant 实例 ——
// 角色以为开着、面板以为关着的分裂状态就是这么来的。

type FakeAssistant = {
  isOpen: ReturnType<typeof ref<boolean>>;
  isAvailable: ReturnType<typeof ref<boolean>>;
  open: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
};

function fakeAssistant(options: { open?: boolean; available?: boolean } = {}): FakeAssistant {
  const assistant: FakeAssistant = {
    isOpen: ref(options.open ?? true),
    isAvailable: ref(options.available ?? true),
    open: vi.fn(() => {
      assistant.isOpen.value = true;
    }),
    close: vi.fn(() => {
      assistant.isOpen.value = false;
    }),
  };
  return assistant;
}

/** 一次可控流式：emit 逐块推内容，finish 正常结束，abort 时像真实 fetch 一样抛 AbortError。 */
function controllableStream() {
  let emit: ((chunk: string) => void) | null = null;
  let finish: (() => void) | null = null;
  const signals: AbortSignal[] = [];

  const streamReply = (content: string, options: { history?: unknown[]; signal?: AbortSignal; onChunk?: (chunk: string) => void }) =>
    new Promise<string>((resolve, reject) => {
      if (options.signal) signals.push(options.signal);
      emit = (chunk) => {
        // 故意允许 abort 之后还回调（真实世界里的迟到竞态），
        // 由面板自己挡住 —— 这就是「无幽灵响应」要测的那条防御。
        options.onChunk?.(chunk);
      };
      finish = () => resolve("");
      options.signal?.addEventListener("abort", () => {
        const error = new Error("The operation was aborted.");
        error.name = "AbortError";
        reject(error);
      });
    });

  return {
    streamReply,
    emit: (chunk: string) => emit?.(chunk),
    finish: () => finish?.(),
    signals,
  };
}

interface HarnessOverrides {
  aiAvailable?: boolean;
  loggedIn?: boolean;
  streamReply?: JellyPanelDeps["streamReply"];
}

function harness(overrides: HarnessOverrides = {}) {
  const assistant = fakeAssistant({ available: overrides.aiAvailable ?? true });
  const requestLogin = vi.fn();
  let idCounter = 0;
  const calls: Array<{ content: string; options: { history?: unknown[]; signal?: AbortSignal } }> = [];

  const streamReply: NonNullable<JellyPanelDeps["streamReply"]> =
    overrides.streamReply ??
    (async (_content, options) => {
      options.onChunk?.("先");
      await Promise.resolve();
      options.onChunk?.("答");
      return "先答";
    });

  const panel = useJellyPanel({
    assistant: assistant as unknown as JellyAssistantApi,
    streamReply: (content, options) => {
      calls.push({ content, options });
      return streamReply(content, options);
    },
    ensureLoggedIn: async () => overrides.loggedIn ?? true,
    requestLogin,
    makeId: () => `msg-${(idCounter += 1)}`,
  });

  return { panel, assistant, requestLogin, calls, streamReply };
}

describe("useJellyPanel · 流式回答", () => {
  it("发送后逐块追加内容，完成后 pending 归位", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    const sending = panel.send("你好");
    await nextTick();
    expect(panel.pending.value).toBe(true);
    expect(panel.messages.value).toHaveLength(2);
    expect(panel.messages.value[0]).toMatchObject({ role: "user", content: "你好" });
    expect(panel.messages.value[1]).toMatchObject({ role: "assistant", streaming: true, content: "" });

    s.emit("你");
    await nextTick();
    expect(panel.messages.value[1].content).toBe("你");

    s.emit("好呀");
    s.finish();
    await sending;

    expect(panel.messages.value[1].content).toBe("你好呀");
    expect(panel.messages.value[1].streaming).toBe(false);
    expect(panel.pending.value).toBe(false);
  });

  it("空内容不发送（输入框白空格不算提问）", async () => {
    const { panel, calls } = harness();

    await panel.send("   ");

    expect(panel.messages.value).toHaveLength(0);
    expect(calls).toHaveLength(0);
  });

  it("发送后清空输入框；pending 期间不允许并发发送", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    panel.draft.value = "第一问";
    const first = panel.send();
    await nextTick();
    expect(panel.draft.value).toBe("");
    expect(panel.pending.value).toBe(true);

    await panel.send("第二问");
    expect(panel.messages.value).toHaveLength(2);

    s.finish();
    await first;
  });

  it("中途停止：消息保留已生成内容并标记 stopped，pending 归位", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    const sending = panel.send("讲个长回答");
    await nextTick();
    s.emit("前半段");
    await nextTick();

    panel.stop();
    await sending;

    const assistantMessage = panel.messages.value[1];
    expect(assistantMessage.content).toBe("前半段");
    expect(assistantMessage.stopped).toBe(true);
    expect(assistantMessage.streaming).toBe(false);
    expect(panel.pending.value).toBe(false);
  });

  it("一个字都没出来就被停止时，给一句「已停止」的兜底文案", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    const sending = panel.send("讲个长回答");
    panel.stop();
    await sending;

    expect(panel.messages.value[1].content).toBe("本次回答已停止。");
    expect(panel.messages.value[1].stopped).toBe(true);
  });

  it("关闭面板即中断请求，迟到的 chunk 不会再追加（无幽灵响应）", async () => {
    const s = controllableStream();
    const { panel, assistant } = harness({ streamReply: s.streamReply });

    const sending = panel.send("问一个会超时的题");
    await nextTick();
    s.emit("已经说了");
    await nextTick();

    panel.closePanel();
    await nextTick();

    expect(assistant.isOpen.value).toBe(false);
    expect(s.signals[0]?.aborted).toBe(true);

    // 迟到的回调（真实世界里的竞态）也不能改已经停止的消息。
    s.emit("幽灵内容");
    await sending;
    await nextTick();

    expect(panel.messages.value[1].content).toBe("已经说了");
    expect(panel.pending.value).toBe(false);
  });

  it("把历史（最近对话）传给流式服务，服务层自己截最近 6 轮", async () => {
    const { panel, calls } = harness();

    await panel.send("第一问");
    await nextTick();

    panel.draft.value = "第二问";
    await panel.send();

    const secondCall = calls[1];
    expect(secondCall.content).toBe("第二问");
    expect(secondCall.options.history).toEqual([
      { role: "user", content: "第一问" },
      { role: "assistant", content: "先答" },
    ]);
  });

  it("面板关掉再打开，历史仍在（关面板不等于清空对话）", async () => {
    const { panel, assistant } = harness();

    await panel.send("保留我");
    panel.closePanel();
    await nextTick();
    panel.openPanel();

    expect(assistant.isOpen.value).toBe(true);
    expect(panel.messages.value).toHaveLength(2);
  });

  it("一键清空：消息清空、进行中的请求被中断", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    const sending = panel.send("正在回答");
    await nextTick();
    s.emit("半句");
    await nextTick();

    panel.clear();
    await sending;
    await nextTick();

    expect(panel.messages.value).toHaveLength(0);
    expect(panel.pending.value).toBe(false);
  });
});

describe("useJellyPanel · 错误分类（超时 / 配额 / 未启用 / 未登录）", () => {
  it("超时：给出可行动文案，而不是「失败」两个字", async () => {
    const { panel } = harness({
      streamReply: async () => {
        throw new Error("AI 请求超时");
      },
    });

    await panel.send("问一下");

    const message = panel.messages.value[1];
    expect(message.errorKind).toBe("timeout");
    expect(message.content).toBe(JELLY_PANEL_ERROR_TEXTS.timeout);
    expect(message.content).toContain("稍后");
  });

  it("配额：提示今天用完了、何时再来", async () => {
    const { panel } = harness({
      streamReply: async () => {
        throw new Error("今日 AI 配额已用尽");
      },
    });

    await panel.send("问一下");

    const message = panel.messages.value[1];
    expect(message.errorKind).toBe("quota");
    expect(message.content).toBe(JELLY_PANEL_ERROR_TEXTS.quota);
    expect(message.content).toContain("配额");
  });

  it("AI 总闸关闭：不发请求，直接说「还开着小差」", async () => {
    const { panel, calls } = harness({ aiAvailable: false });

    await panel.send("问一下");

    expect(calls).toHaveLength(0);
    const message = panel.messages.value[1];
    expect(message.errorKind).toBe("disabled");
    expect(message.content).toBe(JELLY_PANEL_ERROR_TEXTS.disabled);
  });

  it("登录态失效：先引导登录并收起面板，不产生失败消息", async () => {
    const { panel, assistant, requestLogin } = harness({ loggedIn: false });

    await panel.send("问一下");

    expect(requestLogin).toHaveBeenCalledTimes(1);
    expect(assistant.close).toHaveBeenCalled();
    expect(panel.messages.value).toHaveLength(0);
  });

  it("没识别出的失败给通用文案（不能把原始错误对象甩给用户）", async () => {
    const { panel } = harness({
      streamReply: async () => {
        throw new Error("socket hang up");
      },
    });

    await panel.send("问一下");

    const message = panel.messages.value[1];
    expect(message.errorKind).toBe("generic");
    expect(message.content).toBe(JELLY_PANEL_ERROR_TEXTS.generic);
  });

  it("分类函数本身对常见后端文案稳定（单测门禁的独立真值）", () => {
    expect(classifyJellyPanelError(new Error("read timeout")).kind).toBe("timeout");
    expect(classifyJellyPanelError(new Error("quota exceeded")).kind).toBe("quota");
    expect(classifyJellyPanelError(new Error("AI 未启用")).kind).toBe("disabled");
    expect(classifyJellyPanelError(new Error("请先登录后再使用 AI 辅助。")).kind).toBe("auth");
    expect(classifyJellyPanelError(new Error("boom")).kind).toBe("generic");
    expect(classifyJellyPanelError("任意非 Error 值").kind).toBe("generic");
  });
});

describe("useJellyPanel · 快捷问句与输入区", () => {
  it("空态给出 4 个可直接点的问句", () => {
    const { panel } = harness();

    expect(panel.quickQuestions).toHaveLength(4);
    expect(panel.messages.value).toHaveLength(0);
  });

  it("点击快捷问句 = 把问题填进输入框（不自动发送）", () => {
    const { panel, calls } = harness();

    panel.fillDraft(JELLY_QUICK_QUESTIONS[1]);

    expect(panel.draft.value).toBe("这篇长文的核心观点是什么？");
    expect(calls).toHaveLength(0);
  });

  it("Enter 发送、Shift+Enter 换行、输入法组合中的 Enter 不发送", async () => {
    const { panel, calls } = harness();

    panel.draft.value = "回车发送";
    const prevented = vi.fn();
    panel.handleComposerKeydown({
      key: "Enter",
      shiftKey: false,
      isComposing: false,
      preventDefault: prevented,
    } as unknown as KeyboardEvent);
    await flushPromises();

    expect(prevented).toHaveBeenCalled();
    expect(calls).toHaveLength(1);

    panel.handleComposerKeydown({
      key: "Enter",
      shiftKey: true,
      isComposing: false,
      preventDefault: vi.fn(),
    } as unknown as KeyboardEvent);
    panel.handleComposerKeydown({
      key: "Enter",
      shiftKey: false,
      isComposing: true,
      preventDefault: vi.fn(),
    } as unknown as KeyboardEvent);
    await flushPromises();

    expect(calls).toHaveLength(1);
  });

  it("canSend：空输入或进行中都不能发", async () => {
    const s = controllableStream();
    const { panel } = harness({ streamReply: s.streamReply });

    expect(panel.canSend.value).toBe(false);

    panel.draft.value = "有内容";
    expect(panel.canSend.value).toBe(true);

    const sending = panel.send();
    await nextTick();
    expect(panel.canSend.value).toBe(false);

    s.finish();
    await sending;
  });
});
