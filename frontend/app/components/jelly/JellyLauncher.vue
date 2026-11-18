<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { IconChevronLeft, IconX } from "@tabler/icons-vue";

import {
  clampJellyPosition,
  JELLY_HANDLE_HEIGHT,
  JELLY_HANDLE_HIT_WIDTH,
  JELLY_HANDLE_WIDTH,
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
// 这个文件**不含状态**：七态、可用性、气泡、位置全部由 useJellyAssistant 决定
// （它是唯一真相源，#10 的 AI 面板消费同一实例）。这里只做两件事：
//   1. 把状态渲染成外观（class + 无障碍属性 + 位置）；
//   2. 把指针/键盘事件翻译成 composable 的动作（悬停、按压、拖拽、触发）。
//
// 票 425 之后多了收进右边缘这件事：收起态不画角色本体，改画贴右边缘的把手
// （44×56 命中区里的 16×56 薄玻璃胶囊）；位置来自 composable 的派生坐标
// `collapsedPosition`，组件不另算一份 —— 拖动与收起因此永远不会各说各话。
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
  isCollapsed,
  isChipPinned,
  isReducedMotion,
  shouldRender,
  breathDurationMs,
  position,
  collapsedPosition,
  bubbleText,
  hintText,
  isDragged,
  activate,
  collapse,
  expand,
  pinCollapseChip,
  dismissCollapseChip,
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

/** 触摸设备长按唤出收起按钮的时间（毫秒）：与拖拽阈值互斥，移动即取消。 */
const LONG_PRESS_MS = 500;

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
/** 落位帧循环是否在跑：跑的时候根元素不能有 CSS 过渡（否则逐帧值会被补间拖慢）。 */
const settleRunning = ref(false);
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

/**
 * 根元素位置：收起态用派生坐标（贴右边缘），展开态用自由位置 + 落位偏移。
 *
 * `transition` 只在"可以补间"的时候挂上（收起 / 展开 200ms、可中途反向）；
 * 拖拽与落位期一律 `none` —— 这两种状态的位置是直写的，过渡会让手感脱钩。
 */
const rootStyle = computed(() => {
  const target = isCollapsed.value ? collapsedPosition.value : position.value;
  const offset = isCollapsed.value ? { x: 0, y: 0 } : settleOffset.value;
  const animatable = state.value !== "dragging" && !settleRunning.value;

  return {
    transform: `translate3d(${target.x + offset.x}px, ${target.y + offset.y}px, 0)`,
    // 只读副本：样式表靠它算「贴底时才上抬多少」的安全区让位（票 426）。
    // JS 的坐标语义（左上角 transform、clampJellyPosition）一概不动。
    "--jelly-y": `${target.y + offset.y}px`,
    // 几何常量只有一个来源（composable）：组件把它们写成 CSS 自定义属性，
    // 样式块只消费 `var(--jelly-*)`，不再在 CSS 里重抄 56px / 44px / 16px（票 437 评审）。
    "--jelly-size": `${JELLY_LAUNCHER_SIZE}px`,
    "--jelly-margin": `${JELLY_VIEWPORT_MARGIN}px`,
    "--jelly-handle-width": `${JELLY_HANDLE_WIDTH}px`,
    "--jelly-handle-height": `${JELLY_HANDLE_HEIGHT}px`,
    "--jelly-handle-hit": `${JELLY_HANDLE_HIT_WIDTH}px`,
    transition: animatable
      ? "transform var(--motion-base) var(--motion-ease), opacity var(--motion-base) var(--motion-ease)"
      : "none",
  };
});

const bodyStyle = computed(() => ({
  animationDuration: `${breathDurationMs.value}ms`,
}));

const ariaLabel = computed(() =>
  isAvailable.value ? `${assistantName}：AI 助手` : `${assistantName}：AI 助手（当前已关闭）`,
);

/** 捏捏与收起按钮共享的外层 cluster 是否持有焦点（键盘用户不需要 hover 也能收起）。 */
const clusterFocused = ref(false);
/** Esc 刚刚放弃过这颗 chip：在焦点离开 / 指针重新进入 cluster 之前不再浮出来。 */
const chipDismissed = ref(false);
/** 捏捏本体按钮：Esc 或"从把手展开"之后焦点要落回这里（键盘路径的锚点）。 */
const launcherButtonRef = ref<HTMLButtonElement | null>(null);

/**
 * 收起按钮的出现条件（spec 的显式口径）：悬停捏捏（含悬停 chip 自身）**或**
 * cluster 内有焦点**或**长按唤出（pin）时可见；面板开着或已经收起时不出现 ——
 * 那时捏捏分别是面板把手 / 只剩边缘把手，都不该再给一颗"收起"。
 *
 * `chipDismissed` 是 Esc 的落点：没有它，键盘焦点还在捏捏上时 focus-within 会
 * 让刚被 Esc 收掉的 chip 立刻重新出现，"Esc 收 chip"就名存实亡。
 */
const showCollapseChip = computed(
  () =>
    !chipDismissed.value &&
    !isOpen.value &&
    !isCollapsed.value &&
    (state.value === "hover" || clusterFocused.value || isChipPinned.value),
);

/** 长按唤出收起按钮的计时器（仅粗指针；位移超过拖拽阈值即取消）。 */
let longPressTimer: ReturnType<typeof setTimeout> | null = null;

function clearLongPress() {
  if (longPressTimer !== null) {
    clearTimeout(longPressTimer);
    longPressTimer = null;
  }
}

/** 触摸设备（`pointer: coarse`）才启用长按：鼠标有 hover，不需要另一套手势。 */
function isCoarsePointer(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  try {
    return Boolean(window.matchMedia("(pointer: coarse)").matches);
  } catch {
    return false;
  }
}

function scheduleLongPress() {
  clearLongPress();
  if (!isCoarsePointer()) return;
  longPressTimer = setTimeout(() => {
    longPressTimer = null;
    pinCollapseChip();
  }, LONG_PRESS_MS);
}

/** 点收起：先把落位收尾（别把弹簧偏移带进收起态），再交给 composable。 */
function handleCollapse() {
  endSettle();
  collapse();
}

/**
 * 点把手 / 键盘触发：回到自由位置（composable 决定位置，组件只负责收尾落位）。
 *
 * `source` 区分指针与键盘：只有键盘（回车）需要把焦点交给替换上来的捏捏本体 ——
 * 否则把手被移出 DOM 时焦点会掉到 body，下一次 Tab 从页首重新开始（票 426）。
 */
function handleExpand(source: "pointer" | "keyboard" = "pointer") {
  endSettle();
  expand();
  if (source === "keyboard") {
    nextTick(() => launcherButtonRef.value?.focus({ preventScroll: true }));
  }
}

/**
 * Esc：放弃这次收起（焦点留在捏捏上，不触发任何动作）。
 *
 * 焦点可能在 chip 上：chip 会立刻被移出 DOM，所以先把焦点交回捏捏本体，
 * 否则焦点会掉到 body，"留在角色上"就名存实亡（票 426）。
 */
function handleEscape() {
  clearLongPress();
  chipDismissed.value = true;
  dismissCollapseChip();
  if (launcherButtonRef.value && document.activeElement !== launcherButtonRef.value) {
    launcherButtonRef.value.focus({ preventScroll: true });
  }
}

/** 悬停判定挂在 cluster 上：从捏捏移到右上角 chip 不会触发离开，按钮不会闪断。 */
function handlePointerEnter() {
  chipDismissed.value = false;
  pointerEnter();
}

function handlePointerLeave() {
  pointerLeave();
}

function handleClusterFocusIn() {
  clusterFocused.value = true;
}

function handleClusterFocusOut(event: FocusEvent) {
  const cluster = event.currentTarget as HTMLElement | null;
  const nextTarget = event.relatedTarget as Node | null;
  // 焦点只是在 cluster 内部换手（chip → 捏捏 / 捏捏 → chip）：不算离开，
  // Esc 留下的 dismissed 锁不能被 reset，否则 chip 会立刻弹回来（票 426）。
  if (nextTarget && cluster?.contains(nextTarget)) return;
  clusterFocused.value = false;
  chipDismissed.value = false;
}

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
  settleRunning.value = false;
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
  settleRunning.value = true;
  scheduleSettleFrame();
}

function onPointerDown(event: PointerEvent) {
  if (event.button !== 0) return;
  // 收起态不接受拖拽：那时页面上只有边缘把手，按它应该是"展开"，不是"拖动"。
  if (isCollapsed.value) return;

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
  scheduleLongPress();
}

function onPointerMove(event: PointerEvent) {
  if (!draggingByPointer.value) return;

  if (!isDragged.value) {
    const distance = Math.hypot(event.clientX - pressOrigin.x, event.clientY - pressOrigin.y);
    if (distance < DRAG_THRESHOLD_PX) return;
    // 一开始拖，长按就作废：拖拽与"唤出收起按钮"是两个互不干扰的手势。
    clearLongPress();
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
  clearLongPress();

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
  clearLongPress();
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
    <div
      class="jelly-launcher__cluster"
      @pointerenter="handlePointerEnter"
      @pointerleave="handlePointerLeave"
      @focusin="handleClusterFocusIn"
      @focusout="handleClusterFocusOut"
      @keydown.esc.stop="handleEscape"
    >
      <button
        v-if="!isCollapsed"
        ref="launcherButtonRef"
        type="button"
        class="jelly-launcher__button"
        :aria-label="ariaLabel"
        :aria-expanded="isOpen ? 'true' : 'false'"
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
            :width="JELLY_LAUNCHER_SIZE"
            :height="JELLY_LAUNCHER_SIZE"
          >
        </span>
      </button>

      <Transition v-else name="jelly-handle" appear>
        <button
          type="button"
          class="jelly-launcher__handle"
          aria-label="展开捏捏"
          @click.stop="handleExpand()"
          @keydown.enter.prevent="handleExpand('keyboard')"
        >
          <span class="jelly-launcher__handle-capsule glass-thin">
            <IconChevronLeft
              :size="12"
              :stroke-width="2"
              class="jelly-launcher__handle-arrow"
              aria-hidden="true"
            />
          </span>
        </button>
      </Transition>

      <!-- 收起 chip：紧跟在捏捏之后（Tab 序列相邻），命中区 28×28 内是一颗 20×20 的纸面 -->
      <button
        v-if="showCollapseChip"
        type="button"
        class="jelly-collapse-chip"
        aria-label="收起捏捏"
        @click.stop="handleCollapse"
        @keydown.esc.stop="handleEscape"
      >
        <span class="jelly-collapse-chip__face">
          <IconX :size="10" :stroke-width="2" aria-hidden="true" />
        </span>
      </button>

      <button
        v-if="!isCollapsed && bubbleText"
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
  </div>
</template>

<style scoped>
/* 位置：固定定位 + transform（只动 transform，不触发布局）。
 * 位置更新要么直写（拖拽 1:1）、要么逐帧弹簧（松手落位），CSS 过渡一律不参与 ——
 * 拖拽 / 落位期根元素的 inline `transition: none` 就是这条契约的显式声明（票 424）。
 * 收起 / 展开则相反：只有 transform 与 opacity 的 200ms transition，可中途反向。
 *
 * **根元素尺寸恒定**（票 437 评审）：以前收起态把 width 从 56px 改成 44px，等于
 * 每次收起/展开都动一次布局属性。现在根元素始终是角色本体边长（`--jelly-size`，
 * 由 composable 注入），贴边对齐靠 transform + 把手按钮在根内右对齐 ——
 * 收起/展开只动 transform/opacity，不产生任何重排。 */
.jelly-launcher {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 40;
  /* 几何常量来自 composable（`rootStyle` 写成的 CSS 变量），样式表不再重抄像素值。 */
  width: var(--jelly-size);
  height: var(--jelly-size);
  will-change: transform;
  pointer-events: none;

  /* 安全区让位（票 426）：捏捏与把手贴右下角时整体上抬，不被 iPhone 的 Home 指示条压住。
   * 只在**渲染层**让位：`transform` 仍是 JS 坐标（左上角）的唯一真相，
   * `clampJellyPosition` / 持久化 / 拖拽采样一概不掺安全区 —— 上面 inline 的
   * `--jelly-y` 是组件抄给 CSS 的只读副本。让位量 = 角色底边超出「视口底 - 安全区」
   * 的那一段，所以角色在视口中高处时恒为 0（不会整体上移），贴底时最多抬
   * env(safe-area-inset-bottom)。`dvh` 与 JS 的 innerHeight 同源；不支持它的浏览器
   * 整条声明失效、退化为不让位（不写死像素，也不会抬错）。 */
  --jelly-safe-bottom: env(safe-area-inset-bottom, 0px);
  translate: 0px calc(-1 * max(
    0px,
    var(--jelly-y, 0px) + var(--jelly-size) + var(--jelly-margin)
      - 100dvh + var(--jelly-safe-bottom)
  ));
}

/* 收起态：根元素**尺寸不变**，只是本体让位给把手；根左侧那 12px 是透明的，
 * 不能让 cluster 吃掉页面点击 —— 只留把手按钮可交互。 */
.jelly-launcher--collapsed .jelly-launcher__cluster {
  pointer-events: none;
}

/* 悬停判定挂在包含捏捏与 chip 的外层 cluster 上（cluster 吃指针事件）：
 * 从捏捏移到右上角 chip 上时不会触发 pointerleave，按钮不会闪断。 */
.jelly-launcher__cluster {
  position: relative;
  width: 100%;
  height: 100%;
  pointer-events: auto;
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

/* 收起态把手：命中区 44×56（透明），可见胶囊 16×56 右对齐贴边、左侧圆角。
 * 命中区靠按钮本身的宽度拿到（`--jelly-handle-hit`，由 composable 注入），
 * 按钮在**尺寸恒定的根元素内右对齐** —— 不用负 inset、也不改根宽度（票 437 评审）。
 * 贴边元素不能外扩 outline，所以焦点环用负 offset 内嵌。 */
.jelly-launcher__handle {
  position: absolute;
  top: 0;
  right: 0;
  display: block;
  width: var(--jelly-handle-hit);
  height: var(--jelly-handle-height);
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
  pointer-events: auto;
  transform-origin: 100% 50%;
  transition: transform var(--motion-fast) var(--motion-ease);
}

.jelly-launcher__handle-capsule {
  position: absolute;
  top: 0;
  right: 0;
  display: grid;
  place-items: center;
  width: var(--jelly-handle-width);
  height: var(--jelly-handle-height);
  border-radius: var(--radius-pill) 0 0 var(--radius-pill);
  color: var(--color-ink-muted);
  transition: color var(--motion-fast) var(--motion-ease), width var(--motion-fast) var(--motion-ease);
}

.jelly-launcher__handle-arrow {
  transition: transform var(--motion-fast) var(--motion-ease);
}

/* 把手三态：hover 可见宽度 = 可见宽 + 4px（20px）+ 箭头左移 2px；按下 scale(.96)；
 * 焦点用内嵌环。宽度差写成 `calc()`：16/20 这两个数只由 `--jelly-handle-width` 派生。 */
.jelly-launcher__handle:hover .jelly-launcher__handle-capsule {
  width: calc(var(--jelly-handle-width) + 4px);
  color: var(--color-ink);
}

.jelly-launcher__handle:hover .jelly-launcher__handle-arrow {
  transform: translateX(-2px);
}

.jelly-launcher__handle:active {
  transform: scale(0.96);
}

.jelly-launcher__handle:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: -3px;
}

/* 把手进场：opacity + 4px translateX，120ms；减少动态由 token 归零。 */
.jelly-handle-enter-active {
  transition: opacity var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-ease);
}

.jelly-handle-enter-from {
  opacity: 0;
  transform: translateX(4px);
}

.jelly-handle-leave-active {
  transition: opacity var(--motion-fast) var(--motion-ease);
}

.jelly-handle-leave-to {
  opacity: 0;
}

/* 收起 chip：捏捏右上角。视觉 20×20（纸面 + 1px 结构线 + ×），命中区 28×28。 */
.jelly-collapse-chip {
  position: absolute;
  top: -6px;
  right: -6px;
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
  pointer-events: auto;
}

.jelly-collapse-chip__face {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-pill);
  background: var(--color-bg-page);
  color: var(--color-ink-muted);
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.jelly-collapse-chip:hover .jelly-collapse-chip__face {
  border-color: var(--color-ink-faint);
  color: var(--color-ink);
}

.jelly-collapse-chip:active .jelly-collapse-chip__face {
  transform: scale(0.96);
}

.jelly-collapse-chip:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* 七态：悬停挤压、按压先捏、打开压成把手、收起只剩边缘把手、拖拽放大一点、禁用低饱和。 */
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

/* 收起态的状态提示挂在把手左侧（贴边元素没有上方空间，不能向上弹）。 */
.jelly-launcher--collapsed .jelly-hint {
  right: calc(100% + 8px);
  top: 0;
  bottom: auto;
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
