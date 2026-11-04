import { computed, getCurrentScope, onScopeDispose, ref, shallowRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";
import {
  deleteChatMessages,
  deleteChatSessions,
  getChatMessages,
  getChatSessions,
  markChatSessionRead,
  sendChatMessage,
} from "~/services/inbox";
import type { ChatMessageCursor } from "~/services/inbox";
import { uploadImageByTask } from "~/services/image";
import type { ChatMessageItem, ChatSessionItem } from "~/types/api";
import {
  buildChatClientMsgId,
  buildPreparedChatSession,
  isDraftChatSessionId,
  type InboxDraftSessionSeed,
} from "~/utils/chat";
import { mergeChatMessages, toAscending } from "~/utils/chatTimeline";
import type { ChatSendType } from "~/utils/chatMessage";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 私信会话（`/messages/chat` 的右内容）。
 *
 * 本票（#240）收窄后的范围只有私信：
 *   - 站内信（评论 / 互动 / 系统）与全局通知已迁到 useMessageCenter（#239），
 *     这里再留一份"旧 inbox 页"的分支就是死代码 —— 两个取数路径迟早漂移；
 *   - 因此没有 `activeTab` / `activeSiteGroup` 之类开关：进到本 composable 就是私信页。
 *
 * 职责分工：
 *   - 会话与消息的真相在 HTTP（`GET /api/chat/sessions`、`GET /api/chat/messages`）；
 *   - 发送也走 HTTP（#201：唯一发送通道，断 WS 也能发出）；
 *   - WS 只推通知。连接本身属于整个登录会话（`useChatSocket` 单例 + useMessageRealtime），
 *     这个 composable 只在私信页挂载期间**订阅**通知：命中当前会话才增量拉消息。
 *
 * # 时间线的不变量（游标分页改造后）
 *   - `chatMessages` 是**一条升序数组**：最旧在最上、最新在最下；
 *   - 第一页重置它，`loadOlderChatMessages` 只往前**前插**，WS 通知只往后**增量追加**；
 *   - 因此发送 / 删除 / 已读都不得整页重拉，否则用户已经翻出来的历史会被折叠掉。
 */

/** 聊天窗口单页条数：与后端默认一致，翻页与通知增量共用。 */
const CHAT_WINDOW_PAGE_SIZE = 7;

interface UseInboxCenterOptions {
  draftSessionSeed?: MaybeRefOrGetter<InboxDraftSessionSeed | null>;
}

interface SendCurrentChatMessageResult {
  usedPreparedSession: boolean;
  resolvedSessionId: string | null;
}

/**
 * 失败后静默重试一次，永不再抛。
 *
 * 用于「进会话即已读」这类不能挡住界面的后台动作：一次瞬时网络抖动不该让
 * 已读状态漂移，但也不能连一次补救都不做就把错误吞掉。
 */
async function withOneRetry(task: () => Promise<unknown>): Promise<boolean> {
  try {
    await task();
    return true;
  } catch {
    try {
      await task();
      return true;
    } catch {
      // 两次都失败也不打扰用户：下次进会话 / 下一条通知会再试。
      return false;
    }
  }
}

export function useInboxCenter(options: UseInboxCenterOptions = {}) {
  const authStore = useAuthStore();
  const messageStore = useMessageStore();
  const chatStore = useChatStore();
  const socket = useChatSocket();

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

  const sessionPending = shallowRef(false);
  const messagePending = shallowRef(false);

  // 游标分页状态：nextCursor 为空串表示「没有更早的一页」。
  const hasMoreHistory = ref(false);
  const olderPending = ref(false);
  let nextCursor: ChatMessageCursor = { before_send_time: "", before_id: "" };
  // 请求序号：切换会话后在途的响应必须丢弃，否则会把上一个会话的消息写进当前会话。
  let requestId = 0;

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

  // 草稿会话只活在前端：如果服务端已经有一条和这个接收方的会话，就不该再插一条"预备会话"。
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

  const activeSessionId = computed({
    get: () => chatStore.activeSessionId,
    set: (value) => chatStore.setActiveSession(value),
  });

  /** 重置与「当前打开的是哪条会话」绑定的所有分页状态。 */
  function resetChatTimeline() {
    requestId += 1;
    chatMessages.value = [];
    nextCursor = { before_send_time: "", before_id: "" };
    hasMoreHistory.value = false;
    olderPending.value = false;
  }

  /**
   * 选中一条会话。
   *
   * 草稿种子优先：从作者主页「发私信」进来时，用户要看的是**那个人的会话**，
   * 不是列表里的第一条；有服务端会话就用它，没有就选中前端预备会话。
   */
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

  /**
   * 进会话即已读（Q7）。
   *
   * 走整会话模式而不是攒 7 个 id：后端 `session_id` 模式是幂等的，第二次调用
   * 返回 0 affected 也不是错误，所以进来一次 / 收到一条通知都可以无条件调用。
   * 本地把收到的消息直接标成已读（`msg_status: 3`），不重拉整页 —— 重拉会把
   * 已经翻出来的历史折叠掉。整段动作失败静默重试一次，绝不抛给界面、不弹 toast。
   */
  async function markActiveSessionRead(): Promise<void> {
    const sessionId = chatStore.activeSessionId;
    if (!sessionId || isDraftChatSessionId(sessionId)) {
      return;
    }

    await withOneRetry(async () => {
      await markChatSessionRead(sessionId);
      // 会话中途切走就不再动当前数组（它已经是新会话的了）。
      if (chatStore.activeSessionId !== sessionId) {
        return;
      }
      chatMessages.value = chatMessages.value.map((item) =>
        item.is_self ? item : { ...item, is_read: true, msg_status: 3 },
      );
      await Promise.all([messageStore.refreshSummary(), loadChatSessions()]);
    });
  }

  /**
   * 拉（或重拉）第一页：重置数组、游标与 hasMore。
   *
   * 只允许在「切换会话 / 重连 / 预备会话刚变成真会话 / 用户点重试」这些
   * 语义上确实要重置的地方调用。后端给的是 DESC，这里转升序后落到界面。
   */
  async function loadChatMessages(sessionId = chatStore.activeSessionId) {
    if (!sessionId || isDraftChatSessionId(sessionId)) {
      resetChatTimeline();
      return [];
    }

    const currentRequest = (requestId += 1);
    messagePending.value = true;
    // 先清空：切会话的瞬间不能还显示上一条会话的消息。
    chatMessages.value = [];
    nextCursor = { before_send_time: "", before_id: "" };
    hasMoreHistory.value = false;
    try {
      const payload = await getChatMessages({ sessionId, type: 1, limit: CHAT_WINDOW_PAGE_SIZE });
      if (currentRequest !== requestId || chatStore.activeSessionId !== sessionId) {
        return [];
      }
      const ascending = toAscending(payload.list);
      chatMessages.value = ascending;
      nextCursor = payload.next_cursor;
      hasMoreHistory.value = payload.has_more;
      messagePending.value = false;
      // 首屏加载完就标记已读：不等用户手动点，也不等下一条 WS 通知。
      await markActiveSessionRead();
      return ascending;
    } finally {
      if (currentRequest === requestId) {
        messagePending.value = false;
      }
    }
  }

  /**
   * 往前翻一页更早的历史，返回**前插的条数**（组件用它做滚动锚点：
   * `newScrollHeight - oldScrollHeight`）。
   *
   * 无游标 / 无会话 / 正在翻页 / 没有更多 / 会话已切走 → 0。
   * 前插顺序必须保持：先 dedupe 掉当前数组里已有的 id，再把这一页升序结果
   * 摆在前面；`mergeChatMessages` 只做「按 id 替换」而不重排累积历史。
   */
  async function loadOlderChatMessages(): Promise<number> {
    const sessionId = chatStore.activeSessionId;
    if (
      !sessionId
      || isDraftChatSessionId(sessionId)
      || olderPending.value
      || !hasMoreHistory.value
      || !nextCursor.before_send_time
    ) {
      return 0;
    }

    // 与前插 / 首屏共用同一个序号：会话一变，所有在途的分页响应一起作废。
    // 这里只「读」序号而不自增：否则在途的首屏响应会被误判为陈旧并丢弃。
    const currentRequest = requestId;
    const cursor = nextCursor;
    olderPending.value = true;
    try {
      const payload = await getChatMessages({
        sessionId,
        type: 1,
        limit: CHAT_WINDOW_PAGE_SIZE,
        cursor,
      });
      if (currentRequest !== requestId || chatStore.activeSessionId !== sessionId) {
        return 0;
      }
      const olderAsc = toAscending(payload.list);
      const currentIds = new Set(chatMessages.value.map((item) => item.id));
      const prepended = olderAsc.filter((item) => !currentIds.has(item.id));
      // 前插 + 用 merge 让重叠 id 拿到最新的 is_read / msg_status，顺序不变。
      chatMessages.value = mergeChatMessages([...prepended, ...chatMessages.value], olderAsc);
      nextCursor = payload.next_cursor;
      hasMoreHistory.value = payload.has_more;
      return prepended.length;
    } finally {
      if (currentRequest === requestId) {
        olderPending.value = false;
      }
    }
  }

  /**
   * WS 通知的增量路径（Q19）。
   *
   * 只取最新一页，然后把结果合并进累积数组：已存在的行被服务端新鲜状态
   * 替换（顺序不动），没见过的行按时间追加到尾部。整页重拉会把用户已经
   * 翻出来的两三页历史折叠成 7 条，这里必须避免。
   */
  async function loadNewestChatMessages(sessionId: string) {
    // 通知增量只读取当前序号：它自己不是一次「会话切换」，不能作废首屏响应。
    const currentRequest = requestId;
    const payload = await getChatMessages({ sessionId, type: 1, limit: CHAT_WINDOW_PAGE_SIZE });
    if (currentRequest !== requestId || chatStore.activeSessionId !== sessionId) {
      return;
    }
    chatMessages.value = mergeChatMessages(chatMessages.value, toAscending(payload.list));
  }

  /** 界面「重试」按钮：清掉发送错误，再重拉第一页；静默、永不抛。 */
  async function retryChatLoad(): Promise<void> {
    socket.setSendError("");
    try {
      await loadChatMessages(chatStore.activeSessionId);
    } catch {
      // 重试失败也不能再抛一层：界面只保留「可再次重试」的状态。
    }
  }

  async function removeChatSession(sessionId: string) {
    if (isDraftChatSessionId(sessionId)) {
      if (chatStore.activeSessionId === sessionId) {
        resetChatTimeline();
        chatStore.setActiveSession(chatSessionsState.value[0]?.session_id ?? null);
      } else {
        resetChatTimeline();
      }
      return;
    }

    await deleteChatSessions([sessionId]);
    await Promise.all([messageStore.refreshSummary(), loadChatSessions()]);
    if (chatStore.activeSessionId === sessionId) {
      resetChatTimeline();
      pickPreferredActiveSession();
    }
  }

  async function removeChatMessage(id: string) {
    await deleteChatMessages([id]);
    // 只删本地这一行：整页重拉会丢掉已翻出的历史。
    chatMessages.value = chatMessages.value.filter((item) => item.id !== id);
    await messageStore.refreshSummary();
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

  // 发送的统一出口：乐观插入 → HTTP 落库 → 就地替换 + 刷新会话。
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

    // 升序数组：新消息永远在尾部。
    chatMessages.value = [...chatMessages.value, optimisticMessage];

    try {
      const sent = await sendChatMessage({
        receiver_id: currentSession.receiver_id,
        msg_type: msgType,
        content,
        client_msg_id: clientMsgId,
      });

      // 用服务端返回的真实消息替换乐观项：幂等重试返回的是第一次落库那条，
      // 换成它的 ID 后本地不会留下两条，位置仍留在尾部。
      if (sent?.id) {
        chatMessages.value = chatMessages.value.map((item) =>
          item.id === optimisticMessage.id ? sent : item,
        );
      }
      failedSend.value = null;
      // 发送成功要清掉上一次的拒绝原因：否则界面上还挂着上一条「站点关闭陌生人私信」。
      socket.setSendError("");

      // 成功发送**不**调用 loadChatMessages()：那会把累积的历史页折叠回 7 条。
      await loadChatSessions();

      if (usedPreparedSession) {
        const resolvedSession = chatSessionsState.value.find((item) => item.receiver_id === currentSession.receiver_id) ?? null;
        if (resolvedSession) {
          chatStore.setActiveSession(resolvedSession.session_id);
          // 预备会话刚变成真会话：这是一次真正的会话切换，重置是正确的。
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
      socket.setSendError(errorMessage(error, "消息发送失败。"));
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

  /**
   * 建立（或重连）实时连接，并在连上后补齐断线期间错过的消息。
   *
   * 为什么要补：通知是"尽力而为"——对方离线时那条通知帧没人接，中继会把事件
   * 标记完成；重连后不会自动重放。所以连上之后必须主动刷新一次会话、未读与
   * 当前会话消息，否则"断线期间对方发来的消息"在界面上要等下次切页才出现。
   *
   * 连接本身是全局单例（离开私信页不关）；这里只在私信页需要"立即补齐"。
   * 重连是允许重置第一页的少数场景之一。
   */
  async function connectSocket() {
    const instance = await socket.connect();
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

  // 进入私信页即建连（幂等）并拉会话；断开的情况下也照常拉，界面不依赖 WS。
  async function bootstrap() {
    try {
      await socket.connect();
    } catch {
      // 连接失败不该挡住列表：WS 只做通知，会话与消息都走 HTTP。
    }
    await loadChatSessions().catch(() => undefined);
    await loadChatMessages().catch(() => undefined);
  }
  void bootstrap();

  watch(
    () => draftSessionSeed.value,
    () => {
      pickPreferredActiveSession();
    },
  );

  watch(
    () => chatStore.activeSessionId,
    async (sessionId) => {
      if (sessionId) {
        await loadChatMessages(sessionId);
      }
    },
  );

  // WS 只推通知（有新消息 / 对方已读）：收到后刷新会话列表与未读数，
  // 命中当前打开的会话时**增量**合并最新一页，并立即标记已读。
  //
  // 订阅的生命周期 = 私信页的生命周期（取消订阅而已，**不动连接**）：
  // 连接属于整个登录会话，页头未读订阅还在用同一条连接。
  const stopNotifications = socket.onNotification((notification) => {
    const activeId = activeSession.value?.session_id;
    const tasks: Promise<unknown>[] = [loadChatSessions(), messageStore.refreshSummary()];
    if (activeId && !isDraftChatSessionId(activeId) && notification.session_id === activeId) {
      // 先合并再标已读：这样刚追加进来的那一行也会被就地标成已读。
      tasks.push((async () => {
        await loadNewestChatMessages(activeId);
        await markActiveSessionRead();
      })());
    }
    void Promise.all(tasks).catch(() => undefined);
  });
  if (getCurrentScope()) {
    onScopeDispose(stopNotifications);
  }

  return {
    activeSessionId,
    activeSession,
    preparedSession,
    chatSessions,
    filteredChatSessions,
    chatMessages,
    chatKeyword,
    chatDraft,
    chatDraftType,
    imagePending,
    sessionPending,
    messagePending,
    hasMoreHistory,
    olderPending,
    socketStatus: socket.socketStatus,
    socketError: socket.socketError,
    // 发送被拒的明确原因（票 202）：直接透出后端文案（HTTP 发送失败或 WS 异常帧），
    // 由私信面板就地显示，而不是只丢进控制台或连接状态里。
    chatSendError: socket.sendError,
    loadChatSessions,
    loadChatMessages,
    loadOlderChatMessages,
    markActiveSessionRead,
    retryChatLoad,
    removeChatSession,
    removeChatMessage,
    sendCurrentChatMessage,
    sendChatImage,
    connectSocket,
  };
}
