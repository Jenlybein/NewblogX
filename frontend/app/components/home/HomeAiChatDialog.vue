<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { IconLoader2, IconTrash } from "@tabler/icons-vue";
import { NButton, NCard, NInput, NModal } from "naive-ui";
import githubMarkdownCssUrl from "github-markdown-css/github-markdown-light.css?url";
import katexCssUrl from "katex/dist/katex.min.css?url";
import highlightCssUrl from "highlight.js/styles/github.css?url";
import AppAvatar from "~/components/common/AppAvatar.vue";
import MarkdownRenderSurface from "~/components/common/MarkdownRenderSurface.vue";
import { streamAiAssistantReply, type AiConversationMessage } from "~/services/ai";
import { useAuthStore } from "~/stores/auth";
import { useUiStore } from "~/stores/ui";
import type { SiteAiInfo } from "~/types/api";
import { resolveAvatarUrl } from "~/utils/avatar";

type ChatRole = "user" | "assistant";

interface ChatMessage {
  id: string;
  role: ChatRole;
  content: string;
  streaming?: boolean;
  error?: boolean;
}

const props = defineProps<{
  show: boolean;
  aiInfo: SiteAiInfo | null;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
}>();

const authStore = useAuthStore();
const uiStore = useUiStore();
const draft = ref("");
const pending = ref(false);
const messages = ref<ChatMessage[]>([]);
const threadViewportRef = ref<HTMLElement | null>(null);
const activeRequest = ref<AbortController | null>(null);

const aiDisplayName = computed(() => props.aiInfo?.nickname || "BlogX 助手");
const aiAvatarUrl = computed(() => resolveAvatarUrl(props.aiInfo?.avatar || ""));
const markdownThemeHref = computed(() => githubMarkdownCssUrl);
const markdownSupportStyleHrefs = [katexCssUrl, highlightCssUrl];
const canSend = computed(() => !!draft.value.trim() && !pending.value);

function createMessage(role: ChatRole, content: string, extra: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: `${role}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    role,
    content,
    ...extra,
  };
}

async function scrollToBottom() {
  await nextTick();
  threadViewportRef.value?.scrollTo({
    top: threadViewportRef.value.scrollHeight,
    behavior: "smooth",
  });
}

function updateDialogVisibility(show: boolean) {
  if (!show) {
    activeRequest.value?.abort();
  }
  emit("update:show", show);
}

function clearConversation() {
  activeRequest.value?.abort();
  pending.value = false;
  messages.value = [];
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing) {
    return;
  }

  event.preventDefault();
  if (canSend.value) {
    void handleSend();
  }
}

async function handleSend() {
  const content = draft.value.trim();
  if (!content || pending.value) {
    return;
  }

  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    updateDialogVisibility(false);
    uiStore.openAuthModal();
    return;
  }

  const userMessage = createMessage("user", content);
  const assistantMessage = createMessage("assistant", "", {
    streaming: true,
  });
  const history: AiConversationMessage[] = messages.value.map((item) => ({
    role: item.role,
    content: item.content,
  }));

  messages.value = [...messages.value, userMessage, assistantMessage];
  draft.value = "";
  pending.value = true;
  const controller = new AbortController();
  activeRequest.value = controller;
  await scrollToBottom();

  try {
    await streamAiAssistantReply(content, {
      history,
      signal: controller.signal,
      onChunk: (chunk) => {
        assistantMessage.content += chunk;
        void scrollToBottom();
      },
    });

    if (!assistantMessage.content.trim()) {
      assistantMessage.content = "AI 暂时没有返回可显示的内容。";
    }
  } catch (error) {
    if (controller.signal.aborted) {
      assistantMessage.content = assistantMessage.content || "本次回答已停止。";
    } else {
      assistantMessage.error = true;
      assistantMessage.content = assistantMessage.content || (error instanceof Error ? error.message : "AI 对话失败");
    }
  } finally {
    assistantMessage.streaming = false;
    pending.value = false;
    if (activeRequest.value === controller) {
      activeRequest.value = null;
    }
    await scrollToBottom();
  }
}

function stopReply() {
  activeRequest.value?.abort();
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      void scrollToBottom();
    }
  },
);
</script>

<template>
  <NModal :show="show" :mask-closable="!pending" @update:show="updateDialogVisibility">
    <div class="mx-auto w-full max-w-[980px] px-4 py-6">
      <NCard closable :bordered="false" class="surface-card surface-card--strong ai-chat-dialog"
        @close="updateDialogVisibility(false)">
        <div class="ai-chat-dialog__header">
          <div class="flex items-center gap-4">
            <AppAvatar :size="56" :src="aiAvatarUrl" :name="aiDisplayName" fallback="AI" />
            <div>
              <div
                class="inline-flex rounded-full border border-teal-200/70 bg-teal-50 px-3 py-1 text-xs font-semibold tracking-[0.2em] text-teal-700">
                AI CHAT
              </div>
              <div class="mt-3 text-xl font-semibold text-slate-900">
                {{ aiDisplayName }}
              </div>
              <p class="mt-2 max-w-[620px] text-sm leading-7 text-slate-600">
                {{ aiInfo?.abstract || "这里支持直接提问。AI 回复会按 Markdown 渲染，便于阅读代码块、列表和结构化说明。" }}
              </p>
            </div>
          </div>

          <div class="flex items-center gap-3">
            <NButton quaternary :disabled="!messages.length && !pending" @click="clearConversation">
              <template #icon>
                <IconTrash :size="16" />
              </template>
              清空对话
            </NButton>
          </div>
        </div>

        <div class="ai-chat-dialog__body">
          <div ref="threadViewportRef" class="ai-chat-thread">
            <div v-if="messages.length" class="space-y-4">
              <article v-for="message in messages" :key="message.id" class="ai-chat-bubble" :class="{
                'is-user': message.role === 'user',
                'is-assistant': message.role === 'assistant',
                'is-error': message.error,
              }">
                <div class="ai-chat-bubble__meta">
                  <AppAvatar :size="36" :src="message.role === 'assistant' ? aiAvatarUrl : authStore.profileAvatar"
                    :name="message.role === 'assistant' ? aiDisplayName : authStore.profileName"
                    :fallback="message.role === 'assistant' ? 'AI' : '我'" />
                  <div class="text-sm font-semibold text-slate-900">
                    {{ message.role === "assistant" ? aiDisplayName : authStore.profileName }}
                  </div>
                </div>

                <div class="ai-chat-bubble__content">
                  <div v-if="message.role === 'user'"
                    class="whitespace-pre-wrap break-words text-sm leading-7 text-slate-700">
                    {{ message.content }}
                  </div>

                  <div v-else>
                    <div v-if="message.streaming && !message.content"
                      class="flex items-center gap-2 text-sm text-slate-500">
                      <IconLoader2 :size="16" class="animate-spin" />
                      AI 正在整理回答…
                    </div>

                    <MarkdownRenderSurface v-else :source="message.content" :theme-href="markdownThemeHref"
                      :extra-style-hrefs="markdownSupportStyleHrefs" article-class="markdown-body" />
                  </div>
                </div>
              </article>
            </div>

            <div v-else class="ai-chat-empty">
              <div
                class="inline-flex rounded-full border border-dashed border-slate-300 px-3 py-1 text-xs font-semibold tracking-[0.18em] text-slate-500">
                READY
              </div>
              <div class="mt-4 text-lg font-semibold text-slate-900">可以开始提问了</div>
              <p class="mt-3 max-w-[420px] text-sm leading-7 text-slate-600">
                你可以直接问文章选题、站内内容搜索、写作改进建议或技术问题。
              </p>
            </div>
          </div>

          <div class="ai-chat-composer">
            <NInput :value="draft" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" name="ai-chat-draft"
              autocomplete="off" aria-label="AI 对话输入框" placeholder="输入你想让 AI 处理的问题，按 Enter 发送，Shift + Enter 换行"
              @update:value="draft = $event" @keydown="handleComposerKeydown" />
            <div class="flex flex-wrap items-center justify-end gap-3">
              <NButton v-if="pending" quaternary @click="stopReply">停止回答</NButton>
              <NButton type="primary" :disabled="!canSend" :loading="pending" @click="handleSend">
                发送提问
              </NButton>
            </div>
          </div>
        </div>
      </NCard>
    </div>
  </NModal>
</template>

<style scoped>
.ai-chat-dialog {
  overflow: hidden;
  background: #ffffff;
}

:deep(.ai-chat-dialog .n-card),
:deep(.ai-chat-dialog .n-card-header),
:deep(.ai-chat-dialog .n-card__content) {
  background: #ffffff !important;
  color: rgb(15 23 42 / 1);
}

.ai-chat-dialog__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  padding-bottom: 1.25rem;
  border-bottom: 1px solid rgb(226 232 240 / 0.8);
}

.ai-chat-dialog__body {
  display: grid;
  gap: 1rem;
  padding-top: 1.25rem;
}

.ai-chat-thread {
  max-height: min(62vh, 680px);
  overflow-y: auto;
  overscroll-behavior: contain;
  border: 1px solid rgb(226 232 240 / 0.8);
  border-radius: 1.5rem;
  background: #ffffff;
  padding: 1rem;
}

.ai-chat-bubble {
  border: 1px solid rgb(226 232 240 / 0.8);
  border-radius: 1.25rem;
  background: rgb(255 255 255 / 0.9);
  padding: 1rem;
  box-shadow: 0 18px 44px rgb(15 23 42 / 0.06);
}

.ai-chat-bubble.is-user {
  margin-left: auto;
  max-width: min(100%, 720px);
  background: rgb(244 250 255 / 0.98);
}

.ai-chat-bubble.is-assistant {
  max-width: min(100%, 780px);
}

.ai-chat-bubble.is-error {
  border-color: rgb(252 165 165 / 0.8);
  background: rgb(254 242 242 / 0.96);
}

.ai-chat-bubble__meta {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.875rem;
}

.ai-chat-bubble__content {
  overflow: hidden;
}

.ai-chat-empty {
  display: flex;
  min-height: 320px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.ai-chat-composer {
  display: grid;
  gap: 0.875rem;
  border: 1px solid rgb(226 232 240 / 0.85);
  border-radius: 1.5rem;
  background: #ffffff;
  padding: 1rem;
}

:deep(.ai-chat-bubble__content .markdown-body) {
  background: transparent;
  color: rgb(51 65 85 / 1);
  padding: 0;
  font-size: 0.92rem;
}

:deep(.ai-chat-bubble__content .markdown-body pre) {
  overflow-x: auto;
  border-radius: 0.875rem;
}

:deep(.ai-chat-composer .n-input),
:deep(.ai-chat-composer .n-input-wrapper) {
  background: #ffffff !important;
}

:deep(.ai-chat-composer textarea),
:deep(.ai-chat-composer input) {
  color: rgb(15 23 42 / 1) !important;
  caret-color: rgb(15 118 110 / 1);
}

:deep(.ai-chat-composer textarea::placeholder),
:deep(.ai-chat-composer input::placeholder) {
  color: rgb(148 163 184 / 1) !important;
}

@media (max-width: 768px) {
  .ai-chat-dialog__header {
    flex-direction: column;
  }

  .ai-chat-thread {
    max-height: 54vh;
  }
}
</style>
