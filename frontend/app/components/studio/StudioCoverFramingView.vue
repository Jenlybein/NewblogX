<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";

import {
  COVER_ASPECT,
  COVER_ZOOM_MAX,
  COVER_ZOOM_MIN,
  COVER_ZOOM_STEP,
  DEFAULT_FRAMING,
  clampFraming,
  framingStyle,
  nudgeFraming,
  panFraming,
  snapZoom,
  stepFramingZoom,
} from "~/utils/coverFraming";
import type { Framing, Size } from "~/utils/coverFraming";

/**
 * 文章封面的 16:9 取景窗。
 *
 * 与后台轮播取景窗同一套做法与视觉语言：固定 16:9 的框 + 压暗的台面，
 * 拖动图片决定露出哪一块，方向键移动取景范围、加减号缩放。
 * 算术全部在 `~/utils/coverFraming`，这里只管交互。
 *
 * 为什么用 `object-fit: cover` + `object-position` + `transform: scale()`：
 * 预览与取景窗里看到的几何由 `framingStyle()` 一次给出，不引入裁剪库、
 * 不产生第二份图。
 */

const props = withDefaults(
  defineProps<{
    /** 原图地址（本地预览；不裁剪、不重传）。 */
    src: string;
    /** 原图像素尺寸：决定 cover 之后哪个方向有多余量可拖。 */
    imageWidth: number;
    imageHeight: number;
    /** 已有的取景记录（重新取景时带进来）；缺省＝居中、缩放 1。 */
    framing?: Framing | null;
    imageAlt?: string;
  }>(),
  { framing: null, imageAlt: "" },
);

const emit = defineEmits<{
  confirm: [framing: Framing];
  cancel: [];
  "update:framing": [framing: Framing];
}>();

const framing = ref<Framing>(clampFraming(props.framing));
const dragging = ref(false);
const frameEl = ref<HTMLElement | null>(null);

// 同一个组件既要能首次打开，也要能"换一张图再打开"：外部参数变了就重新开场。
watch(
  () => props.framing,
  (next) => {
    framing.value = clampFraming(next);
  },
);

/** 四条内联声明：与线上渲染逐条相同，没有像素值。 */
const imageStyle = computed(() => framingStyle(framing.value));

/** 读数（只给人看）：焦点百分比与缩放倍数。 */
const readout = computed(
  () =>
    `焦点 ${Math.round(framing.value.focus_x * 100)}% · ${Math.round(framing.value.focus_y * 100)}% · 缩放 ${framing.value.zoom.toFixed(2)}×`,
);

function apply(next: Framing) {
  framing.value = clampFraming(next);
  emit("update:framing", framing.value);
}

/**
 * 取景框的像素尺寸。
 *
 * 量不到（首帧未排版、被隐藏、测试环境没有布局引擎）时退回一个 16:9 的
 * 参考尺寸：拖拽换算出来的是**比例**，而比例与框多大无关——只要框仍是 16:9，
 * 用 800×450 和用真实尺寸算出的焦点完全相同。
 */
function measureFrame(): Size {
  const rect = frameEl.value?.getBoundingClientRect();
  if (rect && rect.width > 0 && rect.height > 0) {
    return { width: rect.width, height: rect.height };
  }
  return { width: 800, height: 800 / COVER_ASPECT };
}

function imageSize(): Size {
  return { width: props.imageWidth, height: props.imageHeight };
}

interface DragStart {
  x: number;
  y: number;
  framing: Framing;
}

let dragStart: DragStart | null = null;

function onPointerDown(event: PointerEvent) {
  // 只接主键拖动：右键菜单 / 中键滚动不该把图拖走。
  if (event.button > 0) return;
  dragStart = { x: event.clientX, y: event.clientY, framing: { ...framing.value } };
  dragging.value = true;
  frameEl.value?.focus();
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", stopDrag);
  window.addEventListener("pointercancel", stopDrag);
  event.preventDefault();
}

function onPointerMove(event: PointerEvent) {
  if (!dragStart) return;
  apply(panFraming(dragStart.framing, event.clientX - dragStart.x, event.clientY - dragStart.y, measureFrame(), imageSize()));
}

function stopDrag() {
  dragStart = null;
  dragging.value = false;
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", stopDrag);
  window.removeEventListener("pointercancel", stopDrag);
}

onBeforeUnmount(stopDrag);

/**
 * 方向键移的是**取景范围**（按右箭头看右边），加减号缩放。
 * 与鼠标拖动方向相反是有意的：拖动是"推图"，提示文案因此写"方向键移动取景范围"。
 */
function onKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case "ArrowLeft":
      apply(nudgeFraming(framing.value, -1, 0));
      break;
    case "ArrowRight":
      apply(nudgeFraming(framing.value, 1, 0));
      break;
    case "ArrowUp":
      apply(nudgeFraming(framing.value, 0, -1));
      break;
    case "ArrowDown":
      apply(nudgeFraming(framing.value, 0, 1));
      break;
    case "+":
    case "=":
      apply(stepFramingZoom(framing.value, 1));
      break;
    case "-":
    case "_":
      apply(stepFramingZoom(framing.value, -1));
      break;
    case "Escape":
      onCancel();
      break;
    default:
      return;
  }
  event.preventDefault();
}

function onZoomInput(event: Event) {
  const value = Number((event.target as HTMLInputElement).value);
  apply({ ...framing.value, zoom: snapZoom(value) });
}

function resetFraming() {
  apply(DEFAULT_FRAMING);
}

function confirmFraming() {
  emit("confirm", clampFraming(framing.value));
}

/** 取消：不上抛任何取景参数（调用方据此中止本次上传，什么也不产生）。 */
function onCancel() {
  emit("cancel");
}
</script>

<template>
  <section class="cover-framing">
    <p class="cover-framing__hint">
      固定 16:9：拖动图片决定露出哪一块，或用方向键移动取景范围、加减号缩放；框里看到的就是最终效果。
    </p>

    <div class="cover-framing__stage">
      <div
        ref="frameEl"
        class="cover-framing__frame"
        :class="{ 'cover-framing__frame--dragging': dragging }"
        role="group"
        tabindex="0"
        aria-label="16:9 取景框：拖动或用方向键移动取景范围，加减号缩放"
        @pointerdown="onPointerDown"
        @keydown="onKeydown">
        <img
          class="cover-framing__image"
          :src="src"
          :alt="imageAlt"
          :style="imageStyle"
          draggable="false"
          @dragstart.prevent>
      </div>
    </div>

    <div class="cover-framing__controls">
      <button
        type="button"
        class="cover-framing__action cover-framing__action--icon"
        aria-label="缩小"
        :disabled="framing.zoom <= COVER_ZOOM_MIN"
        @click="apply(stepFramingZoom(framing, -1))">
        −
      </button>
      <input
        class="cover-framing__zoom"
        type="range"
        :min="COVER_ZOOM_MIN"
        :max="COVER_ZOOM_MAX"
        :step="COVER_ZOOM_STEP"
        :value="framing.zoom"
        aria-label="缩放"
        @input="onZoomInput">
      <button
        type="button"
        class="cover-framing__action cover-framing__action--icon"
        aria-label="放大"
        :disabled="framing.zoom >= COVER_ZOOM_MAX"
        @click="apply(stepFramingZoom(framing, 1))">
        +
      </button>
      <span class="cover-framing__readout">{{ readout }}</span>
    </div>

    <div class="cover-framing__actions">
      <button type="button" class="cover-framing__action" aria-label="重置取景" @click="resetFraming">重置</button>
      <button type="button" class="cover-framing__action" aria-label="取消取景" @click="onCancel">取消</button>
      <button
        type="button"
        class="cover-framing__action cover-framing__action--primary"
        aria-label="确认取景"
        @click="confirmFraming">
        确认取景
      </button>
    </div>
  </section>
</template>

<style scoped>
/* 与后台取景窗同一套材料：压暗的台面 + 16:9 的窗；颜色只消费语义 token。 */
.cover-framing {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cover-framing__hint {
  margin: 0;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-ink-muted);
}

.cover-framing__stage {
  display: flex;
  width: 100%;
  justify-content: center;
  padding: 14px;
  border-radius: var(--radius-lg);
  background: var(--color-surface-inverse);
}

.cover-framing__frame {
  position: relative;
  width: 100%;
  overflow: hidden;
  aspect-ratio: 16 / 9;
  border: 1px solid var(--color-on-strong-line);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.cover-framing__frame--dragging {
  cursor: grabbing;
}

.cover-framing__image {
  display: block;
  width: 100%;
  height: 100%;
  /* 让拖动始终落在取景框上：图片自己不吃指针事件，也不会被原生拖影拖走。 */
  pointer-events: none;
}

.cover-framing__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.cover-framing__zoom {
  flex: 1;
  min-width: 160px;
  accent-color: var(--color-ink);
}

.cover-framing__readout {
  white-space: nowrap;
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--color-ink-muted);
}

.cover-framing__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.cover-framing__action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  color: var(--color-ink);
  font-size: 14px;
  cursor: pointer;
  transition:
    background-color var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
}

.cover-framing__action:hover {
  border-color: var(--color-line-strong);
  background: var(--color-bg-subtle);
}

.cover-framing__action:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.cover-framing__action--icon {
  min-width: 34px;
  padding: 0 10px;
}

.cover-framing__action--primary {
  border-color: var(--color-ink);
  background: var(--color-ink);
  color: var(--color-on-strong);
}

.cover-framing__action--primary:hover {
  border-color: var(--color-ink-muted);
  background: var(--color-ink-muted);
  color: var(--color-bg-page);
}
</style>
