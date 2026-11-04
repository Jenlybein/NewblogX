<script setup lang="ts">
import { computed, watch } from "vue";
import { useMessage } from "naive-ui";

import InboxChatPanel from "~/components/inbox/InboxChatPanel.vue";
import { useInboxCenter } from "~/composables/useInboxCenter";
import { isDraftChatSessionId, parseDraftSessionSeed } from "~/utils/chat";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 消息中心「私信」子项（2026-11-05 重构）。
 *
 * 复用 `InboxChatPanel`（会话列表 + 聊天窗）与 `useInboxCenter`（升序消息数组、
 * 复合游标、整会话已读、增量 WS 追加）。`draft_receiver_*` 跨页契约不变：
 * 写入由 useDraftChatJump / buildDraftChatQuery 单一来源负责，命中已有会话
 * 直接选中，没有就建前端占位，首条消息发出后转真实会话并清 query。
 */
const route = useRoute();
const router = useRouter();
const message = useMessage();

const draftSessionSeed = computed(() => parseDraftSessionSeed(route.query));

const {
  activeSessionId,
  activeSession,
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
  socketStatus,
  socketError,
  chatSendError,
  removeChatSession,
  removeChatMessage,
  sendCurrentChatMessage,
  sendChatImage,
  connectSocket,
  loadOlderChatMessages,
  retryChatLoad,
} = useInboxCenter({ draftSessionSeed });

async function clearDraftSessionQuery() {
  if (!draftSessionSeed.value) return;
  await router.replace({
    query: {
      ...route.query,
      draft_receiver_id: undefined,
      draft_receiver_nickname: undefined,
      draft_receiver_avatar: undefined,
      draft_relation: undefined,
    },
  });
}

async function handleSend() {
  try {
    const result = await sendCurrentChatMessage();
    if (result && result.usedPreparedSession) await clearDraftSessionQuery();
  } catch (error) {
    message.error(errorMessage(error, "发送消息失败"));
  }
}

async function handleSendImage(file: File) {
  try {
    const result = await sendChatImage(file);
    if (result && result.usedPreparedSession) await clearDraftSessionQuery();
  } catch (error) {
    message.error(errorMessage(error, "发送图片失败"));
  }
}

async function handleReconnect() {
  await connectSocket();
}

watch(
  () => [draftSessionSeed.value?.receiverId, activeSession.value?.session_id, activeSession.value?.receiver_id] as const,
  async ([receiverId, sessionId, activeReceiverId]) => {
    if (!receiverId || !sessionId || isDraftChatSessionId(sessionId) || activeReceiverId !== receiverId) return;
    await clearDraftSessionQuery();
  },
);
</script>

<template>
  <InboxChatPanel
    :sessions="filteredChatSessions"
    :active-session-id="activeSessionId ?? null"
    :current-session="activeSession"
    :items="chatMessages"
    :keyword="chatKeyword"
    :draft="chatDraft"
    :draft-type="chatDraftType"
    :image-pending="imagePending"
    :session-pending="sessionPending"
    :message-pending="messagePending"
    :has-more-history="hasMoreHistory"
    :older-pending="olderPending"
    :socket-status="socketStatus"
    :socket-error="socketError"
    :send-error="chatSendError"
    @update:active-session-id="activeSessionId = $event"
    @update:keyword="chatKeyword = $event"
    @update:draft="chatDraft = $event"
    @update:draft-type="chatDraftType = $event"
    @send="handleSend()"
    @send-image="handleSendImage($event)"
    @remove-session="removeChatSession($event)"
    @remove-message="removeChatMessage($event)"
    @reconnect="handleReconnect()"
    @retry="retryChatLoad()"
    @load-older="loadOlderChatMessages()"
  />
</template>
