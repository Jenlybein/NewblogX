<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";

import {
  DEFAULT_SHAPE_FRAMING,
  SHAPE_FRAMING_ZOOM_MAX,
  SHAPE_FRAMING_ZOOM_MIN,
  SHAPE_FRAMING_ZOOM_STEP,
  clampFraming,
  framingImageBox,
  nudgeFraming,
  panFramingByFraction,
  stepFramingZoom,
} from "~/utils/imageShaping";
import type { ShapeFramingState, ShapeImageTarget } from "~/utils/imageShaping";

/**
 * 裁剪窗的交互面（票 215）。
 *
 * 固定比例框 + 拖动画面 + 1–3× 缩放；没有旋转、镜像、滤镜。
 * 这里只做 DOM 与事件：比例判定、视口→源图矩形换算、缩放钳位全在
 * `~/utils/imageShaping`（两端逐字节同源）。预览画面与最终烤进文件用的是
 * **同一组源矩形**（`framingImageBox` 由 `framingSourceRect` 反推），
 * 所以"框里看到的就是上传的那张图"不靠对齐两套实现。
 */
const props = withDefaults(
  defineProps<{
    /** 原图的本地预览地址（确认前不上传；空串时只显示空白台面）。 */
    src?: string;
    /** 原图像素尺寸：决定拖动余量与预览画面的放大倍数。 */
    imageWidth: number;
    imageHeight: number;
    /** 目标比例/尺寸（封面 16:9、头像 1:1）。 */
    target: ShapeImageTarget;
    /** 比例的界面口径（"16:9" / "1:1"），只用于文案与可访问名称。 */
    ratioText?: string;
  }>(),
  { src: "", ratioText: "16:9" },
);

const emit = defineEmits<{
  confirm: [framing: ShapeFramingState];
  cancel: [];
  "update:framing": [framing: ShapeFramingState];
}>();

/** 方向键一次移动的取景范围（占视口宽/高的比例）。 */
const KEY_PAN_STEP = 0.05;
/** 量不到取景框尺寸时的参考宽（换算的是比例，与框多大无关）。 */
const FALLBACK_FRAME_WIDTH = 800;

const sourceSize = computed(() => ({ width: props.imageWidth, height: props.imageHeight }));
const framing = ref<ShapeFramingState>({ ...DEFAULT_SHAPE_FRAMING });
const dragging = ref(false);
const frameEl = ref<HTMLElement | null>(null);

/**
 * 比例框固定：比例由目标尺寸派生，组件里没有第二个数字。
 *
 * 尺寸方向（票 242）：以前框是整宽 + aspect-ratio，高度按宽度派生——头像 1:1 时
 * 高度≈弹层内容宽，观感就是"铺满窗口"。现在**由高度决定宽**：组件样式里把宽度
 * 算成 `高度预算（min(46dvh, 360px)）× 比例`，高度再由 aspect-ratio 反推；这里
 * 只交出比例这一个数。
 */
const frameStyle = computed(() => {
  const ratio = Number((props.target.width / props.target.height).toFixed(6));
  return {
    aspectRatio: `${props.target.width} / ${props.target.height}`,
    "--crop-frame-ratio": `${ratio}`,
  };
});

/** 画面布局（相对顶点的百分比）：与源矩形一一对应。 */
const imageStyle = computed(() => {
  const box = framingImageBox(sourceSize.value, props.target, framing.value);
  const percent = (value: number) => `${Math.round(value * 1000000) / 10000}%`;
  return {
    left: percent(box.left),
    top: percent(box.top),
    width: percent(box.width),
    height: percent(box.height),
  };
});

const readout = computed(
  () =>
    `缩放 ${framing.value.zoom.toFixed(2)}× · 画面 ${Math.round(framing.value.centerX * 100)}% / ${Math.round(
      framing.value.centerY * 100,
    )}%`,
);

function apply(next: ShapeFramingState) {
  framing.value = clampFraming(next, sourceSize.value, props.target);
  emit("update:framing", framing.value);
}

function measureFrame(): { width: number; height: number } {
  const rect = frameEl.value?.getBoundingClientRect();
  if (rect && rect.width > 0 && rect.height > 0) {
    return { width: rect.width, height: rect.height };
  }
  return {
    width: FALLBACK_FRAME_WIDTH,
    height: (FALLBACK_FRAME_WIDTH * props.target.height) / props.target.width,
  };
}

interface DragStart {
  x: number;
  y: number;
  framing: ShapeFramingState;
}

let dragStart: DragStart | null = null;

function onPointerDown(event: PointerEvent) {
  // 只接主键拖动：右键 / 中键不该把画面拖走。
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
  const frame = measureFrame();
  apply(
    panFramingByFraction(
      dragStart.framing,
      (event.clientX - dragStart.x) / frame.width,
      (event.clientY - dragStart.y) / frame.height,
      sourceSize.value,
      props.target,
    ),
  );
}

function stopDrag() {
  dragStart = null;
  dragging.value = false;
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", stopDrag);
  window.removeEventListener("pointercancel", stopDrag);
}

onBeforeUnmount(stopDrag);

/** 方向键移取景范围（右箭头看右边），加减号缩放，Esc 取消。 */
function onKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case "ArrowLeft":
      apply(nudgeFraming(framing.value, -KEY_PAN_STEP, 0, sourceSize.value, props.target));
      break;
    case "ArrowRight":
      apply(nudgeFraming(framing.value, KEY_PAN_STEP, 0, sourceSize.value, props.target));
      break;
    case "ArrowUp":
      apply(nudgeFraming(framing.value, 0, -KEY_PAN_STEP, sourceSize.value, props.target));
      break;
    case "ArrowDown":
      apply(nudgeFraming(framing.value, 0, KEY_PAN_STEP, sourceSize.value, props.target));
      break;
    case "+":
    case "=":
      apply(stepFramingZoom(framing.value, 1, sourceSize.value, props.target));
      break;
    case "-":
    case "_":
      apply(stepFramingZoom(framing.value, -1, sourceSize.value, props.target));
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
  apply({ ...framing.value, zoom: Number((event.target as HTMLInputElement).value) });
}

function resetFraming() {
  apply(DEFAULT_SHAPE_FRAMING);
}

function confirmFraming() {
  emit("confirm", clampFraming(framing.value, sourceSize.value, props.target));
}

/** 取消：不上抛取景状态；调用方据此中止本次上传、保留现有封面。 */
function onCancel() {
  emit("cancel");
}
</script>

<template>
  <section class="image-crop">
    <div class="image-crop__body" data-role="crop-body">
      <p class="image-crop__hint">
        固定 {{ props.ratioText }}：拖动画面决定露出哪一块，或用方向键移动取景范围、加减号缩放；
        框里看到的就是最终上传的那张图。
      </p>

      <div class="image-crop__stage">
        <div
          ref="frameEl"
          data-role="crop-frame"
          class="image-crop__frame"
          :class="{ 'image-crop__frame--dragging': dragging }"
          role="group"
          tabindex="0"
          :style="frameStyle"
          :aria-label="`裁剪框（${props.ratioText}）：拖动画面，方向键移动取景范围，加减号缩放，Esc 取消`"
          @pointerdown="onPointerDown"
          @keydown="onKeydown">
          <img
            v-if="props.src"
            data-role="crop-image"
            class="image-crop__image"
            :src="props.src"
            :style="imageStyle"
            alt="待裁剪的原图"
            draggable="false"
            @dragstart.prevent>
        </div>
      </div>

      <div class="image-crop__controls">
        <button
          type="button"
          class="image-crop__action image-crop__action--icon"
          aria-label="缩小"
          :disabled="framing.zoom <= SHAPE_FRAMING_ZOOM_MIN"
          @click="apply(stepFramingZoom(framing, -1, sourceSize, props.target))">
          −
        </button>
        <input
          class="image-crop__zoom"
          type="range"
          :min="SHAPE_FRAMING_ZOOM_MIN"
          :max="SHAPE_FRAMING_ZOOM_MAX"
          :step="SHAPE_FRAMING_ZOOM_STEP"
          :value="framing.zoom"
          aria-label="缩放"
          @input="onZoomInput">
        <button
          type="button"
          class="image-crop__action image-crop__action--icon"
          aria-label="放大"
          :disabled="framing.zoom >= SHAPE_FRAMING_ZOOM_MAX"
          @click="apply(stepFramingZoom(framing, 1, sourceSize, props.target))">
          +
        </button>
        <span class="image-crop__readout">{{ readout }}</span>
      </div>
    </div>

    <div class="image-crop__actions" data-role="crop-actions">
      <button type="button" class="image-crop__action" aria-label="重置取景" @click="resetFraming">重置</button>
      <button type="button" class="image-crop__action" aria-label="取消裁剪" @click="onCancel">取消</button>
      <button
        type="button"
        class="image-crop__action image-crop__action--primary"
        aria-label="确认裁剪"
        @click="confirmFraming">
        确认裁剪
      </button>
    </div>
  </section>
</template>

<style scoped>
/* 颜色与圆角只消费语义 token（ADR-0012 / ADR-0017），不新增裸色值。 */
.image-crop {
  display: flex;
  flex-direction: column;
  gap: 12px;
  /* 弹层给的是 max-height（100dvh - 48px），这里再兜一层卡片标题/内边距的预算：
     内容一长就在这里内部滚动，底部动作条不会被滚出去（票 242）。 */
  max-height: calc(100dvh - 144px);
}

.image-crop__body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
  /* 滚动到边界不把弹层后面的页面一起拖走。 */
  overscroll-behavior: contain;
}

.image-crop__hint {
  margin: 0;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-ink-muted);
}

.image-crop__stage {
  display: flex;
  width: 100%;
  justify-content: center;
  padding: 14px;
  border-radius: var(--radius-lg);
  background: var(--color-surface-inverse);
}

.image-crop__frame {
  position: relative;
  /* 由高度决定宽（票 242）：高度预算 46dvh 封顶 360px → 宽度 = 预算 × 比例
     （比例来自 frameStyle 的 --crop-frame-ratio），高度再由 aspect-ratio 反推；
     台面更窄时回落到 100%。旧写法（整宽撑高）已删除。 */
  width: min(100%, calc(min(46dvh, 360px) * var(--crop-frame-ratio, 1)));
  flex: none;
  max-width: 100%;
  overflow: hidden;
  border: 1px solid var(--color-on-strong-line);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
  cursor: grab;
  /* 触摸屏上拖动画面而不是滚动页面（移动端可拖动）。 */
  touch-action: none;
  user-select: none;
}

.image-crop__frame--dragging {
  cursor: grabbing;
}

.image-crop__image {
  position: absolute;
  display: block;
  max-width: none;
  /* 指针事件落在框上：图片自己不吃拖动，也不会被原生拖影拖走。 */
  pointer-events: none;
}

.image-crop__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.image-crop__zoom {
  flex: 1;
  min-width: 160px;
  accent-color: var(--color-ink);
}

.image-crop__readout {
  white-space: nowrap;
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--color-ink-muted);
}

.image-crop__actions {
  display: flex;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.image-crop__action {
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

.image-crop__action:hover {
  border-color: var(--color-line-strong);
  background: var(--color-bg-subtle);
}

.image-crop__action:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.image-crop__action--icon {
  min-width: 34px;
  padding: 0 10px;
}

.image-crop__action--primary {
  border-color: var(--color-ink);
  background: var(--color-ink);
  color: var(--color-on-strong);
}

.image-crop__action--primary:hover {
  border-color: var(--color-ink-muted);
  background: var(--color-ink-muted);
  color: var(--color-bg-page);
}
</style>
