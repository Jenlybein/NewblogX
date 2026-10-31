<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

import {
  BANNER_ASPECT,
  DEFAULT_FRAMING,
  ZOOM_MAX,
  ZOOM_MIN,
  ZOOM_STEP,
  clampFraming,
  framingStyle,
  nudgeFraming,
  panFraming,
  snapZoom,
  stepFramingZoom,
} from '~/utils/framing'
import type { Framing, Size } from '~/utils/framing'

/**
 * 16:9 取景窗（#45）。只管交互：算术全部在 `~/utils/framing`。
 *
 * 为什么图片用 `object-fit: cover` + `object-position` + `transform: scale()`
 * 而不是自绘一块画布：这四条声明就是读者站渲染同一张封面的方式。
 * 预览与线上用的是**同一组**百分比，于是"所见即最终效果"不是靠对齐两个实现，
 * 而是根本没有第二个实现（`framingStyle()` 是唯一出口）。
 *
 * 不引入任何裁剪库：这里没有任何像素级图像处理，只在原图上挪一个窗口。
 */

const props = withDefaults(
  defineProps<{
    /** 原图地址（不裁剪、不重传，线上也用这一张）。 */
    src: string
    /** 原图像素尺寸：决定 cover 之后哪个方向有多余量可拖。 */
    imageWidth: number
    imageHeight: number
    /** 已有的取景记录（重新取景时带进来）；缺省＝居中、zoom 1。 */
    framing?: Framing | null
    imageAlt?: string
  }>(),
  { framing: null, imageAlt: '' },
)

const emit = defineEmits<{
  confirm: [framing: Framing]
  cancel: []
  'update:framing': [framing: Framing]
}>()

const framing = ref<Framing>(clampFraming(props.framing))
const dragging = ref(false)
const frameEl = ref<HTMLElement | null>(null)

// 同一个组件既要能首次打开，也要能"换一张图再打开"：外部参数变了就重新开场。
watch(
  () => props.framing,
  (next) => {
    framing.value = clampFraming(next)
  },
)

/** 四条内联声明：与线上渲染逐条相同，没有像素值。 */
const imageStyle = computed(() => framingStyle(framing.value))

/** 读数（只给人看）：焦点百分比与缩放倍数。 */
const readout = computed(
  () =>
    `焦点 ${Math.round(framing.value.focus_x * 100)}% · ${Math.round(framing.value.focus_y * 100)}% · 缩放 ${framing.value.zoom.toFixed(2)}×`,
)

function apply(next: Framing) {
  framing.value = clampFraming(next)
  emit('update:framing', framing.value)
}

/**
 * 取景框的像素尺寸。
 *
 * 量不到（首帧未排版、被隐藏、测试用的 happy-dom 没有布局引擎）时退回一个
 * 16:9 的参考尺寸：拖拽换算出来的**是比例**，而比例与框多大无关——只要框仍是
 * 16:9，用 800×450 和用真实尺寸算出的焦点完全相同。
 */
function measureFrame(): Size {
  const rect = frameEl.value?.getBoundingClientRect()
  if (rect && rect.width > 0 && rect.height > 0) {
    return { width: rect.width, height: rect.height }
  }
  return { width: 800, height: 800 / BANNER_ASPECT }
}

function imageSize(): Size {
  return { width: props.imageWidth, height: props.imageHeight }
}

interface DragStart {
  x: number
  y: number
  framing: Framing
}

let dragStart: DragStart | null = null

function onPointerDown(event: PointerEvent) {
  // 只接主键拖动：右键菜单 / 中键滚动不该把图拖走。
  if (event.button > 0) return
  dragStart = { x: event.clientX, y: event.clientY, framing: { ...framing.value } }
  dragging.value = true
  frameEl.value?.focus()
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', stopDrag)
  window.addEventListener('pointercancel', stopDrag)
  event.preventDefault()
}

function onPointerMove(event: PointerEvent) {
  if (!dragStart) return
  apply(panFraming(dragStart.framing, event.clientX - dragStart.x, event.clientY - dragStart.y, measureFrame(), imageSize()))
}

function stopDrag() {
  dragStart = null
  dragging.value = false
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', stopDrag)
  window.removeEventListener('pointercancel', stopDrag)
}

onBeforeUnmount(stopDrag)

/**
 * 方向键移的是**取景范围**（按右箭头看右边），加减号缩放。
 * 与鼠标拖动方向相反是有意的：拖动是"推图"，一句话各自讲得清；
 * 提示文案因此不写"方向键推图"。
 */
function onKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case 'ArrowLeft':
      apply(nudgeFraming(framing.value, -1, 0))
      break
    case 'ArrowRight':
      apply(nudgeFraming(framing.value, 1, 0))
      break
    case 'ArrowUp':
      apply(nudgeFraming(framing.value, 0, -1))
      break
    case 'ArrowDown':
      apply(nudgeFraming(framing.value, 0, 1))
      break
    case '+':
    case '=':
      apply(stepFramingZoom(framing.value, 1))
      break
    case '-':
    case '_':
      apply(stepFramingZoom(framing.value, -1))
      break
    case 'Escape':
      onCancel()
      break
    default:
      return
  }
  event.preventDefault()
}

function onZoomInput(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  apply({ ...framing.value, zoom: snapZoom(value) })
}

function resetFraming() {
  apply(DEFAULT_FRAMING)
}

function confirmFraming() {
  emit('confirm', clampFraming(framing.value))
}

/** 取消：不上抛任何取景参数（调用方据此中止本次上传，什么也不产生）。 */
function onCancel() {
  emit('cancel')
}
</script>

<template>
  <section class="admin-framing">
    <p class="admin-framing__hint">
      固定 16:9：拖动图片决定露出哪一块，或用方向键移动取景范围、加减号缩放；框里看到的就是线上最终效果。
    </p>

    <div class="admin-framing__stage">
      <div
        ref="frameEl"
        class="admin-framing__frame"
        :class="{ 'admin-framing__frame--dragging': dragging }"
        role="group"
        tabindex="0"
        aria-label="16:9 取景框：拖动或用方向键移动取景范围，加减号缩放"
        @pointerdown="onPointerDown"
        @keydown="onKeydown">
        <img
          class="admin-framing__image"
          :src="src"
          :alt="imageAlt"
          :style="imageStyle"
          draggable="false"
          @dragstart.prevent>
      </div>
    </div>

    <div class="admin-framing__controls">
      <button
        type="button"
        class="admin-framing__action admin-framing__action--icon"
        aria-label="缩小"
        :disabled="framing.zoom <= ZOOM_MIN"
        @click="apply(stepFramingZoom(framing, -1))">
        −
      </button>
      <input
        class="admin-framing__zoom"
        type="range"
        :min="ZOOM_MIN"
        :max="ZOOM_MAX"
        :step="ZOOM_STEP"
        :value="framing.zoom"
        aria-label="缩放"
        @input="onZoomInput">
      <button
        type="button"
        class="admin-framing__action admin-framing__action--icon"
        aria-label="放大"
        :disabled="framing.zoom >= ZOOM_MAX"
        @click="apply(stepFramingZoom(framing, 1))">
        +
      </button>
      <span class="admin-framing__readout">{{ readout }}</span>
    </div>

    <div class="admin-framing__actions">
      <button type="button" class="admin-framing__action" aria-label="重置取景" @click="resetFraming">重置</button>
      <button type="button" class="admin-framing__action" aria-label="取消取景" @click="onCancel">取消</button>
      <button
        type="button"
        class="admin-framing__action admin-framing__action--primary"
        aria-label="确认取景"
        @click="confirmFraming">
        确认取景
      </button>
    </div>
  </section>
</template>
