<script setup lang="ts">
import { useMessage } from "naive-ui";
import InboxChatPanel from "~/components/inbox/InboxChatPanel.vue";
import InboxSitePanel from "~/components/inbox/InboxSitePanel.vue";
import type { SiteMessageItem } from "~/types/api";
import type { SiteMessageGroup } from "~/composables/useInboxCenter";
import { isDraftChatSessionId, type InboxDraftSessionSeed } from "~/utils/chat";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const route = useRoute();
const router = useRouter();
const messageStore = useMessageStore();

const draftSessionSeed = computed<InboxDraftSessionSeed | null>(() => {
  const receiverId = typeof route.query.draft_receiver_id === "string" ? route.query.draft_receiver_id.trim() : "";
  if (!receiverId) {
    return null;
  }

  return {
    receiverId,
    receiverNickname:
      typeof route.query.draft_receiver_nickname === "string" && route.query.draft_receiver_nickname.trim()
        ? route.query.draft_receiver_nickname
        : "新会话",
    receiverAvatar: typeof route.query.draft_receiver_avatar === "string" ? route.query.draft_receiver_avatar : "",
    relation:
      typeof route.query.draft_relation === "string" && Number.isFinite(Number(route.query.draft_relation))
        ? Number(route.query.draft_relation)
        : 0,
  };
});

const inbox = useInboxCenter({
  draftSessionSeed,
});
const {
  activeTab,
  activeSiteGroup,
  siteHasMore,
  globalHasMore,
  activeSessionId,
  activeSession,
  siteCategories,
  siteMessages,
  globalNotices,
  filteredChatSessions,
  chatMessages,
  chatKeyword,
  chatDraft,
  sitePending,
  globalPending,
  sessionPending,
  messagePending,
  socketStatus,
  socketError,
  loadSiteMessages,
  loadGlobalNotices,
  loadMoreSiteMessages,
  loadMoreGlobalNotices,
  markCurrentSiteGroupRead,
  removeSiteMessage,
  markAllGlobalRead,
  removeGlobalNotice,
  removeChatSession,
  removeChatMessage,
  sendCurrentChatMessage,
  connectSocket,
} = inbox;

type InboxTab = "site" | "chat";
type InboxMessageGroup = SiteMessageGroup | "global";

function resolveInboxTab(value: unknown): InboxTab {
  return value === "chat" ? "chat" : "site";
}

watch(
  () => route.query.tab,
  (value) => {
    activeTab.value = resolveInboxTab(value);
  },
  { immediate: true },
);

watch(
  () => activeTab.value,
  (value) => {
    const next = resolveInboxTab(route.query.tab);
    if (next === value) return;
    void router.replace({
      query: {
        ...route.query,
        tab: value,
      },
    });
  },
);

const activeMessageGroup = shallowRef<InboxMessageGroup>(1);

function resolveMessageGroup(value: unknown): InboxMessageGroup {
  return value === "global" ? "global" : [1, 2, 3].includes(Number(value)) ? (Number(value) as SiteMessageGroup) : 1;
}

watch(
  () => route.query.group,
  (value) => {
    activeMessageGroup.value = resolveMessageGroup(value);
  },
  { immediate: true },
);

watch(
  () => activeMessageGroup.value,
  (value) => {
    if (value !== "global") {
      activeSiteGroup.value = value;
    }
    const nextGroup = resolveMessageGroup(route.query.group);
    if (nextGroup === value) return;
    void router.replace({
      query: {
        ...route.query,
        group: value === 1 ? undefined : String(value),
      },
    });
  },
);

watch(
  () => activeMessageGroup.value,
  async (value) => {
    if (activeTab.value === "chat") return;
    if (value === "global") {
      await loadGlobalNotices(1);
      return;
    }
    activeSiteGroup.value = value;
    await loadSiteMessages(1);
  },
  { immediate: true },
);

const mergedCategories = computed(() => [
  ...siteCategories.value,
  {
    key: "global" as const,
    label: "全局通知",
    hint: "",
    count: messageStore.summary.global_msg_count,
  },
]);

const mergedItems = computed<SiteMessageItem[]>(() => {
  if (activeMessageGroup.value !== "global") {
    return siteMessages.value;
  }
  return globalNotices.value.map((item) => ({
    id: item.id,
    created_at: item.create_at,
    updated_at: item.create_at,
    type: 9,
    receiver_id: "",
    action_user_id: null,
    action_user_nickname: "系统",
    action_user_avatar: null,
    content: item.content,
    article_id: "",
    comment_id: "",
    article_title: item.title,
    link_title: item.title,
    link_herf: item.herf,
    is_read: item.is_read,
    read_at: null,
  }));
});

const mergedPending = computed(() => (activeMessageGroup.value === "global" ? globalPending.value : sitePending.value));
const mergedHasMore = computed(() => (activeMessageGroup.value === "global" ? globalHasMore.value : siteHasMore.value));
const isPreparedSession = computed(() => {
  const sessionId = activeSession.value?.session_id;
  return sessionId ? isDraftChatSessionId(sessionId) : false;
});

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

async function handleMarkAllRead() {
  if (activeMessageGroup.value === "global") {
    await markAllGlobalRead();
    return;
  }
  await markCurrentSiteGroupRead();
}

async function handleRemoveMessage(id: string) {
  if (activeMessageGroup.value === "global") {
    await removeGlobalNotice(id);
    return;
  }
  await removeSiteMessage({ id });
}

async function handleClearGroup() {
  if (activeMessageGroup.value === "global") {
    const unreadIds = globalNotices.value.filter((item) => !item.is_read).map((item) => item.id);
    if (unreadIds.length) {
      await markAllGlobalRead();
    }
    return;
  }
  await removeSiteMessage({ group: activeSiteGroup.value });
}

async function handleLoadMore() {
  if (activeMessageGroup.value === "global") {
    await loadMoreGlobalNotices();
    return;
  }
  await loadMoreSiteMessages();
}

async function handleSend() {
  try {
    const result = await sendCurrentChatMessage();
    if (result && result.usedPreparedSession && result.resolvedSessionId) {
      await clearDraftSessionQuery();
    }
    message.success("消息已发送");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "发送消息失败");
  }
}

async function handleReconnect() {
  await connectSocket();
}

watch(
  () => [draftSessionSeed.value?.receiverId, activeSession.value?.session_id, activeSession.value?.receiver_id] as const,
  async ([receiverId, sessionId, activeReceiverId]) => {
    if (!receiverId || !sessionId || isDraftChatSessionId(sessionId) || activeReceiverId !== receiverId) {
      return;
    }

    await clearDraftSessionQuery();
  },
);

useSeoMeta({
  title: "个人中心 - 消息中心",
});
</script>

<template>
  <div class="page-stack">
    <InboxSitePanel v-if="activeTab === 'site'" :categories="mergedCategories" :active-group="activeMessageGroup"
      :items="mergedItems" :pending="mergedPending" :has-more="mergedHasMore"
      @update:active-group="activeMessageGroup = $event" @mark-all-read="handleMarkAllRead()"
      @remove="handleRemoveMessage($event)" @clear-group="handleClearGroup()" @load-more="handleLoadMore()" />

    <InboxChatPanel v-else :sessions="filteredChatSessions" :active-session-id="activeSessionId ?? null"
      :current-session="activeSession" :items="chatMessages" :keyword="chatKeyword" :draft="chatDraft"
      :prepared-session="isPreparedSession"
      :session-pending="sessionPending" :message-pending="messagePending" :socket-status="socketStatus"
      :socket-error="socketError" @update:active-session-id="activeSessionId = $event"
      @update:keyword="chatKeyword = $event" @update:draft="chatDraft = $event" @send="handleSend()"
      @remove-session="removeChatSession($event)" @remove-message="removeChatMessage($event)"
      @reconnect="handleReconnect()" />

  </div>
</template>
