<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

import {
  clampJellyPosition,
  JELLY_LAUNCHER_SIZE,
  JELLY_VIEWPORT_MARGIN,
  useJellyAssistant,
} from "~/composables/useJellyAssistant";
import type { JellyAssistantApi, JellyPosition } from "~/composables/useJellyAssistant";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { resolveSpringMotion, SPRING_LAUNCHER_SETTLE } from "~/utils/motion";
import { brandAiName, brandAssetUrl } from "~/utils/brand";

// 「捏捏」的外观组件（票 #9）。
//
// 这个文件**不含状态**：六态、可用性、气泡、位置全部由 useJellyAssistant 决定
// （它是唯一真相源，#10 的 AI 面板消费同一实例）。这里只做两件事：
//   1. 把状态渲染成外观（class + 无障碍属性 + 位置）；
//   2. 把指针/键盘事件翻译成 composable 的动作（悬停、按压、拖拽、触发）。
//
// 票 424 之后多了第三件事：**拖拽松手后的落位**。位置仍然只有 composable 一份（拖拽期
// 直写、视口边界即时钳制），这里只在松手那一刻把指针速度交给登记过的弹簧
// （`SPRING_LAUNCHER_SETTLE`），逐帧算出"相对松手点的偏移"，把它叠进 transform。
// 落位结束偏移回到 0，所以松手处就是最终位置 —— 弹簧不产生任何位置漂移。
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

/** 松手速度采样窗口（毫秒）：只取最后一小段指针历史，甩出去的手感才跟得上。 */
const RELEASE_WINDOW_MS = 80;
/** 松手速度上限（px/s）：留一点余劲，但不会把角色甩出半屏。 */
const RELEASE_VELOCITY_MAX = 600;

/** 落位结束判定：偏移（px）与速度（px/s）都小到看不出来就收帧。 */
const SETTLE_REST_PX = 0.5;
const SETTLE_REST_VELOCITY = 12;
/** 单帧上限与积分子步（毫秒）：切后台回来 dt 会很大，钳住才不会发散。 */
const SETTLE_FRAME_MS = 48;
const SETTLE_SUBSTEP_MS = 16;
/** rAF 没给时间戳时的名义帧长（浏览器总会给；测试桩可能不给）。 */
const NOMINAL_FRAME_MS = 16;

/** 读屏名称：品牌助手昵称统一从品牌模块取。 */
const assistantName = brandAiName();
/** 扁平矢量果冻（读者站 baseURL 为根；后台不带捏捏，因此这里不需要 baseURL 拼接）。 */
const glyphUrl = brandAssetUrl("/", "flat");

/** 气泡上的指针镜面高光（移动端 / 减少动态时 composable 内部关闭）。 */
const { handlePointerMove } = usePointerSpecular();

const draggingByPointer = ref(false);
let pressOrigin = { x: 0, y: 0 };
let pointerOffset = { x: 0, y: 0 };

/** 松手速度采样：按下之后每次指针移动的坐标与时间戳。 */
let velocitySamples: Array<{ x: number; y: number; t: number }> = [];

/** 落位偏移（相对松手点）；只改视觉，不回写 composable 的 position。 */
const settleOffset = ref<JellyPosition>({ x: 0, y: 0 });
let settle: { x: number; y: number; vx: number; vy: number } | null = null;
let settleFrame: number | null = null;
let settleLastTimestamp: number | null = null;

/**
 * 松手落位用哪一档：登记弹簧，或「减少动态」下的落位（时长 0）。
 * 组件只读解析结果，不自己写 stiffness/damping，也不另开 reduced 分支。
 */
const settleMotion = computed(() =>
  resolveSpringMotion(SPRING_LAUNCHER_SETTLE, isReducedMotion.value),
);

const rootStyle = computed(() => ({
  // 位置要么直写（拖拽 1:1）、要么逐帧弹簧（松手落位）：CSS 过渡一律不参与。
  transform: `translate3d(${position.value.x + settleOffset.value.x}px, ${position.value.y + settleOffset.value.y}px, 0)`,
  transition: "none",
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

function notePointerSample(event: PointerEvent) {
  velocitySamples.push({ x: event.clientX, y: event.clientY, t: event.timeStamp });
  if (velocitySamples.length > 8) velocitySamples.shift();
}

/** 松手速度（px/s）：采样窗口内最早与最新两点求差；没有有效时间差就当没甩。 */
function releaseVelocity(): { x: number; y: number } {
  if (velocitySamples.length < 2) return { x: 0, y: 0 };

  const last = velocitySamples[velocitySamples.length - 1];
  const recent = velocitySamples.filter((sample) => last.t - sample.t <= RELEASE_WINDOW_MS);
  const first = recent.length >= 2 ? recent[0] : velocitySamples[0];
  const elapsed = last.t - first.t;
  if (!Number.isFinite(elapsed) || elapsed <= 0) return { x: 0, y: 0 };

  const vx = ((last.x - first.x) / elapsed) * 1000;
  const vy = ((last.y - first.y) / elapsed) * 1000;
  const speed = Math.hypot(vx, vy);
  if (speed <= RELEASE_VELOCITY_MAX) return { x: vx, y: vy };

  const scale = RELEASE_VELOCITY_MAX / speed;
  return { x: vx * scale, y: vy * scale };
}

/**
 * 视口内合法的左上角范围。
 *
 * 边界不在这里另写一套公式：直接问 `clampJellyPosition`（拖拽期钳制位置的同一个函数），
 * 把极小/极大坐标喂进去就得到下限与上限 —— 松手落位的越界判断与拖拽永远一致。
 */
function settleBounds() {
  const viewport =
    typeof window === "undefined"
      ? { width: 0, height: 0 }
      : { width: window.innerWidth || 0, height: window.innerHeight || 0 };
  const low = clampJellyPosition(
    { x: -Number.MAX_SAFE_INTEGER, y: -Number.MAX_SAFE_INTEGER },
    viewport,
    JELLY_LAUNCHER_SIZE,
    JELLY_VIEWPORT_MARGIN,
  );
  const high = clampJellyPosition(
    { x: Number.MAX_SAFE_INTEGER, y: Number.MAX_SAFE_INTEGER },
    viewport,
    JELLY_LAUNCHER_SIZE,
    JELLY_VIEWPORT_MARGIN,
  );

  return { minX: low.x, minY: low.y, maxX: high.x, maxY: high.y };
}

/** 落位不许越界：偏移到墙就把那一轴的速度收掉（否则边界上会一直抖）。 */
function clampSettleOffset() {
  if (!settle) return;

  const bounds = settleBounds();
  const wantedX = position.value.x + settle.x;
  const wantedY = position.value.y + settle.y;

  if (wantedX < bounds.minX) {
    settle.x = bounds.minX - position.value.x;
    if (settle.vx < 0) settle.vx = 0;
  } else if (wantedX > bounds.maxX) {
    settle.x = bounds.maxX - position.value.x;
    if (settle.vx > 0) settle.vx = 0;
  }

  if (wantedY < bounds.minY) {
    settle.y = bounds.minY - position.value.y;
    if (settle.vy < 0) settle.vy = 0;
  } else if (wantedY > bounds.maxY) {
    settle.y = bounds.maxY - position.value.y;
    if (settle.vy > 0) settle.vy = 0;
  }
}

/** 半隐式欧拉积分（子步 ≤16ms）；参数全部来自登记预设，本文件没有弹簧数字。 */
function integrateSettle(preset: { stiffness: number; damping: number; mass: number }, dtMs: number) {
  if (!settle) return;

  let remaining = dtMs;
  while (remaining > 0) {
    const stepSeconds = Math.min(remaining, SETTLE_SUBSTEP_MS) / 1000;
    const ax = (-preset.stiffness * settle.x - preset.damping * settle.vx) / preset.mass;
    const ay = (-preset.stiffness * settle.y - preset.damping * settle.vy) / preset.mass;
    settle.vx += ax * stepSeconds;
    settle.vy += ay * stepSeconds;
    settle.x += settle.vx * stepSeconds;
    settle.y += settle.vy * stepSeconds;
    remaining -= Math.min(remaining, SETTLE_SUBSTEP_MS);
  }
}

function stopSettleFrame() {
  if (settleFrame !== null) {
    cancelAnimationFrame(settleFrame);
    settleFrame = null;
  }
}

/** 结束落位（自然归位、被新的按下接管、卸载）：清零偏移，不留残余帧。 */
function endSettle() {
  stopSettleFrame();
  settle = null;
  settleLastTimestamp = null;
  settleOffset.value = { x: 0, y: 0 };
}

function scheduleSettleFrame() {
  if (settleFrame !== null) return;
  settleFrame = requestAnimationFrame(stepSettleFrame);
}

function stepSettleFrame(timestamp?: number) {
  settleFrame = null;
  const active = settle;
  const motion = settleMotion.value;

  // 减少动态（解析结果是落位档）或已经没有落位任务：直接静止。
  if (!active || motion.type !== "spring") {
    endSettle();
    return;
  }

  const frameTime = typeof timestamp === "number" && Number.isFinite(timestamp) ? timestamp : null;
  const dtMs =
    frameTime !== null && settleLastTimestamp !== null
      ? Math.min(SETTLE_FRAME_MS, Math.max(1, frameTime - settleLastTimestamp))
      : NOMINAL_FRAME_MS;
  settleLastTimestamp = frameTime;

  integrateSettle(motion, dtMs);
  clampSettleOffset();
  settleOffset.value = { x: active.x, y: active.y };

  if (
    Math.abs(active.x) < SETTLE_REST_PX &&
    Math.abs(active.y) < SETTLE_REST_PX &&
    Math.hypot(active.vx, active.vy) < SETTLE_REST_VELOCITY
  ) {
    endSettle();
    return;
  }

  scheduleSettleFrame();
}

/** 松手：把指针余劲交给登记弹簧；减少动态时解析结果就是「直接落位」。 */
function startSettle(velocity: { x: number; y: number }) {
  if (settleMotion.value.type !== "spring") {
    endSettle();
    return;
  }

  stopSettleFrame();
  settle = { x: 0, y: 0, vx: velocity.x, vy: velocity.y };
  settleLastTimestamp = null;
  settleOffset.value = { x: 0, y: 0 };
  scheduleSettleFrame();
}

function onPointerDown(event: PointerEvent) {
  if (event.button !== 0) return;

  endSettle();
  velocitySamples = [{ x: event.clientX, y: event.clientY, t: event.timeStamp }];
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

  notePointerSample(event);
  moveTo({
    x: event.clientX - pointerOffset.x,
    y: event.clientY - pointerOffset.y,
  });
}

function onPointerUp(event: PointerEvent) {
  detachPointerListeners();
  draggingByPointer.value = false;

  if (isDragged.value) {
    // 拖完松手不是一次点击：记下松手速度 → 持久化位置 → 交给弹簧把余劲走完。
    notePointerSample(event);
    const velocity = releaseVelocity();
    velocitySamples = [];
    endDrag();
    pointerUp();
    startSettle(velocity);
    return;
  }

  velocitySamples = [];
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
  endSettle();
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

    <button
      v-if="bubbleText"
      type="button"
      class="jelly-bubble glass-thin"
      @click.stop="dismissBubble"
      @pointermove="handlePointerMove"
    >
      {{ bubbleText }}
    </button>

    <p v-if="hintText" class="jelly-hint" role="status">
      {{ hintText }}
    </p>
  </div>
</template>

<style scoped>
/* 位置：固定定位 + transform（只动 transform，不触发布局）。
 * 位置更新要么直写（拖拽 1:1）、要么逐帧弹簧（松手落位），CSS 过渡一律不参与 ——
 * 根元素的 inline `transition: none` 就是这条契约的显式声明（票 424）。 */
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

/* 气泡：薄玻璃胶囊，点击即关；只动 transform/opacity。
 * 材料（填充/模糊/边缘/噪点）由 .glass-thin 提供，这里只留布局。 */
.jelly-bubble {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  width: max-content;
  max-width: 220px;
  margin: 0;
  padding: 10px 14px;
  border-radius: var(--radius-md);
  color: var(--color-ink);
  font-size: 13px;
  line-height: 1.6;
  text-align: left;
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
  border-radius: var(--radius-md);
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
