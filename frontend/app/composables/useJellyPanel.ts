import { computed, ref, watch } from "vue";

import type { JellyAssistantApi } from "~/composables/useJellyAssistant";
import { streamAiAssistantReply, type AiConversationMessage } from "~/services/ai";

// 捏捏 AI 面板（票 #10）的状态层。
//
// 面板的**开关**不在这里另开一套 ref：它消费 #9 的共享 `useJellyAssistant` 实例
// （`isOpen` 是唯一开关真相源，`open()`/`close()` 是唯一动作）。这里只负责对话本身：
// 消息、流式追加、停止、关闭即中断、错误分类、历史保留/清空、快捷问句。
//
// 边界：真正发请求的是既有 `services/ai.ts`（契约零改动）；本模块只编排与兜底，
// 且流式函数可注入（单测用假 SSE 流，不碰真实网络）。

export type JellyPanelErrorKind = "timeout" | "quota" | "disabled" | "auth" | "generic";

/** 四类错误各说一句人话（可行动），不要甩原始错误对象给用户。 */
export const JELLY_PANEL_ERROR_TEXTS: Record<JellyPanelErrorKind, string> = {
  timeout: "等太久了：AI 这次没有按时回答。稍后再试，或者把问题拆短一点。",
  quota: "今天的 AI 配额用完了，明天再来就恢复了；也可以先去后台看看用量。",
  disabled: "AI 还开着小差：站点暂时关闭了 AI 助手。",
  auth: "登录状态过期了，先重新登录再问吧。",
  generic: "这次回答中断了，请稍后再试一次。",
};

/** 空态四个快捷问句：点击只填进输入框，不自动发送（避免用户被代发）。 */
export const JELLY_QUICK_QUESTIONS = [
  "帮我找几篇讲 Go 并发的文章",
  "这篇长文的核心观点是什么？",
  "帮我把这段话改得更克制",
  "站内有哪些高赞的 Nuxt 笔记？",
] as const;

const TIMEOUT_HINTS = ["超时", "timeout", "timed out", "504", "deadline"] as const;
const QUOTA_HINTS = ["配额", "quota", "次数", "限流", "429", "rate limit", "rate_limit", "exceeded"] as const;
const DISABLED_HINTS = ["未启用", "未开启", "关闭了 ai", "ai 已关闭", "ai已关闭", "disabled", "not enabled"] as const;
const AUTH_HINTS = ["请先登录", "未登录", "登录过期", "401", "403", "unauthorized", "session expired"] as const;

/** 把后端/网络错误归到四类可行动文案之一（纯函数，便于单独钉住分类真值）。 */
export function classifyJellyPanelError(error: unknown): { kind: JellyPanelErrorKind; text: string } {
  const raw = error instanceof Error ? error.message : typeof error === "string" ? error : "";
  const normalized = raw.toLowerCase();
  const has = (hints: readonly string[]) => hints.some((hint) => normalized.includes(hint));

  if (has(DISABLED_HINTS)) return { kind: "disabled", text: JELLY_PANEL_ERROR_TEXTS.disabled };
  if (has(AUTH_HINTS)) return { kind: "auth", text: JELLY_PANEL_ERROR_TEXTS.auth };
  if (has(TIMEOUT_HINTS)) return { kind: "timeout", text: JELLY_PANEL_ERROR_TEXTS.timeout };
  if (has(QUOTA_HINTS)) return { kind: "quota", text: JELLY_PANEL_ERROR_TEXTS.quota };
  return { kind: "generic", text: JELLY_PANEL_ERROR_TEXTS.generic };
}

export interface JellyChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  /** 还在流式输出：UI 据此显示打字光标。 */
  streaming?: boolean;
  /** 被用户停止/关闭面板中断。 */
  stopped?: boolean;
  /** 失败的分类（UI 据此换错误底色/文案）。 */
  errorKind?: JellyPanelErrorKind;
}

export interface StreamJellyPanelReplyOptions {
  history?: AiConversationMessage[];
  signal?: AbortSignal;
  onChunk?: (chunk: string) => void;
}

export type StreamJellyPanelReply = (
  content: string,
  options: StreamJellyPanelReplyOptions,
) => Promise<string>;

export interface JellyPanelDeps {
  /** #9 的共享实例（页面级 provide）；面板绝不自己 new。 */
  assistant: JellyAssistantApi;
  /** 流式服务；缺省走 services/ai.ts 的 streamAiAssistantReply。 */
  streamReply?: StreamJellyPanelReply;
  /** 发送前的登录检查；缺省 authStore.initializeSession()。 */
  ensureLoggedIn?: () => Promise<boolean>;
  /** 未登录时的登录引导；缺省 uiStore.openAuthModal()。 */
  requestLogin?: () => void;
  /** 消息 id 生成（测试注入稳定 id）。 */
  makeId?: () => string;
}

/** 关闭/停止时给一句明确的收尾，避免把「空消息气泡」留在界面上。 */
export const JELLY_PANEL_STOPPED_TEXT = "本次回答已停止。";
/** 流正常结束但一个字都没生成时的兜底。 */
export const JELLY_PANEL_EMPTY_REPLY_TEXT = "AI 暂时没有返回可显示的内容。";

function defaultEnsureLoggedIn(): Promise<boolean> {
  return useAuthStore().initializeSession();
}

function defaultRequestLogin(): void {
  useUiStore().openAuthModal();
}

let idSequence = 0;
function defaultMakeId(): string {
  idSequence += 1;
  return `jelly-msg-${Date.now()}-${idSequence}`;
}

export function useJellyPanel(deps: JellyPanelDeps) {
  const { assistant } = deps;
  const streamReply = deps.streamReply ?? streamAiAssistantReply;
  const ensureLoggedIn = deps.ensureLoggedIn ?? defaultEnsureLoggedIn;
  const requestLogin = deps.requestLogin ?? defaultRequestLogin;
  const makeId = deps.makeId ?? defaultMakeId;

  const { isOpen, isAvailable, open, close } = assistant;

  const messages = ref<JellyChatMessage[]>([]);
  const draft = ref("");
  const pending = ref(false);
  const activeController = ref<AbortController | null>(null);
  // 「这次发送还没真正开始就被要求停止/关闭」：登录检查是异步的，
  // 期间用户可能已经关面板，必须记住这个意图并在启动后立刻收尾。
  let stopRequested = false;
  // 有一次发送在途（含登录检查阶段）：只有这时 stop/close 才需要记「停止意图」，
  // 否则关闭一个空面板会把下一轮发送误判成「已停止」。
  let sendInFlight = false;

  const canSend = computed(() => Boolean(draft.value.trim()) && !pending.value);
  const showEmptyState = computed(() => messages.value.length === 0);

  function createMessage(
    role: JellyChatMessage["role"],
    content: string,
    extra: Partial<JellyChatMessage> = {},
  ): JellyChatMessage {
    return { id: makeId(), role, content, ...extra };
  }

  function conversationHistory(): AiConversationMessage[] {
    return messages.value
      .filter((message) => !message.streaming && message.content.trim())
      .map((message) => ({ role: message.role, content: message.content }));
  }

  /** 中断进行中的请求；状态收尾统一由 send 的 catch/finally 做。 */
  function interruptActive() {
    if (!sendInFlight) return;
    stopRequested = true;
    const controller = activeController.value;
    if (controller && !controller.signal.aborted) {
      controller.abort();
    }
  }

  function finishMessage(message: JellyChatMessage) {
    message.streaming = false;
    pending.value = false;
    activeController.value = null;
    stopRequested = false;
  }

  async function send(explicitContent?: string) {
    const content = (explicitContent ?? draft.value).trim();
    if (!content || pending.value) {
      return;
    }

    sendInFlight = true;
    try {
      if (!isAvailable.value) {
        // AI 总闸关闭：不发请求，但也要把「为什么没回答」说清楚。
        messages.value = [
          ...messages.value,
          createMessage("user", content),
          createMessage("assistant", JELLY_PANEL_ERROR_TEXTS.disabled, { errorKind: "disabled" }),
        ];
        draft.value = "";
        return;
      }

      const loggedIn = await ensureLoggedIn();
      if (!loggedIn) {
        // 登录态失效：收起面板并引导登录，不产生「失败」消息。
        requestLogin();
        close();
        return;
      }

      const history = conversationHistory();
      const userMessage = createMessage("user", content);
      const assistantMessage = createMessage("assistant", "", { streaming: true });
      messages.value = [...messages.value, userMessage, assistantMessage];
      draft.value = "";
      pending.value = true;

      const controller = new AbortController();
      activeController.value = controller;

      // 登录检查期间已经要求停止：不再发请求，直接按「已停止」收尾。
      if (stopRequested) {
        stopRequested = false;
        assistantMessage.content = JELLY_PANEL_STOPPED_TEXT;
        assistantMessage.stopped = true;
        finishMessage(assistantMessage);
        return;
      }

      try {
        await streamReply(content, {
          history,
          signal: controller.signal,
          onChunk: (chunk) => {
            // 防御迟到回调：abort 之后任何 chunk 都不能再改这条消息（无幽灵响应）。
            if (controller.signal.aborted) return;
            assistantMessage.content += chunk;
          },
        });

        if (!assistantMessage.content.trim()) {
          assistantMessage.content = JELLY_PANEL_EMPTY_REPLY_TEXT;
        }
      } catch (error) {
        if (controller.signal.aborted) {
          assistantMessage.stopped = true;
          if (!assistantMessage.content.trim()) {
            assistantMessage.content = JELLY_PANEL_STOPPED_TEXT;
          }
          return;
        }

        const classified = classifyJellyPanelError(error);
        assistantMessage.errorKind = classified.kind;
        if (!assistantMessage.content.trim()) {
          assistantMessage.content = classified.text;
        }
      } finally {
        finishMessage(assistantMessage);
      }
    } finally {
      sendInFlight = false;
    }
  }

  function stop() {
    interruptActive();
  }

  function clear() {
    interruptActive();
    messages.value = [];
    draft.value = "";
    pending.value = false;
  }

  function fillDraft(question: string) {
    draft.value = question;
  }

  function openPanel() {
    open();
  }

  function closePanel() {
    interruptActive();
    close();
  }

  function togglePanel() {
    if (isOpen.value) {
      closePanel();
    } else {
      openPanel();
    }
  }

  function handleComposerKeydown(event: KeyboardEvent) {
    if (event.key !== "Enter" || event.shiftKey || event.isComposing) {
      return;
    }

    event.preventDefault();
    void send();
  }

  // 面板开关是共享实例的 isOpen：一旦被关掉（点把手/点关闭/外部收起），
  // 无论走哪条路径都必须中断在途请求，否则会留下一条永远在「输入中」的幽灵回答。
  watch(
    isOpen,
    (open) => {
      if (!open) interruptActive();
    },
    { flush: "sync" },
  );

  return {
    // 状态
    isOpen,
    isAvailable,
    messages,
    draft,
    pending,
    canSend,
    showEmptyState,
    quickQuestions: JELLY_QUICK_QUESTIONS,
    // 动作
    send,
    stop,
    clear,
    fillDraft,
    openPanel,
    closePanel,
    togglePanel,
    handleComposerKeydown,
  };
}

export type JellyPanelApi = ReturnType<typeof useJellyPanel>;
