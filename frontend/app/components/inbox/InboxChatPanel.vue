<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, ref, watch } from "vue";
import { IconLoader2, IconPhoto, IconPlugConnectedX } from "@tabler/icons-vue";
import { NButton, NInput, NTag } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { ChatSessionItem } from "~/types/api";
import type { ChatSendType } from "~/utils/chatMessage";
import { fullChatTimestamp, type ChatTimelineEntry } from "~/utils/chatTimeline";
import { formatDateTimeLabel } from "~/utils/format";
import { getRelationLabel } from "~/utils/relation";

import katexCssUrl from "katex/dist/katex.min.css?url";

/**
 * 私信面板（2026-11-05 重构）。
 *
 * 用户可见文案里不再出现任何实现说明：通道协议、存储位置、草稿态这些词
 * 只属于代码与文档。消息区按自然日分隔、同人 2 分钟内合并，完整时间戳
 * 放在 title 里，不再逐条啰嗦。
 *
 * 面板**只画递到手上的结果**：升序消息、日期分隔、合并分组、气泡内容与回执
 * 都由 `useInboxCenter`（唯一持有者）算好，经 `timeline` 传进来。这里只剩
 * DOM 滚动补偿与窄屏二选一，不再有第二份日期键 / 排序 / 分组。
 */
const ChatMarkdownRenderer = defineAsyncComponent(() => import("~/components/common/MarkdownRenderSurface.vue"));
const markdownSupportStyleHrefs = [katexCssUrl];

const props = defineProps<{
  sessions: ChatSessionItem[];
  activeSessionId: string | null;
  currentSession: ChatSessionItem | null;
  /** 已切好的时间线（日期分隔条 + 合并组 + 已解析内容 + 回执）。 */
  timeline: ChatTimelineEntry[];
  keyword: string;
  draft: string;
  draftType: ChatSendType;
  sessionPending: boolean;
  messagePending: boolean;
  imagePending: boolean;
  /** 是否还有更早的消息可上滑加载。 */
  hasMoreHistory: boolean;
  olderPending: boolean;
  socketStatus: string;
  socketError: string;
  sendError?: string;
}>();

const emit = defineEmits<{
  "update:activeSessionId": [value: string];
  "update:keyword": [value: string];
  "update:draft": [value: string];
  "update:draftType": [value: ChatSendType];
  send: [];
  sendImage: [file: File];
  removeSession: [sessionId: string];
  removeMessage: [id: string];
  reconnect: [];
  retry: [];
  loadOlder: [];
}>();

const isMarkdownDraft = computed(() => props.draftType === 7);

/** 连接状态条只在异常时出现；健康连接不渲染任何状态（Q3）。 */
const showSocketBadge = computed(
  () => props.socketStatus !== "connected" || Boolean(props.socketError),
);
const socketBadgeLabel = computed(() => {
  if (props.socketStatus === "connecting") return "实时连接中…";
  return props.socketError || "实时未连接";
});

/**
 * 窄屏二选一：<1024px 时列表与聊天窗不能同时显示。宽屏该状态不生效。
 */
const narrowPane = ref<"list" | "chat">(props.currentSession ? "chat" : "list");
const narrowPaneClass = computed(() => (narrowPane.value === "chat" ? "is-narrow-chat" : "is-narrow-list"));

watch(
  () => props.currentSession?.session_id ?? null,
  (sessionId) => {
    narrowPane.value = sessionId ? "chat" : "list";
  },
);

function selectSession(sessionId: string) {
  narrowPane.value = "chat";
  emit("update:activeSessionId", sessionId);
}

function backToSessionList() {
  narrowPane.value = "list";
}

const scrollerRef = ref<HTMLDivElement | null>(null);
const imageInputRef = ref<HTMLInputElement | null>(null);
let stickToBottom = true;
let awaitingOlder = false;
let olderAnchorBottom = 0;

function scrollToBottom() {
  const el = scrollerRef.value;
  if (el) el.scrollTop = el.scrollHeight;
}

function onThreadScroll() {
  const el = scrollerRef.value;
  if (!el) return;
  stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= 48;
  if (
    el.scrollTop <= 40 &&
    props.hasMoreHistory &&
    !props.olderPending &&
    !awaitingOlder &&
    !props.messagePending
  ) {
    olderAnchorBottom = el.scrollHeight - el.scrollTop;
    awaitingOlder = true;
    emit("loadOlder");
  }
}

/** 上滑加载更早时锁滚动位置：用 scrollHeight 差值把视口钉回原来的那条消息。 */
watch(
  () => [props.timeline.length, props.timeline[0]?.key] as const,
  async () => {
    await nextTick();
    if (awaitingOlder) {
      awaitingOlder = false;
      const el = scrollerRef.value;
      if (el) el.scrollTop = Math.max(0, el.scrollHeight - olderAnchorBottom);
      return;
    }
    if (stickToBottom) scrollToBottom();
  },
);

watch(
  () => props.activeSessionId,
  async () => {
    stickToBottom = true;
    awaitingOlder = false;
    await nextTick();
    scrollToBottom();
  },
);

function openImagePicker() {
  if (props.imagePending) return;
  imageInputRef.value?.click();
}

function handleImagePicked(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) emit("sendImage", file);
  input.value = "";
}
</script>

<template>
  <section class="surface-card studio-inbox-card">
    <div class="studio-inbox-grid" :class="narrowPaneClass">
      <aside class="studio-inbox-grid__aside">
        <div class="studio-toolbar studio-toolbar--stack">
          <h2 class="section-title">私信会话</h2>
          <NInput
            :value="keyword"
            placeholder="搜索联系人或消息…"
            clearable
            name="chat-search"
            autocomplete="off"
            @update:value="emit('update:keyword', $event)"
          />
        </div>

        <div class="space-y-2">
          <button
            v-for="session in sessions"
            :key="session.session_id"
            type="button"
            class="studio-chat-session"
            :class="{ 'is-active': activeSessionId === session.session_id }"
            @click="selectSession(session.session_id)"
          >
            <AppAvatar :src="session.receiver_avatar" :name="session.receiver_nickname" fallback="私" />
            <div class="min-w-0 flex-1 text-left">
              <div class="flex items-center justify-between gap-2">
                <strong class="truncate">{{ session.receiver_nickname }}</strong>
                <span class="muted text-xs">{{ formatDateTimeLabel(session.last_msg_time || undefined) }}</span>
              </div>
              <div class="mt-1 flex items-center gap-2">
                <span class="muted truncate text-sm">{{ session.last_msg_content }}</span>
                <NTag v-if="session.unread_count" size="small" type="error">{{ session.unread_count }}</NTag>
              </div>
            </div>
          </button>
        </div>
      </aside>

      <div class="studio-inbox-grid__main studio-chat-main">
        <button type="button" class="studio-chat-back" @click="backToSessionList()">← 会话列表</button>

        <div class="studio-toolbar">
          <div v-if="currentSession" class="flex items-center gap-2">
            <AppAvatar :src="currentSession.receiver_avatar" :name="currentSession.receiver_nickname" fallback="私" />
            <h2 class="section-title">{{ currentSession.receiver_nickname }}</h2>
            <NTag size="small">{{ getRelationLabel(currentSession.relation) }}</NTag>
          </div>
          <div v-else />

          <div v-if="showSocketBadge" class="studio-socket-badge" :class="`is-${socketStatus}`">
            <component
              :is="socketStatus === 'connecting' ? IconLoader2 : IconPlugConnectedX"
              :size="16"
              aria-hidden="true"
              class="shrink-0"
            />
            <span>{{ socketBadgeLabel }}</span>
            <NButton quaternary size="tiny" @click="emit('reconnect')">重连</NButton>
          </div>
        </div>

        <template v-if="currentSession">
          <div ref="scrollerRef" class="studio-chat-thread studio-chat-scroller" @scroll.passive="onThreadScroll()">
            <div v-if="olderPending" class="studio-chat-load-older">正在拉取更早的消息…</div>
            <div v-else-if="!hasMoreHistory && timeline.length" class="studio-chat-load-older">没有更早的消息</div>

            <template v-for="entry in timeline" :key="entry.key">
              <div v-if="entry.kind === 'date'" class="studio-chat-date-separator">{{ entry.label }}</div>
              <div
                v-else
                class="studio-chat-group"
                :class="{ 'is-self': entry.isSelf }"
              >
                <article
                  v-for="bubble in entry.entries"
                  :key="bubble.item.id"
                  class="studio-chat-bubble"
                  :class="{ 'is-self': entry.isSelf }"
                >
                  <AppAvatar
                    v-if="!entry.isSelf"
                    class="studio-chat-bubble__avatar"
                    :src="currentSession.receiver_avatar"
                    :name="currentSession.receiver_nickname"
                    fallback="私"
                    :size="28"
                  />
                  <div class="studio-chat-bubble__content" :title="fullChatTimestamp(bubble.item.send_time)">
                    <img
                      v-if="bubble.resolved.kind === 'image'"
                      :src="bubble.resolved.imageUrl"
                      alt="图片消息"
                      class="max-h-60 max-w-full rounded-field object-contain"
                    />
                    <ChatMarkdownRenderer
                      v-else-if="bubble.resolved.kind === 'markdown'"
                      :source="bubble.resolved.text"
                      :extra-style-hrefs="markdownSupportStyleHrefs"
                      article-class="markdown-body"
                    />
                    <p v-else class="leading-7">{{ bubble.resolved.text }}</p>
                    <div class="studio-chat-bubble__meta">
                      <time>{{ formatDateTimeLabel(bubble.item.send_time) }}</time>
                      <span v-if="bubble.item.is_self" class="studio-chat-receipt">{{ bubble.receipt }}</span>
                    </div>
                  </div>
                  <NButton
                    quaternary
                    size="tiny"
                    class="studio-chat-bubble__delete"
                    aria-label="删除这条消息"
                    @click="emit('removeMessage', bubble.item.id)"
                  >删除</NButton>
                </article>
              </div>
            </template>

            <div v-if="messagePending && !timeline.length" class="studio-chat-load-older">正在拉取消息记录…</div>
          </div>

          <div v-if="sendError" class="studio-chat-error" role="alert">
            <span>{{ sendError }}</span>
            <NButton quaternary size="tiny" @click="emit('retry')">重试</NButton>
          </div>

          <div class="studio-chat-composer">
            <NInput
              :value="draft"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 5 }"
              placeholder="输入消息…"
              name="chat-draft"
              @update:value="emit('update:draft', $event)"
            />
            <div class="studio-chat-composer__actions">
              <div class="flex flex-wrap items-center gap-2">
                <NButton type="primary" :loading="messagePending" @click="emit('send')">
                  {{ isMarkdownDraft ? "发送 Markdown" : "发送文本" }}
                </NButton>
                <NButton :loading="imagePending" @click="openImagePicker()">
                  <template #icon><IconPhoto :size="16" /></template>
                  发送图片
                </NButton>
                <NButton
                  quaternary
                  :type="isMarkdownDraft ? 'primary' : 'default'"
                  @click="emit('update:draftType', isMarkdownDraft ? 1 : 7)"
                >{{ isMarkdownDraft ? "Markdown" : "纯文本" }}</NButton>
                <NButton quaternary @click="emit('removeSession', currentSession.session_id)">删除会话</NButton>
              </div>
            </div>
            <input
              ref="imageInputRef"
              type="file"
              accept="image/*"
              class="hidden"
              name="chat-image"
              @change="handleImagePicked"
            />
          </div>
        </template>
      </div>
    </div>
  </section>
</template>
