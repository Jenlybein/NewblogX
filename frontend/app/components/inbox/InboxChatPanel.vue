<script setup lang="ts">
import { computed, defineAsyncComponent, ref } from "vue";
import { IconLoader2, IconPhoto, IconPlugConnected, IconPlugConnectedX } from "@tabler/icons-vue";
import { NButton, NInput, NSpace, NTag } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { ChatMessageItem, ChatSessionItem } from "~/types/api";
import { isDraftChatSessionId } from "~/utils/chat";
import { resolveChatMessageContent, type ChatSendType } from "~/utils/chatMessage";
import { formatDateTimeLabel } from "~/utils/format";
import { getRelationLabel } from "~/utils/relation";

import katexCssUrl from "katex/dist/katex.min.css?url";

// 私信面板。
//
// 消息**内容怎么显示**不在这里判断，而是交给 utils/chatMessage 的纯函数：
// 后端把图片消息的 content 包装成了 JSON，这个约定属于"传输格式"，
// 散在模板里用 v-if 处理的话，每加一种消息类型就要改所有渲染点。
//
// Markdown 主题：不再传 github-markdown-css 的 `<link>`。MarkdownRenderSurface
// 的 shadow root 内置自有主题（消费 tokens.css 语义 token），明暗切换跟随站点
// 根类自动生效 —— 暗色下不会再出现"私信白块"。
const ChatMarkdownRenderer = defineAsyncComponent(() => import("~/components/common/MarkdownRenderSurface.vue"));
// katex 只保留基线布局样式（分数/根号），配色由自有主题覆盖。
const markdownSupportStyleHrefs = [katexCssUrl];

const props = defineProps<{
  sessions: ChatSessionItem[];
  activeSessionId: string | null;
  currentSession: ChatSessionItem | null;
  items: ChatMessageItem[];
  keyword: string;
  draft: string;
  /** 草稿的发送类型：1 文本 / 7 Markdown。 */
  draftType: ChatSendType;
  preparedSession?: boolean;
  sessionPending: boolean;
  messagePending: boolean;
  /** 图片上传中（上传完成前不能重复点）。 */
  imagePending: boolean;
  socketStatus: string;
  socketError: string;
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
}>();

const isPreparedSession = computed(() =>
  props.preparedSession ?? (props.currentSession ? isDraftChatSessionId(props.currentSession.session_id) : false),
);

const isMarkdownDraft = computed(() => props.draftType === 7);

/** 预先解析好每条消息的内容，避免在模板里对同一条消息反复解析。 */
const renderedItems = computed(() =>
  props.items.map((item) => ({ item, resolved: resolveChatMessageContent(item) })),
);

const imageInputRef = ref<HTMLInputElement | null>(null);

function openImagePicker() {
  if (props.imagePending) {
    return;
  }
  imageInputRef.value?.click();
}

function handleImagePicked(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) {
    emit("sendImage", file);
  }
  // 清空以便连发同一张图
  input.value = "";
}
</script>

<template>
  <section class="surface-card studio-inbox-card">
    <div class="studio-inbox-grid">
      <aside class="studio-inbox-grid__aside">
        <div class="studio-toolbar studio-toolbar--stack">
          <div>
            <h2 class="section-title">私信会话</h2>
            <p class="muted">HTTP 拉会话，WebSocket 推消息。</p>
          </div>
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
            @click="emit('update:activeSessionId', session.session_id)"
          >
            <AppAvatar :src="session.receiver_avatar" :name="session.receiver_nickname" fallback="私" />
            <div class="min-w-0 flex-1 text-left">
              <div class="flex items-center justify-between gap-2">
                <strong class="truncate">{{ session.receiver_nickname }}</strong>
                <span class="muted text-xs">{{ formatDateTimeLabel(session.last_msg_time || undefined) }}</span>
              </div>
              <div class="mt-1 flex items-center gap-2">
                <span class="muted truncate text-sm">{{ session.last_msg_content || "还没有消息" }}</span>
                <NTag v-if="session.unread_count" size="small" type="error">{{ session.unread_count }}</NTag>
              </div>
            </div>
          </button>
        </div>
      </aside>

      <div class="studio-inbox-grid__main studio-chat-main">
        <div class="studio-toolbar">
          <div v-if="currentSession">
            <div class="flex items-center gap-2">
              <h2 class="section-title">{{ currentSession.receiver_nickname }}</h2>
              <NTag size="small">{{ getRelationLabel(currentSession.relation) }}</NTag>
              <NTag v-if="isPreparedSession" size="small" type="warning">预备会话</NTag>
            </div>
            <p class="muted mt-1">
              {{ isPreparedSession ? "首条消息发出后才会创建真实会话并拉取历史记录。" : "当前会话会自动拉取历史消息，并尝试连接实时推送。" }}
            </p>
          </div>
          <div v-else>
            <h2 class="section-title">选择一个会话</h2>
            <p class="muted mt-1">选中左侧会话后才能查看或发送消息。</p>
          </div>

          <div class="studio-socket-badge" :class="`is-${socketStatus}`">
            <component
              :is="socketStatus === 'connected' ? IconPlugConnected : socketStatus === 'connecting' ? IconLoader2 : IconPlugConnectedX"
              :size="16"
              aria-hidden="true"
              class="shrink-0"
            />
            <span>
              {{
                socketStatus === "connected"
                  ? "实时已连接"
                  : socketStatus === "connecting"
                    ? "实时连接中…"
                    : socketError || "实时未连接"
              }}
            </span>
            <NButton quaternary size="tiny" @click="emit('reconnect')">重连</NButton>
          </div>
        </div>

        <div v-if="currentSession" class="studio-chat-thread">
          <div v-if="renderedItems.length" class="space-y-3">
            <article
              v-for="entry in renderedItems"
              :key="entry.item.id"
              class="studio-chat-bubble"
              :class="{ 'is-self': entry.item.is_self }"
            >
              <div class="studio-chat-bubble__content">
                <img
                  v-if="entry.resolved.kind === 'image'"
                  :src="entry.resolved.imageUrl"
                  alt="图片消息"
                  class="max-h-60 max-w-full rounded-lg object-contain"
                />
                <ChatMarkdownRenderer
                  v-else-if="entry.resolved.kind === 'markdown'"
                  :source="entry.resolved.text"
                  :extra-style-hrefs="markdownSupportStyleHrefs"
                  article-class="markdown-body"
                />
                <p v-else class="leading-7">{{ entry.resolved.text }}</p>
                <div class="studio-list-meta mt-3">
                  <span>{{ formatDateTimeLabel(entry.item.send_time) }}</span>
                  <span>{{ entry.item.is_read ? "已读" : "未读" }}</span>
                </div>
              </div>
              <NButton quaternary size="tiny" @click="emit('removeMessage', entry.item.id)">删除</NButton>
            </article>
          </div>

          <StudioEmptyState
            v-else
            title="这个会话还没有消息"
            :description="messagePending ? '正在拉取消息记录…' : isPreparedSession ? '先发出第一条消息，这里就会切换成真实会话。' : '可以发一段文本、一张图片，或切到 Markdown 发送富文本。'"
          />

          <div class="studio-chat-composer">
            <NInput
              :value="draft"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              :placeholder="isMarkdownDraft ? '输入 Markdown，支持标题、列表、代码块…' : '输入文本消息…'"
              name="chat-draft"
              @update:value="emit('update:draft', $event)"
            />
            <div class="flex flex-wrap items-center justify-between gap-3">
              <NSpace>
                <NButton type="primary" :loading="messagePending" @click="emit('send')">
                  {{ isMarkdownDraft ? "发送 Markdown" : "发送文本" }}
                </NButton>
                <NButton :loading="imagePending" @click="openImagePicker()">
                  <template #icon>
                    <IconPhoto :size="16" />
                  </template>
                  发送图片
                </NButton>
                <NButton
                  quaternary
                  :type="isMarkdownDraft ? 'primary' : 'default'"
                  @click="emit('update:draftType', isMarkdownDraft ? 1 : 7)"
                >
                  {{ isMarkdownDraft ? "Markdown" : "纯文本" }}
                </NButton>
                <NButton v-if="!isPreparedSession" quaternary @click="emit('removeSession', currentSession.session_id)">删除会话</NButton>
              </NSpace>
              <span class="muted text-sm">{{ isPreparedSession ? "这个预备会话只保存在前端，首条消息发出前不会请求后端。" : "文本与 Markdown 走草稿框，图片会先上传到对象存储再发出。" }}</span>
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
        </div>

        <StudioEmptyState
          v-else
          title="还没有选中会话"
          :description="sessionPending ? '正在加载会话列表…' : '从左侧选择一个联系人后，这里会展示消息历史与实时状态。'"
        />
      </div>
    </div>
  </section>
</template>
