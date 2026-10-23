<script setup lang="ts">
import { computed } from "vue";
import { IconLoader2, IconPlugConnected, IconPlugConnectedX } from "@tabler/icons-vue";
import { NButton, NInput, NSpace, NTag } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { ChatMessageItem, ChatSessionItem } from "~/types/api";
import { isDraftChatSessionId } from "~/utils/chat";
import { formatDateTimeLabel } from "~/utils/format";
import { getRelationLabel } from "~/utils/relation";

const props = defineProps<{
  sessions: ChatSessionItem[];
  activeSessionId: string | null;
  currentSession: ChatSessionItem | null;
  items: ChatMessageItem[];
  keyword: string;
  draft: string;
  preparedSession?: boolean;
  sessionPending: boolean;
  messagePending: boolean;
  socketStatus: string;
  socketError: string;
}>();

const emit = defineEmits<{
  "update:activeSessionId": [value: string];
  "update:keyword": [value: string];
  "update:draft": [value: string];
  send: [];
  removeSession: [sessionId: string];
  removeMessage: [id: string];
  reconnect: [];
}>();

const isPreparedSession = computed(() =>
  props.preparedSession ?? (props.currentSession ? isDraftChatSessionId(props.currentSession.session_id) : false),
);
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
          <div v-if="items.length" class="space-y-3">
            <article
              v-for="item in items"
              :key="item.id"
              class="studio-chat-bubble"
              :class="{ 'is-self': item.is_self }"
            >
              <div class="studio-chat-bubble__content">
                <p class="leading-7">{{ item.content }}</p>
                <div class="studio-list-meta mt-3">
                  <span>{{ formatDateTimeLabel(item.send_time) }}</span>
                  <span>{{ item.is_read ? "已读" : "未读" }}</span>
                </div>
              </div>
              <NButton quaternary size="tiny" @click="emit('removeMessage', item.id)">删除</NButton>
            </article>
          </div>

          <StudioEmptyState
            v-else
            title="这个会话还没有消息"
            :description="messagePending ? '正在拉取消息记录…' : isPreparedSession ? '先发出第一条消息，这里就会切换成真实会话。' : '可以先发送一条文本消息验证链路。'"
          />

          <div class="studio-chat-composer">
            <NInput
              :value="draft"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              placeholder="输入文本消息…"
              name="chat-draft"
              @update:value="emit('update:draft', $event)"
            />
            <div class="flex flex-wrap items-center justify-between gap-3">
              <NSpace>
                <NButton type="primary" :loading="messagePending" @click="emit('send')">发送文本</NButton>
                <NButton v-if="!isPreparedSession" quaternary @click="emit('removeSession', currentSession.session_id)">删除会话</NButton>
              </NSpace>
              <span class="muted text-sm">{{ isPreparedSession ? "这个预备会话只保存在前端，首条消息发出前不会请求后端。" : "当前仅接入文本发送，图片与 Markdown 仍待补协议联调。" }}</span>
            </div>
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
