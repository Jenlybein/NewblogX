import { computed, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";
import {
  deleteChatMessages,
  deleteChatSessions,
  deleteGlobalNoticeForCurrentUser,
  deleteSiteMessage,
  getChatMessages,
  getChatSessions,
  getGlobalNotices,
  getSiteMessages,
  markChatMessagesRead,
  markGlobalNoticeRead,
  markSiteMessageRead,
  sendChatMessage,
} from "~/services/inbox";
import { uploadImageByTask } from "~/services/image";
import type {
  ChatMessageItem,
  ChatNotificationPayload,
  ChatSessionItem,
  GlobalNoticeItem,
  SiteMessageItem,
} from "~/types/api";
import {
  buildChatClientMsgId,
  buildPreparedChatSession,
  isDraftChatSessionId,
  type InboxDraftSessionSeed,
} from "~/utils/chat";
import type { ChatSendType } from "~/utils/chatMessage";

export type InboxTab = "site" | "global" | "chat";
export type SiteMessageGroup = 1 | 2 | 3;
const MESSAGE_PAGE_SIZE = 9;

interface UseInboxCenterOptions {
  draftSessionSeed?: MaybeRefOrGetter<InboxDraftSessionSeed | null>;
}

interface SendCurrentChatMessageResult {
  usedPreparedSession: boolean;
  resolvedSessionId: string | null;
}

export function useInboxCenter(options: UseInboxCenterOptions = {}) {
  const authStore = useAuthStore();
  const messageStore = useMessageStore();
  const chatStore = useChatStore();
  const { socketStatus, socketError, sendError, setSendError, connect, takeNotifications, inbox } = useChatSocket();

  const activeTab = shallowRef<InboxTab>("site");
  const activeSiteGroup = shallowRef<SiteMessageGroup>(1);
  const sitePage = shallowRef(1);
  const globalPage = shallowRef(1);
  const siteHasMore = shallowRef(false);
  const globalHasMore = shallowRef(false);
  const siteMessages = ref<SiteMessageItem[]>([]);
  const globalNotices = ref<GlobalNoticeItem[]>([]);
  const chatSessionsState = ref<ChatSessionItem[]>([]);
  const chatMessages = ref<ChatMessageItem[]>([]);
  const chatKeyword = shallowRef("");
  const chatDraft = shallowRef("");
  // 草稿的发送类型：1 文本 / 7 Markdown。图片不走草稿，走 sendChatImage。
  const chatDraftType = shallowRef<ChatSendType>(1);
  const imagePending = shallowRef(false);
  // 上一次失败的发送：同一内容重试时复用它的幂等键。
  // 否则"请求已落库、响应丢了"的重试会变成第二条消息。
  const failedSend = shallowRef<{ content: string; msgType: ChatSendType; clientMsgId: string } | null>(null);

  const sitePending = shallowRef(false);
  const globalPending = shallowRef(false);
  const sessionPending = shallowRef(false);
  const messagePending = shallowRef(false);

  const draftSessionSeed = computed(() => {
    const seed = toValue(options.draftSessionSeed);
    if (!seed?.receiverId) {
      return null;
    }

    return {
      receiverId: seed.receiverId,
      receiverNickname: seed.receiverNickname || "新会话",
      receiverAvatar: seed.receiverAvatar || "",
      relation: seed.relation ?? 0,
    } satisfies InboxDraftSessionSeed;
  });

  const preparedSession = computed(() => {
    const seed = draftSessionSeed.value;
    if (!seed) {
      return null;
    }

    const existingSession = chatSessionsState.value.find((item) => item.receiver_id === seed.receiverId);
    if (existingSession) {
      return null;
    }

    return buildPreparedChatSession(seed);
  });

  const chatSessions = computed(() => {
    if (!preparedSession.value) {
      return chatSessionsState.value;
    }

    return [preparedSession.value, ...chatSessionsState.value];
  });

  const activeSession = computed(
    () => chatSessions.value.find((item) => item.session_id === chatStore.activeSessionId) ?? null,
  );

  const filteredChatSessions = computed(() => {
    const keyword = chatKeyword.value.trim().toLowerCase();
    if (!keyword) {
      return chatSessions.value;
    }

    return chatSessions.value.filter((item) => {
      const haystack = `${item.receiver_nickname} ${item.last_msg_content}`.toLowerCase();
      return haystack.includes(keyword);
    });
  });

  const siteCategories = computed(() => [
    {
      key: 1 as const,
      label: "评论与回复",
      hint: "",
      count: messageStore.summary.comment_msg_count,
    },
    {
      key: 2 as const,
      label: "点赞与收藏",
      hint: "",
      count: messageStore.summary.digg_favor_msg_count,
    },
    {
      key: 3 as const,
      label: "系统通知",
      hint: "",
      count: messageStore.summary.system_msg_count,
    },
  ]);

  function pickPreferredActiveSession(serverSessions = chatSessionsState.value) {
    const seed = draftSessionSeed.value;
    const existingActiveSession = chatStore.activeSessionId
      ? chatSessions.value.find((item) => item.session_id === chatStore.activeSessionId)
      : null;

    if (seed) {
      const matchedServerSession = serverSessions.find((item) => item.receiver_id === seed.receiverId);
      if (matchedServerSession) {
        if (chatStore.activeSessionId !== matchedServerSession.session_id) {
          chatStore.setActiveSession(matchedServerSession.session_id);
        }
        return matchedServerSession.session_id;
      }

      if (preparedSession.value) {
        if (chatStore.activeSessionId !== preparedSession.value.session_id) {
          chatStore.setActiveSession(preparedSession.value.session_id);
        }
        return preparedSession.value.session_id;
      }
    }

    if (existingActiveSession) {
      return existingActiveSession.session_id;
    }

    const nextSessionId = serverSessions[0]?.session_id ?? null;
    if (chatStore.activeSessionId !== nextSessionId) {
      chatStore.setActiveSession(nextSessionId);
    }
    return nextSessionId;
  }

  async function loadSiteMessages(page = sitePage.value, options: { append?: boolean } = {}) {
    const group = activeSiteGroup.value;
    sitePending.value = true;
    try {
      const payload = await getSiteMessages({ group, page, limit: MESSAGE_PAGE_SIZE });
      const nextList = options.append ? [...siteMessages.value, ...payload.list] : payload.list;

      if (group !== activeSiteGroup.value) {
        return payload.list;
      }

      sitePage.value = page;
      siteHasMore.value = payload.has_more;
      siteMessages.value = nextList;
      return payload.list;
    } finally {
      sitePending.value = false;
    }
  }

  async function loadGlobalNotices(page = globalPage.value, options: { append?: boolean } = {}) {
    globalPending.value = true;
    try {
      const payload = await getGlobalNotices({ page, limit: MESSAGE_PAGE_SIZE, type: 1 });
      globalPage.value = page;
      globalHasMore.value = payload.has_more;
      globalNotices.value = options.append ? [...globalNotices.value, ...payload.list] : payload.list;
      return payload.list;
    } finally {
      globalPending.value = false;
    }
  }

  async function loadChatSessions() {
    sessionPending.value = true;
    try {
      const payload = await getChatSessions({ type: 1 });
      chatSessionsState.value = payload.list;
      pickPreferredActiveSession(payload.list);
      return payload.list;
    } finally {
      sessionPending.value = false;
    }
  }

  async function loadChatMessages(sessionId = chatStore.activeSessionId) {
    if (!sessionId || isDraftChatSessionId(sessionId)) {
      chatMessages.value = [];
      return [];
    }

    messagePending.value = true;
    try {
      const payload = await getChatMessages({ sessionId, type: 1 });
      chatMessages.value = payload.list;
      const unreadIds = payload.list.filter((item) => !item.is_read && !item.is_self).map((item) => item.id);
      if (unreadIds.length) {
        await markChatMessagesRead(unreadIds).catch(() => undefined);
        await messageStore.refreshSummary().catch(() => undefined);
      }
      return payload.list;
    } finally {
      messagePending.value = false;
    }
  }

  async function markCurrentSiteGroupRead() {
    await markSiteMessageRead({ group: activeSiteGroup.value });
    sitePage.value = 1;
    await Promise.all([messageStore.refreshSummary(), loadSiteMessages(1)]);
  }

  async function removeSiteMessage(payload: { id?: string; group?: SiteMessageGroup }) {
    await deleteSiteMessage(payload);
    sitePage.value = 1;
    await Promise.all([messageStore.refreshSummary(), loadSiteMessages(1)]);
  }

  async function markAllGlobalRead() {
    const unreadIds = globalNotices.value.filter((item) => !item.is_read).map((item) => item.id);
    if (!unreadIds.length) return;
    await markGlobalNoticeRead(unreadIds);
    globalPage.value = 1;
    await Promise.all([messageStore.refreshSummary(), loadGlobalNotices(1)]);
  }

  async function removeGlobalNotice(id: string) {
    await deleteGlobalNoticeForCurrentUser([id]);
    globalPage.value = 1;
    await Promise.all([messageStore.refreshSummary(), loadGlobalNotices(1)]);
  }

  async function removeChatSession(sessionId: string) {
    if (isDraftChatSessionId(sessionId)) {
      chatMessages.value = [];
      if (chatStore.activeSessionId === sessionId) {
        chatStore.setActiveSession(chatSessionsState.value[0]?.session_id ?? null);
      }
      return;
    }

    await deleteChatSessions([sessionId]);
    await Promise.all([messageStore.refreshSummary(), loadChatSessions()]);
    if (chatStore.activeSessionId === sessionId) {
      pickPreferredActiveSession();
    }
  }

  async function removeChatMessage(id: string) {
    await deleteChatMessages([id]);
    await Promise.all([messageStore.refreshSummary(), loadChatMessages()]);
  }

  // newClientNonce 生成幂等键的随机段：优先 crypto.randomUUID，旧环境下退化。
  function newClientNonce() {
    const cryptoApi = globalThis.crypto;
    if (cryptoApi && typeof cryptoApi.randomUUID === "function") {
      return cryptoApi.randomUUID();
    }
    return `${Math.random().toString(36).slice(2)}${Math.random().toString(36).slice(2)}`;
  }

  // resolveClientMsgId 让"同一次发送的重试"复用同一个幂等键。
  function resolveClientMsgId(content: string, msgType: ChatSendType) {
    const last = failedSend.value;
    if (last && last.content === content && last.msgType === msgType) {
      return last.clientMsgId;
    }
    return buildChatClientMsgId(newClientNonce, () => Date.now());
  }

  // 发送的统一出口：乐观插入 → HTTP 落库 → 刷新会话与消息。
  //
  // #201 之后发送只走 HTTP（唯一发送通道）：WebSocket 断开/断网时照样能发出，
  // WS 只负责把通知推给对端。三种消息类型共用一个出口，类型只传一次。
  async function deliverChatMessage(
    content: string,
    msgType: ChatSendType,
  ): Promise<false | SendCurrentChatMessageResult> {
    const currentSession = activeSession.value;
    if (!content.trim() || !currentSession) {
      return false;
    }

    const usedPreparedSession = isDraftChatSessionId(currentSession.session_id);
    const clientMsgId = resolveClientMsgId(content, msgType);
    const optimisticMessage: ChatMessageItem = {
      id: `temp-${Date.now()}`,
      sender_id: String(authStore.profileId ?? ""),
      receiver_id: currentSession.receiver_id,
      session_id: currentSession.session_id,
      content,
      send_time: new Date().toISOString(),
      msg_status: 1,
      msg_type: msgType,
      is_self: true,
      is_read: false,
    };

    chatMessages.value = [...chatMessages.value, optimisticMessage];

    try {
      const sent = await sendChatMessage({
        receiver_id: currentSession.receiver_id,
        msg_type: msgType,
        content,
        client_msg_id: clientMsgId,
      });

      // 用服务端返回的真实消息替换乐观项：幂等重试返回的是第一次落库那条，
      // 换成它的 ID 后本地不会留下两条。
      if (sent?.id) {
        chatMessages.value = chatMessages.value.map((item) =>
          item.id === optimisticMessage.id ? sent : item,
        );
      }
      failedSend.value = null;
      // 发送成功要清掉上一次的拒绝原因：否则界面上还挂着上一条「站点关闭陌生人私信」。
      setSendError("");

      await loadChatSessions();

      if (usedPreparedSession) {
        const resolvedSession = chatSessionsState.value.find((item) => item.receiver_id === currentSession.receiver_id) ?? null;
        if (resolvedSession) {
          chatStore.setActiveSession(resolvedSession.session_id);
          await loadChatMessages(resolvedSession.session_id);
          return {
            usedPreparedSession: true,
            resolvedSessionId: resolvedSession.session_id,
          };
        }

        return {
          usedPreparedSession: true,
          resolvedSessionId: null,
        };
      }

      await loadChatMessages(currentSession.session_id);
      return {
        usedPreparedSession: false,
        resolvedSessionId: currentSession.session_id,
      };
    } catch (error) {
      chatMessages.value = chatMessages.value.filter((item) => item.id !== optimisticMessage.id);
      // 记住这次发送：用户再点一次"发送"（内容相同）时复用同一个幂等键，
      // 避免"其实已落库、只是响应丢了"的重试变成第二条消息。
      failedSend.value = { content, msgType, clientMsgId };
      // 拒绝原因直接透出后端文案（站点策略等），由私信面板就地显示（票 202）。
      setSendError(error instanceof Error ? error.message : "消息发送失败。");
      throw error;
    }
  }

  /** 发送草稿框里的内容；类型由 chatDraftType 决定（文本或 Markdown）。 */
  async function sendCurrentChatMessage(): Promise<false | SendCurrentChatMessageResult> {
    const content = chatDraft.value.trim();
    if (!content) {
      return false;
    }

    chatDraft.value = "";
    try {
      return await deliverChatMessage(content, chatDraftType.value);
    } catch (error) {
      chatDraft.value = content;
      throw error;
    }
  }

  /**
   * 发一张图片。
   *
   * 顺序是"先上传、再把地址当消息发出"：后端收到 msg_type=2 时会把 content
   * 包装成 {"kind":"image","image_url":...} 落库，所以前端这里传**裸地址**即可，
   * 不要自己拼那段 JSON（拼了就会被包两层，解析出来是 JSON 文本）。
   */
  async function sendChatImage(file: File): Promise<false | SendCurrentChatMessageResult> {
    if (!activeSession.value) {
      return false;
    }

    imagePending.value = true;
    try {
      // 私信发图是任意比例照片，#215 没有定形：显式不声明用途，
      // 后端确认时才不会拿封面 16:9 规则把它误判为失败。
      const uploaded = await uploadImageByTask(file, { purpose: null });
      if (!uploaded.url) {
        throw new Error("上传成功但未返回图片地址");
      }
      return await deliverChatMessage(uploaded.url, 2);
    } finally {
      imagePending.value = false;
    }
  }

  watch(
    () => activeSiteGroup.value,
    async () => {
      if (activeTab.value === "site") {
        sitePage.value = 1;
        await loadSiteMessages(1);
      }
    },
  );

  watch(
    () => draftSessionSeed.value,
    () => {
      if (activeTab.value === "chat") {
        pickPreferredActiveSession();
      }
    },
  );

  watch(
    () => activeTab.value,
    async (tab) => {
      if (tab === "site") {
        sitePage.value = 1;
        await Promise.all([
          loadSiteMessages(1),
          loadGlobalNotices(1).catch(() => []),
        ]);
        return;
      }

      if (tab === "global") {
        globalPage.value = 1;
        await loadGlobalNotices(1);
        return;
      }

      await connect();
      await loadChatSessions();
      await loadChatMessages();
    },
    { immediate: true },
  );

  watch(
    () => chatStore.activeSessionId,
    async (sessionId) => {
      if (activeTab.value === "chat" && sessionId) {
        await loadChatMessages(sessionId);
      }
    },
  );

  // WS 只推通知（有新消息 / 对方已读）：收到后刷新会话列表与未读数，
  // 命中当前打开的会话时再刷新消息。这样"对端在线但当前没打开这个会话"
  // 也会更新左侧红点，而不是只回购当前会话。
  watch(
    () => inbox.value.length,
    async () => {
      const notifications: ChatNotificationPayload[] = takeNotifications();
      if (!notifications.length) return;

      const activeId = activeSession.value?.session_id;
      const tasks: Promise<unknown>[] = [loadChatSessions(), messageStore.refreshSummary()];
      if (
        activeId &&
        !isDraftChatSessionId(activeId) &&
        notifications.some((item) => item.session_id === activeId)
      ) {
        tasks.push(loadChatMessages(activeId));
      }
      await Promise.all(tasks);
    },
  );

  /**
   * 建立（或重连）实时连接，并在连上后补齐断线期间错过的消息。
   *
   * 为什么要补：通知是"尽力而为"——对方离线时那条通知帧没人接，中继会把事件
   * 标记完成；重连后不会自动重放。所以连上之后必须主动刷新一次会话、未读与
   * 当前会话消息，否则"断线期间对方发来的消息"在界面上要等下次切页才出现。
   */
  async function connectSocket() {
    const instance = await connect();
    if (!instance) {
      return null;
    }
    const tasks: Promise<unknown>[] = [loadChatSessions(), messageStore.refreshSummary()];
    if (chatStore.activeSessionId) {
      tasks.push(loadChatMessages(chatStore.activeSessionId));
    }
    await Promise.all(tasks);
    return instance;
  }

  async function loadMoreSiteMessages() {
    if (sitePending.value || !siteHasMore.value) {
      return [];
    }
    return loadSiteMessages(sitePage.value + 1, { append: true });
  }

  async function loadMoreGlobalNotices() {
    if (globalPending.value || !globalHasMore.value) {
      return [];
    }
    return loadGlobalNotices(globalPage.value + 1, { append: true });
  }

  return {
    activeTab,
    activeSiteGroup,
    activeSessionId: computed({
      get: () => chatStore.activeSessionId,
      set: (value) => chatStore.setActiveSession(value),
    }),
    activeSession,
    preparedSession,
    siteCategories,
    sitePage,
    siteHasMore,
    globalPage,
    globalHasMore,
    siteMessages,
    globalNotices,
    chatSessions,
    filteredChatSessions,
    chatMessages,
    chatKeyword,
    chatDraft,
    chatDraftType,
    imagePending,
    sitePending,
    globalPending,
    sessionPending,
    messagePending,
    socketStatus,
    socketError,
    // 发送被拒的明确原因（票 202）：直接透出后端文案（HTTP 发送失败或 WS 异常帧），
    // 由私信面板就地显示，而不是只丢进控制台或连接状态里。
    chatSendError: sendError,
    loadSiteMessages,
    loadGlobalNotices,
    loadMoreSiteMessages,
    loadMoreGlobalNotices,
    loadChatSessions,
    loadChatMessages,
    markCurrentSiteGroupRead,
    removeSiteMessage,
    markAllGlobalRead,
    removeGlobalNotice,
    removeChatSession,
    removeChatMessage,
    sendCurrentChatMessage,
    sendChatImage,
    connectSocket,
  };
}
