<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AnimatePresence, Motion } from "motion-v";
// markdown 渲染走自有 token 主题（明暗自动跟随），外部只留 katex 的数学字体样式。
import katexCssUrl from "katex/dist/katex.min.css?url";

import MarkdownRenderSurface from "~/components/common/MarkdownRenderSurface.vue";
import { useJellyPanel } from "~/composables/useJellyPanel";
import type { JellyAssistantApi } from "~/composables/useJellyAssistant";
import type { SiteAiInfo } from "~/types/api";
import { brandAiName, brandAssetUrl } from "~/utils/brand";
import { MOTION_DURATIONS_MS, MOTION_EASE, motionSeconds } from "~/utils/motion";

// 捏捏张开的 AI 面板（票 #10）。
//
// 形态：桌面从右下角生长（transform-origin: 100% 100%，约 420px 宽，**非遮罩**，
// 可以一边读文章一边问）；手机是 78vh 底部抽屉。面板打开时，角色（#9 的 JellyLauncher）
// 被压成面板右下角的把手 —— 两者共享同一个 useJellyAssistant 实例，所以这里的开关
// 就是 `assistant.isOpen`，绝不另开一份 ref。
//
// 边界：真请求走既有 services/ai.ts（流式/6 轮历史/401 重试都在那层），本组件只负责
// 把 useJellyPanel 的状态渲染出来 + 把交互翻译成动作。
//
// SSR：AnimatePresence 的退场与 initial 内联会在没有 JS 时把内容藏掉（#3 报告的坑），
// 所以整块必须客户端挂载后才渲染（布局里还包了一层 ClientOnly 做双保险）。

const props = defineProps<{
  assistant: JellyAssistantApi;
  aiInfo?: SiteAiInfo | null;
}>();

const panel = useJellyPanel({ assistant: props.assistant });
const {
  isOpen,
  isAvailable,
  messages,
  draft,
  pending,
  canSend,
  showEmptyState,
  quickQuestions,
  send,
  stop,
  clear,
  fillDraft,
  closePanel,
  handleComposerKeydown,
} = panel;

const assistantName = computed(() => brandAiName(props.aiInfo?.nickname));
const assistantTagline = computed(
  () => props.aiInfo?.abstract || "问文章、问写法、问站内内容都可以；回答支持 Markdown。",
);
const glyphUrl = brandAssetUrl("/", "flat");
const markdownExtraStyleHrefs = [katexCssUrl];

// 客户端挂载守卫：AnimatePresence / 视口测量都只在浏览器里有意义。
const hydrated = ref(false);
const viewport = ref({ width: 0, height: 0 });
const threadRef = ref<HTMLElement | null>(null);
const composerRef = ref<HTMLTextAreaElement | null>(null);

const JELLY_SIZE = 56;
const isMobile = computed(() => viewport.value.width > 0 && viewport.value.width <= 640);

function syncViewport() {
  if (typeof window === "undefined") return;
  viewport.value = { width: window.innerWidth, height: window.innerHeight };
}

/**
 * 面板定位：
 * - 桌面：右下角生长，右边/下边贴着手柄（捏捏当前位置的右下角），左边界会被钳进视口；
 * - 手机：底部抽屉，不挤压正文。
 */
const panelStyle = computed(() => {
  if (isMobile.value) {
    return {
      left: "0",
      right: "0",
      bottom: "0",
      width: "100%",
      height: "78vh",
      transformOrigin: "100% 100%",
    };
  }

  const vw = viewport.value.width || 1024;
  const vh = viewport.value.height || 768;
  const width = Math.min(420, vw - 24);
  const launcherRight = props.assistant.position.value.x + JELLY_SIZE;
  // 面板底边落在捏捏上沿：角色本体（打开时压成把手）就是面板右下角的那一角，
  // 既不会盖住输入区，视觉上又是"面板从它身上长出来"。
  const launcherTop = props.assistant.position.value.y;
  // 面板右边贴着捏捏右边，但整块必须留在视口内（拖到左边时也不会溢出屏幕外）。
  const right = Math.min(Math.max(vw - launcherRight, 8), Math.max(8, vw - width - 8));
  const bottom = Math.max(8, vh - launcherTop);
  const maxHeight = Math.max(240, Math.min(640, launcherTop - 8));

  return {
    right: `${right}px`,
    bottom: `${bottom}px`,
    width: `${width}px`,
    maxHeight: `${maxHeight}px`,
    transformOrigin: "100% 100%",
  };
});

const panelMotion = computed(() => {
  const duration = motionSeconds(props.assistant.isReducedMotion.value ? 0 : MOTION_DURATIONS_MS.base);
  return {
    initial: { opacity: 0, scale: 0.92, y: 16 },
    animate: { opacity: 1, scale: 1, y: 0 },
    exit: { opacity: 0, scale: 0.92, y: 12 },
    transition: { duration, ease: MOTION_EASE },
  };
});

async function scrollToBottom() {
  await nextTick();
  const thread = threadRef.value;
  if (thread) {
    thread.scrollTop = thread.scrollHeight;
  }
}

watch(
  () => (messages.value.length ? messages.value[messages.value.length - 1]?.content.length ?? 0 : 0),
  () => {
    void scrollToBottom();
  },
);

watch(isOpen, (open) => {
  if (open) {
    void nextTick(() => composerRef.value?.focus());
  }
});

onMounted(() => {
  hydrated.value = true;
  syncViewport();
  window.addEventListener("resize", syncViewport);
  if (isOpen.value) {
    void nextTick(() => composerRef.value?.focus());
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", syncViewport);
});
</script>

<template>
  <AnimatePresence v-if="hydrated">
    <Motion
      v-if="isOpen"
      key="jelly-panel"
      class="jelly-panel"
      :class="{ 'jelly-panel--mobile': isMobile }"
      :style="panelStyle"
      :initial="panelMotion.initial"
      :animate="panelMotion.animate"
      :exit="panelMotion.exit"
      :transition="panelMotion.transition"
      role="dialog"
      aria-modal="false"
      :aria-label="`${assistantName} AI 对话面板`"
      @keydown.esc="closePanel()"
    >
      <header class="jelly-panel__head">
        <img class="jelly-panel__glyph" :src="glyphUrl" alt="" aria-hidden="true" width="28" height="28">
        <div class="jelly-panel__title-block">
          <div class="jelly-panel__title">{{ assistantName }}</div>
          <p class="jelly-panel__tagline">{{ assistantTagline }}</p>
        </div>
        <div class="jelly-panel__tools">
          <button
            type="button"
            class="jelly-panel__clear"
            :disabled="!messages.length && !pending"
            @click="clear()"
          >
            清空对话
          </button>
          <button type="button" class="jelly-panel__close" aria-label="关闭捏捏面板" @click="closePanel()">
            ×
          </button>
        </div>
      </header>

      <div ref="threadRef" class="jelly-panel__thread" role="log" aria-live="polite">
        <template v-if="messages.length">
          <article
            v-for="message in messages"
            :key="message.id"
            class="jelly-panel__message"
            :class="{
              'jelly-panel__message--user': message.role === 'user',
              'jelly-panel__message--assistant': message.role === 'assistant',
              'jelly-panel__message--error': message.errorKind,
            }"
          >
            <div v-if="message.role === 'user'" class="jelly-panel__bubble jelly-panel__bubble--user">
              {{ message.content }}
            </div>
            <div v-else class="jelly-panel__bubble jelly-panel__bubble--assistant">
              <span v-if="message.streaming && !message.content" class="jelly-panel__thinking">
                正在整理回答…
              </span>
              <MarkdownRenderSurface
                v-else
                :source="message.content"
                        :extra-style-hrefs="markdownExtraStyleHrefs"
                 article-class="jelly-panel__markdown markdown-body"
              />
              <span v-if="message.streaming" class="jelly-panel__caret" aria-hidden="true" />
              <span v-if="message.stopped" class="jelly-panel__stopped">已停止</span>
            </div>
          </article>
        </template>

        <div v-else-if="showEmptyState" class="jelly-panel__empty">
          <p class="jelly-panel__empty-hint">可以这样问：</p>
          <div class="jelly-panel__chips">
            <button
              v-for="question in quickQuestions"
              :key="question"
              type="button"
              class="jelly-panel__chip"
              @click="fillDraft(question)"
            >
              {{ question }}
            </button>
          </div>
        </div>
      </div>

      <div class="jelly-panel__composer">
        <textarea
          ref="composerRef"
          v-model="draft"
          class="jelly-panel__input"
          rows="3"
          name="jelly-ai-draft"
          autocomplete="off"
          aria-label="向捏捏提问"
          :placeholder="isAvailable ? '问点什么…（Enter 发送，Shift + Enter 换行）' : 'AI 还开着小差，暂时不能提问'"
          @keydown="handleComposerKeydown"
        />
        <div class="jelly-panel__actions">
          <button v-if="pending" type="button" class="jelly-panel__stop" @click="stop()">停止回答</button>
          <button
            type="button"
            class="jelly-panel__send"
            :disabled="!canSend"
            @click="send()"
          >
            {{ pending ? "回答中…" : "发送" }}
          </button>
        </div>
      </div>
    </Motion>
  </AnimatePresence>
</template>

<style scoped>
/* 颜色只用语义 token（check:design-tokens 会卡住裸色值）；圆角归零。 */
.jelly-panel {
  position: fixed;
  /* 低于角色(#40)：打开时捏捏仍是面板右下角的可见把手。 */
  z-index: 39;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--color-line-strong);
  border-radius: 0;
  background: var(--color-surface);
  color: var(--color-ink);
  box-shadow: 0 24px 60px var(--color-shadow);
  font-family: var(--font-sans);
}

.jelly-panel--mobile {
  border-left: 0;
  border-right: 0;
  border-bottom: 0;
}

.jelly-panel__head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--color-line);
}

.jelly-panel__glyph {
  display: block;
  flex-shrink: 0;
}

.jelly-panel__title-block {
  min-width: 0;
  flex: 1;
}

.jelly-panel__title {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: var(--type-tracking-dense);
}

.jelly-panel__tagline {
  margin: 2px 0 0;
  color: var(--color-ink-muted);
  font-size: 12px;
  line-height: 1.5;
}

.jelly-panel__tools {
  display: flex;
  align-items: center;
  gap: 8px;
}

.jelly-panel__clear {
  border: 1px solid var(--color-line);
  border-radius: 0;
  padding: 5px 9px;
  background: transparent;
  color: var(--color-ink-muted);
  font-size: 12px;
  cursor: pointer;
  transition: color var(--motion-fast) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease);
}

.jelly-panel__clear:not(:disabled):hover {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
}

.jelly-panel__clear:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.jelly-panel__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--color-ink);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}

.jelly-panel__close:hover {
  color: var(--color-focus);
}

.jelly-panel__thread {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.jelly-panel__message {
  display: flex;
  flex-direction: column;
}

.jelly-panel__message--user {
  align-items: flex-end;
}

.jelly-panel__bubble {
  max-width: 88%;
  padding: 10px 12px;
  border: 1px solid var(--color-line);
  border-radius: 0;
  background: var(--color-surface-raised);
  font-size: 13px;
  line-height: 1.7;
}

.jelly-panel__bubble--user {
  background: var(--color-line-faint);
  white-space: pre-wrap;
  word-break: break-word;
}

.jelly-panel__message--error .jelly-panel__bubble--assistant {
  border-color: var(--color-danger);
  background: var(--color-bg-subtle);
  color: var(--color-danger-ink);
}

.jelly-panel__thinking {
  color: var(--color-ink-muted);
}

.jelly-panel__caret {
  display: inline-block;
  width: 8px;
  height: 14px;
  margin-left: 2px;
  vertical-align: text-bottom;
  background: var(--color-focus);
  animation: jelly-caret-blink 1s steps(2, start) infinite;
}

@keyframes jelly-caret-blink {
  50% {
    opacity: 0;
  }
}

.jelly-panel__stopped {
  display: inline-block;
  margin-top: 6px;
  color: var(--color-ink-muted);
  font-size: 11px;
  letter-spacing: 0.08em;
}

.jelly-panel__empty {
  margin: auto 0;
  padding: 16px 4px;
}

.jelly-panel__empty-hint {
  margin: 0 0 10px;
  color: var(--color-ink-muted);
  font-size: 12px;
}

.jelly-panel__chips {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.jelly-panel__chip {
  border: 1px solid var(--color-line);
  border-radius: 0;
  padding: 9px 11px;
  background: var(--color-surface-raised);
  color: var(--color-ink);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  transition: border-color var(--motion-fast) var(--motion-ease), color var(--motion-fast) var(--motion-ease);
}

.jelly-panel__chip:hover {
  border-color: var(--color-line-strong);
  color: var(--color-focus);
}

.jelly-panel__composer {
  border-top: 1px solid var(--color-line);
  padding: 12px 16px 14px;
  background: var(--color-surface);
}

.jelly-panel__input {
  display: block;
  width: 100%;
  box-sizing: border-box;
  resize: vertical;
  border: 1px solid var(--color-line);
  border-radius: 0;
  padding: 10px 12px;
  background: var(--color-surface-raised);
  color: var(--color-ink);
  font: inherit;
  font-size: 13px;
  line-height: 1.6;
}

.jelly-panel__input:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 1px;
}

.jelly-panel__actions {
  margin-top: 10px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.jelly-panel__stop {
  border: 1px solid var(--color-line);
  border-radius: 0;
  padding: 8px 12px;
  background: transparent;
  color: var(--color-ink-muted);
  font-size: 12px;
  cursor: pointer;
}

.jelly-panel__stop:hover {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
}

.jelly-panel__send {
  border: 1px solid var(--color-focus);
  border-radius: 0;
  padding: 8px 16px;
  background: var(--color-focus);
  color: var(--color-surface);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.jelly-panel__send:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

/* 面板里的 Markdown 回答不要自带白底（暗色下会变成白块）。 */
:deep(.jelly-panel__markdown) {
  background: transparent;
  color: inherit;
  padding: 0;
  font-size: 13px;
}

:deep(.jelly-panel__markdown pre) {
  overflow-x: auto;
}

@media (max-width: 640px) {
  .jelly-panel {
    border-left: 0;
    border-right: 0;
    border-bottom: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .jelly-panel__caret {
    animation: none;
  }
}
</style>
