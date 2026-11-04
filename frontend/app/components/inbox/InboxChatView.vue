<script setup lang="ts">
import { computed, watch } from "vue";
import { useMessage } from "naive-ui";

import InboxChatPanel from "~/components/inbox/InboxChatPanel.vue";
import { useInboxCenter } from "~/composables/useInboxCenter";
import { isDraftChatSessionId, parseDraftSessionSeed } from "~/utils/chat";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 消息中心「私信」子项（`/messages/chat` 的右内容，票 240）。
 *
 * 拆成独立组件而不是塞进页面：私信只有这一栏才需要取数、建连与订阅通知。
 * 页面按子项 `v-if` 渲染本组件，访问评论/互动子项时就不会顺手拉起私信会话。
 *
 * 复用既有实现：会话列表 + 聊天窗是 `InboxChatPanel`，取数/发送/通知订阅是
 * `useInboxCenter`（本票已收窄为纯私信）。
 *
 * `draft_receiver_*` 是 `users/[id].vue`「发私信」的跨页契约（原样保留）：
 * 命中已有会话就直接选中它，没有就建前端预备会话，首条消息发出后转真实会话并
 * 清掉 query，避免刷新后又落回"预备会话"。
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
  socketStatus,
  socketError,
  chatSendError,
  removeChatSession,
  removeChatMessage,
  sendCurrentChatMessage,
  sendChatImage,
  connectSocket,
} = useInboxCenter({ draftSessionSeed });

const isPreparedSession = computed(() => isDraftChatSessionId(activeSession.value?.session_id ?? null));

async function clearDraftSessionQuery() {
  if (!draftSessionSeed.value) {
    return;
  }

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
    // 只在"预备会话转成真实会话"这一步清 query：普通会话的 query 本来就不含 draft。
    if (result && result.usedPreparedSession) {
      await clearDraftSessionQuery();
    }
    message.success("消息已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送消息失败"));
  }
}

// 发图片：先上传再发消息，两步都在 useInboxCenter 里，这里只管提示。
// 上传失败与发送失败给不同的文案——用户要能分清"图没传上去"和"图传了但消息没发出去"。
async function handleSendImage(file: File) {
  try {
    const result = await sendChatImage(file);
    if (result && result.usedPreparedSession) {
      await clearDraftSessionQuery();
    }
    message.success("图片已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送图片失败"));
  }
}

async function handleReconnect() {
  await connectSocket();
}

// 草稿命中了服务端已存在的会话：此时会话已经是真实的，清掉 query 即可。
// 刷新后还会重新命中同一条会话，但 URL 上不该长期挂着 draft 参数。
watch(
  () => [draftSessionSeed.value?.receiverId, activeSession.value?.session_id, activeSession.value?.receiver_id] as const,
  async ([receiverId, sessionId, activeReceiverId]) => {
    if (!receiverId || !sessionId || isDraftChatSessionId(sessionId) || activeReceiverId !== receiverId) {
      return;
    }

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
    :prepared-session="isPreparedSession"
    :image-pending="imagePending"
    :session-pending="sessionPending"
    :message-pending="messagePending"
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
  />
</template>
