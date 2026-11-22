<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { IconChevronLeft, IconX } from "@tabler/icons-vue";

import {
  JELLY_HANDLE_HEIGHT,
  JELLY_HANDLE_HIT_WIDTH,
  JELLY_HANDLE_WIDTH,
  JELLY_LAUNCHER_SIZE,
  JELLY_VIEWPORT_MARGIN,
  useJellyAssistant,
} from "~/composables/useJellyAssistant";
import type { JellyAssistantApi } from "~/composables/useJellyAssistant";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { brandAssetUrl } from "~/utils/brand";

// 「捏捏」的外观组件。
//
// 这个文件**不含状态**：六态、可用性、气泡、收起全部由 useJellyAssistant 决定
// （它是唯一真相源，AI 面板消费同一实例）。这里只做两件事：
//   1. 把状态渲染成外观（class + 无障碍属性）；
//   2. 把指针/键盘事件翻译成 composable 的动作（悬停、按压、长按、触发）。
//
// 位置完全交给 CSS：角色固定右下角（`right` / `bottom` 直接吃安全区），收起把手
// 固定右边缘垂直中部（`top: 45%`），避开底部进度条 / 页脚 / iPhone 手势条。
// 组件不再写 `translate3d`、不再有拖拽与松手落位 —— 指针移动不产生任何位置语义。
//
// 收起态不画角色本体，改画贴右边缘的把手（44×56 命中区里的 16×56 薄玻璃胶囊）。
// 收起 / 展开只有 transform 与 opacity 的过渡：角色向右滑出并淡出，把手从右边缘进。
//
// 形状来源是扁平矢量果冻方块（`brand/jelly-flat.svg`），颜色全部来自语义 token
// （--color-*），本文件不出现任何裸色值（由漂移门禁卡住）。

// 状态实例：布局会传入页面级共享实例（AI 面板消费同一份）；
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
  bubbleText,
  hintText,
  activate,
  collapse,
  expand,
  pinCollapseChip,
  dismissCollapseChip,
  pointerEnter,
  pointerLeave,
  pointerDown,
  pointerUp,
  dismissBubble,
  startSession,
  stopSession,
} = assistant;

/** 触摸设备长按唤出收起按钮的时间（毫秒）。 */
const LONG_PRESS_MS = 500;

/** 读屏名称：中性说法，不自称「捏捏」。 */
const ariaLabel = computed(() =>
  isAvailable.value ? "AI 助手：打开对话" : "AI 助手：打开对话（当前已关闭）",
);

/** 扁平矢量果冻（读者站 baseURL 为根；后台不带捏捏，因此这里不需要 baseURL 拼接）。 */
const glyphUrl = brandAssetUrl("/", "flat");

/** 气泡上的指针镜面高光（移动端 / 减少动态时 composable 内部关闭）。 */
const { handlePointerMove } = usePointerSpecular();

/**
 * 几何常量只有一个来源（composable）：组件把它们写成 CSS 自定义属性，
 * 样式块只消费 `var(--jelly-*)`，不再在 CSS 里重抄 56px / 44px / 16px。
 *
 * 这里不写任何位置：固定右下角、把手中部对齐全由样式表负责。
 */
const rootStyle = {
  "--jelly-size": `${JELLY_LAUNCHER_SIZE}px`,
  "--jelly-margin": `${JELLY_VIEWPORT_MARGIN}px`,
  "--jelly-handle-width": `${JELLY_HANDLE_WIDTH}px`,
  "--jelly-handle-height": `${JELLY_HANDLE_HEIGHT}px`,
  "--jelly-handle-hit": `${JELLY_HANDLE_HIT_WIDTH}px`,
};

const bodyStyle = computed(() => ({
  animationDuration: `${breathDurationMs.value}ms`,
}));

/** 捏捏与收起按钮共享的外层 cluster 是否持有焦点（键盘用户不需要 hover 也能收起）。 */
const clusterFocused = ref(false);
/** Esc 刚刚放弃过这颗 chip：在焦点离开 / 指针重新进入 cluster 之前不再浮出来。 */
const chipDismissed = ref(false);
/** 捏捏本体按钮：Esc 或"从把手展开"之后焦点要落回这里（键盘路径的锚点）。 */
const launcherButtonRef = ref<HTMLButtonElement | null>(null);
/** 收起态把手：键盘触发收起后焦点交给它（否则焦点随 chip 卸载掉到 body）。 */
const handleButtonRef = ref<HTMLButtonElement | null>(null);

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

/** 长按唤出收起按钮的计时器（仅粗指针）。 */
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

/**
 * 点收起：`click` 的 `detail` 区分触发来源：指针点击 ≥1，键盘（Enter / Space
 * 合成 click）为 0。只有键盘要把焦点交给随后出现的把手 —— 指针用户点完收起
 * 不该被抢焦点。收起后 chip 立即卸载，不接住焦点就会掉到 body（票 423）。
 */
function handleCollapse(event?: MouseEvent) {
  const keyboard = !event || event.detail === 0;
  collapse();
  if (keyboard) {
    nextTick(() => handleButtonRef.value?.focus({ preventScroll: true }));
  }
}

/**
 * 点把手 / 键盘触发：展开角色（位置由 CSS 固定，组件不再收尾任何落位）。
 *
 * `source` 区分指针与键盘：只有键盘（回车）需要把焦点交给替换上来的捏捏本体 ——
 * 否则把手被移出 DOM 时焦点会掉到 body，下一次 Tab 从页首重新开始（票 426）。
 */
function handleExpand(source: "pointer" | "keyboard" = "pointer") {
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

/**
 * 按下：只记录按压态 +（粗指针时）排长按。没有位置采样、没有阈值、没有 window 监听。
 */
function onPointerDown(event: PointerEvent) {
  if (event.button !== 0) return;
  if (isCollapsed.value) return;

  pointerDown();
  scheduleLongPress();
}

/** 松手：取消长按计时，交给 composable 结算（是点击还是被长按抑制由它决定）。 */
function onPointerUp() {
  clearLongPress();
  pointerUp();
}

/** 指针被系统取消（滚动 / 手势接管）：只收掉按压态，不触发点击。 */
function onPointerCancel() {
  clearLongPress();
  if (state.value === "press") pointerLeave();
}

/** 回车：先捏（press）再触发，最后松开展示态，避免按压态卡住。 */
function handleEnter() {
  pointerDown();
  activate();
  pointerLeave();
}

onMounted(() => {
  startSession();
});

onBeforeUnmount(() => {
  clearLongPress();
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
      <!--
        角色与把手同住一个根：收起时角色先带着 transform/opacity 过渡滑向右边缘、
        淡出；Vue 的 <Transition> 在过渡结束后才真正卸载它（可中途反向）。
        把手用独立 <Transition appear> 叠上来，因此收起有完整可见位移，不是瞬时换装。
      -->
      <Transition name="jelly-character">
        <button
          v-if="!isCollapsed"
          ref="launcherButtonRef"
          type="button"
          class="jelly-launcher__button"
          :aria-label="ariaLabel"
          :aria-expanded="isOpen ? 'true' : 'false'"
          @pointerdown="onPointerDown"
          @pointerup="onPointerUp"
          @pointercancel="onPointerCancel"
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
      </Transition>

      <Transition name="jelly-handle" appear>
        <button
          v-if="isCollapsed"
          ref="handleButtonRef"
          type="button"
          class="jelly-launcher__handle"
          aria-label="展开 AI 助手"
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
        aria-label="收起 AI 助手"
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
/* 位置：纯 CSS 固定右下角。
 *
 * `right` / `bottom` 直接吃安全区：左边是 12px 留白，右边最多让到
 * env(safe-area-inset-right)；底边最多抬一个 env(safe-area-inset-bottom)，
 * 不会被 iPhone 的 Home 指示条压住。根元素尺寸恒定（`--jelly-size`，由 composable
 * 注入），收起 / 展开只动子元素的 transform / opacity，不产生任何重排。
 *
 * 注意：根元素**不能**有 transform / filter / will-change: transform —— 收起把手是
 * `position: fixed`，任何会生成包含块的祖先都会把它从「贴视口右边缘」变成
 * 「贴根元素」，右边缘中部就失效了。 */
.jelly-launcher {
  position: fixed;
  right: max(var(--jelly-margin), env(safe-area-inset-right));
  bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom));
  z-index: 40;
  /* 几何常量来自 composable（`rootStyle` 写成的 CSS 变量），样式表不再重抄像素值。 */
  width: var(--jelly-size);
  height: var(--jelly-size);
  pointer-events: none;
}

/* 收起态：根元素尺寸不变，只是本体让位给把手；根所在的 56×56 区域是透明的，
 * 不能让 cluster 吃掉页面点击 —— 只留（固定定位到右边缘中部的）把手可交互。
 * 同时把根抬到 50：根元素的 z-index 会形成层叠上下文，不抬的话把手自身的 50
 * 会被限制在 40 这一层（仍低于 60 的评论面板 / 弹窗）。 */
.jelly-launcher--collapsed {
  z-index: 50;
}

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
  cursor: pointer;
  pointer-events: auto;
  touch-action: manipulation;
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
 *
 * 位置固定在**右边缘的垂直中部**（top 45% + 垂直居中），不贴右下角 —— 底部进度条、
 * 页脚、移动端底栏与 iPhone 手势条都碰不到它。z-index 50 高于页面普通 chrome，
 * 仍低于评论面板 / 弹窗的 60。
 *
 * 垂直居中用独立的 `translate` 属性而不是 `transform: translateY(-50%)`：进场动画
 * 的 `transform: translateX(4px)`、hover 的 scaleX / active 的 scale 都在用
 * `transform`，混在一起会互相覆盖。 */
.jelly-launcher__handle {
  position: fixed;
  right: 0;
  bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom));
  z-index: 50;
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
  /* 锚在右边缘放大，胶囊只会向左"探出"，不改变按钮命中区（仍是 44px 布局宽度）。 */
  transform-origin: 100% 50%;
  transition: color var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-ease);
}

.jelly-launcher__handle-arrow {
  transition: transform var(--motion-fast) var(--motion-ease);
}

/* 把手三态：hover 用 `scaleX(1.25)`（= 可见 16px → 20px，不补间 width）+ 箭头左移 2px；
 * 按下 scale(.96)；焦点用内嵌环。宽度本身仍是 `--jelly-handle-width`，命中区不变。 */
.jelly-launcher__handle:hover .jelly-launcher__handle-capsule {
  transform: scaleX(1.25);
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

/* 六态：悬停挤压、按压先捏、打开压成把手、收起只剩边缘把手、禁用低饱和。 */
.jelly-launcher--hover .jelly-launcher__button {
  transform: scale(1.08, 0.94);
}

.jelly-launcher--press .jelly-launcher__button {
  transform: scale(0.92, 1.08);
}

.jelly-launcher--open .jelly-launcher__button {
  /* 预留 AI 面板的把手态：压扁成面板右下角一角，面板就是从这里长出来的。 */
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

/* 收起 / 展开角色本体：200ms，只动 transform 与 opacity，可中途反向（Vue 过渡类）。
 * 位移量 = 自身宽度 − 把手可见宽度：角色向右滑出并淡出，动画结束才卸载。
 * 选择器带根类前缀（0,2,0）并排在上面的六态规则之后，保证六态的挤压 transform
 * 不会盖掉过渡的起止值。 */
.jelly-launcher .jelly-launcher__button.jelly-character-enter-active,
.jelly-launcher .jelly-launcher__button.jelly-character-leave-active {
  transition:
    transform var(--motion-base) var(--motion-ease),
    opacity var(--motion-base) var(--motion-ease);
}

.jelly-launcher .jelly-launcher__button.jelly-character-enter-from,
.jelly-launcher .jelly-launcher__button.jelly-character-leave-to {
  opacity: 0;
  transform: translateX(calc(100% - var(--jelly-handle-width)));
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

/* 收起态的状态提示挂在把手左侧：跟把手同样固定在右边缘垂直中部，
 * 这样「我先躲起来」不会落在底部进度条 / 页脚附近。 */
.jelly-launcher--collapsed .jelly-hint {
  position: fixed;
  top: 45%;
  right: calc(max(var(--jelly-margin), env(safe-area-inset-right)) + var(--jelly-handle-hit) + 8px);
  bottom: auto;
  translate: 0 -50%;
}

/* 不可用提示：展开态挂在角色上方，读屏可读（role=status）。 */
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


