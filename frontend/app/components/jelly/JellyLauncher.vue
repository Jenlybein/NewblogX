<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

import { useJellyAssistant } from "~/composables/useJellyAssistant";
import type { JellyAssistantApi } from "~/composables/useJellyAssistant";
import { brandAiName, brandAssetUrl } from "~/utils/brand";

// 「捏捏」的外观组件（票 #9）。
//
// 这个文件**不含状态**：六态、可用性、气泡、位置全部由 useJellyAssistant 决定
// （它是唯一真相源，#10 的 AI 面板消费同一实例）。这里只做两件事：
//   1. 把状态渲染成外观（class + 无障碍属性 + 位置）；
//   2. 把指针/键盘事件翻译成 composable 的动作（悬停、按压、拖拽、触发）。
//
// 形状来源是 #4 交付的扁平矢量果冻方块（`brand/jelly-flat.svg`），颜色全部来自
// 语义 token（--color-*），本文件不出现任何裸色值（由漂移门禁卡住）。

// 状态实例：布局会传入页面级共享实例（#10 的 AI 面板消费同一份）；
// 不传时退回自己创建一份，让组件在独立使用/单测里也能跑。
const props = defineProps<{ assistant?: JellyAssistantApi }>();

const assistant = props.assistant ?? useJellyAssistant();
const {
  state,
  isAvailable,
  isOpen,
  isReducedMotion,
  shouldRender,
  breathDurationMs,
  position,
  bubbleText,
  hintText,
  isDragged,
  activate,
  pointerEnter,
  pointerLeave,
  pointerDown,
  pointerUp,
  beginDrag,
  endDrag,
  moveTo,
  dismissBubble,
  handleViewportResize,
  startSession,
  stopSession,
} = assistant;

/** 拖动阈值：小于它的位移算手抖，仍按点击处理。 */
const DRAG_THRESHOLD_PX = 6;
/** 读屏名称：品牌助手昵称统一从品牌模块取。 */
const assistantName = brandAiName();
/** 扁平矢量果冻（读者站 baseURL 为根；后台不带捏捏，因此这里不需要 baseURL 拼接）。 */
const glyphUrl = brandAssetUrl("/", "flat");

const draggingByPointer = ref(false);
let pressOrigin = { x: 0, y: 0 };
let pointerOffset = { x: 0, y: 0 };

const rootStyle = computed(() => ({
  transform: `translate3d(${position.value.x}px, ${position.value.y}px, 0)`,
}));

const bodyStyle = computed(() => ({
  animationDuration: `${breathDurationMs.value}ms`,
}));

const ariaLabel = computed(() =>
  isAvailable.value ? `${assistantName}：AI 助手` : `${assistantName}：AI 助手（当前已关闭）`,
);

function attachPointerListeners() {
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp);
  window.addEventListener("pointercancel", onPointerUp);
}

function detachPointerListeners() {
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", onPointerUp);
  window.removeEventListener("pointercancel", onPointerUp);
}

function onPointerDown(event: PointerEvent) {
  if (event.button !== 0) return;

  pointerDown();
  draggingByPointer.value = true;
  pressOrigin = { x: event.clientX, y: event.clientY };
  pointerOffset = {
    x: event.clientX - position.value.x,
    y: event.clientY - position.value.y,
  };
  attachPointerListeners();
}

function onPointerMove(event: PointerEvent) {
  if (!draggingByPointer.value) return;

  if (!isDragged.value) {
    const distance = Math.hypot(event.clientX - pressOrigin.x, event.clientY - pressOrigin.y);
    if (distance < DRAG_THRESHOLD_PX) return;
    beginDrag();
  }

  moveTo({
    x: event.clientX - pointerOffset.x,
    y: event.clientY - pointerOffset.y,
  });
}

function onPointerUp() {
  detachPointerListeners();
  draggingByPointer.value = false;

  if (isDragged.value) {
    // 拖完松手不是一次点击：持久化位置后清掉「本次按下算拖拽」的标记。
    endDrag();
    pointerUp();
    return;
  }

  pointerUp();
}

/** 回车：先捏（press）再触发，最后松开展示态，避免按压态卡住。 */
function handleEnter() {
  pointerDown();
  activate();
  pointerLeave();
}

function handleWindowResize() {
  handleViewportResize();
}

onMounted(() => {
  startSession();
  window.addEventListener("resize", handleWindowResize);
});

onBeforeUnmount(() => {
  detachPointerListeners();
  window.removeEventListener("resize", handleWindowResize);
  stopSession();
});
</script>

<template>
  <div
    v-if="shouldRender"
    class="jelly-launcher"
    :class="[`jelly-launcher--${state}`, { 'jelly-launcher--reduced': isReducedMotion }]"
    :style="rootStyle"
  >
    <button
      type="button"
      class="jelly-launcher__button"
      :aria-label="ariaLabel"
      :aria-expanded="isOpen ? 'true' : 'false'"
      @pointerenter="pointerEnter"
      @pointerleave="pointerLeave"
      @pointerdown="onPointerDown"
      @keydown.enter.prevent="handleEnter"
      @keydown.space.prevent="pointerDown"
      @keyup.space.prevent="pointerUp"
    >
      <span class="jelly-launcher__body" :style="bodyStyle">
        <img
          class="jelly-launcher__glyph"
          :src="glyphUrl"
          alt=""
          aria-hidden="true"
          draggable="false"
          width="56"
          height="56"
        >
      </span>
    </button>

    <button v-if="bubbleText" type="button" class="jelly-bubble" @click.stop="dismissBubble">
      {{ bubbleText }}
    </button>

    <p v-if="hintText" class="jelly-hint" role="status">
      {{ hintText }}
    </p>
  </div>
</template>

<style scoped>
/* 位置：固定定位 + transform（只动 transform，不触发布局；拖拽时有轻微滞后跟随）。 */
.jelly-launcher {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 40;
  width: 56px;
  height: 56px;
  will-change: transform;
  pointer-events: none;
}

.jelly-launcher--dragging {
  transition: transform var(--motion-fast) var(--motion-ease);
}

.jelly-launcher__button {
  display: block;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: grab;
  pointer-events: auto;
  touch-action: none;
  transform-origin: 50% 100%;
  transition: transform var(--motion-fast) var(--motion-ease);
}

.jelly-launcher__button:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 3px;
}

.jelly-launcher__body {
  display: block;
  width: 100%;
  height: 100%;
  transform-origin: 50% 100%;
  animation-name: jelly-breathe;
  animation-timing-function: var(--motion-ease);
  animation-iteration-count: infinite;
}

.jelly-launcher__glyph {
  display: block;
  width: 100%;
  height: 100%;
  user-select: none;
}

/* 六态：悬停挤压、按压先捏、打开压成把手、拖拽放大一点、禁用低饱和。 */
.jelly-launcher--hover .jelly-launcher__button {
  transform: scale(1.08, 0.94);
}

.jelly-launcher--press .jelly-launcher__button {
  transform: scale(0.92, 1.08);
}

.jelly-launcher--dragging .jelly-launcher__button {
  cursor: grabbing;
  transform: scale(1.04);
}

.jelly-launcher--open .jelly-launcher__button {
  /* 预留 #10 的把手态：压扁成面板右下角一角，面板就是从这里长出来的。 */
  transform: scale(0.86, 0.72);
  transform-origin: 100% 100%;
}

.jelly-launcher--open .jelly-launcher__body {
  animation: none;
}

.jelly-launcher--disabled .jelly-launcher__glyph {
  filter: saturate(0.32);
  opacity: 0.68;
}

.jelly-launcher--disabled .jelly-launcher__button {
  cursor: not-allowed;
}

.jelly-launcher--reduced .jelly-launcher__body {
  animation: none;
}

@keyframes jelly-breathe {
  0%,
  100% {
    transform: scale(1);
  }

  50% {
    transform: scale(1.03);
  }
}

/* 气泡：直角、语义色纸面，点击即关；只动 transform/opacity。 */
.jelly-bubble {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  width: max-content;
  max-width: 220px;
  margin: 0;
  padding: 10px 14px;
  border: 1px solid var(--color-line);
  border-radius: 0;
  background: var(--color-surface);
  color: var(--color-ink);
  font-size: 13px;
  line-height: 1.6;
  text-align: left;
  box-shadow: 0 12px 28px var(--color-shadow);
  cursor: pointer;
  pointer-events: auto;
  animation: jelly-bubble-in var(--motion-base) var(--motion-ease);
  transform-origin: 100% 100%;
}

/* 不可用提示：同一位置，但用次级色，读屏可读（role=status）。 */
.jelly-hint {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  width: max-content;
  max-width: 200px;
  margin: 0;
  padding: 8px 12px;
  border: 1px solid var(--color-line-strong);
  background: var(--color-bg-subtle);
  color: var(--color-ink-muted);
  font-size: 12px;
  line-height: 1.5;
  pointer-events: none;
}

@keyframes jelly-bubble-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .jelly-launcher--dragging {
    transition: none;
  }

  .jelly-bubble {
    animation: none;
  }
}

@media (max-width: 640px) {
  .jelly-bubble,
  .jelly-hint {
    max-width: 168px;
  }
}
</style>
