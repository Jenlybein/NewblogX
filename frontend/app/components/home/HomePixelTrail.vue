<script setup lang="ts">
import { ref } from "vue";

import { usePixelTrail } from "~/composables/usePixelTrail";
import { useMotionPreference } from "~/composables/useMotionPreference";

/**
 * 首屏背景板上的「像素尾迹」（ADR-0031）。
 *
 * 画布只是装饰层：`aria-hidden`、`pointer-events: none`，不抢轮播/链接的点击；
 * 指针采样从 window 收（画布不吃事件），划过内容层也照样有尾迹。
 *
 * 颜色不在组件里写死：尾迹读首屏的 `--jelly-yellow`（与黄色色带同一支），
 * 所以亮/暗主题切换、token 调整都不需要改这里。
 */
const props = withDefaults(
  defineProps<{
    /** 像素格边长（CSS px）。 */
    cellSize?: number;
    /** 方块寿命（毫秒）。 */
    lifeMs?: number;
  }>(),
  { cellSize: 14, lifeMs: 700 },
);

const root = ref<HTMLElement | null>(null);
const canvas = ref<HTMLCanvasElement | null>(null);
const { prefersReducedMotion } = useMotionPreference();

usePixelTrail(
  canvas,
  root,
  () => ({ cellSize: props.cellSize, lifeMs: props.lifeMs }),
  prefersReducedMotion,
);
</script>

<template>
  <div ref="root" class="home-pixels" aria-hidden="true" data-home-pixels>
    <canvas ref="canvas" class="home-pixels__canvas" aria-hidden="true" />
  </div>
</template>

<style scoped>
.home-pixels {
  position: absolute;
  inset: 0;
  z-index: 1;
  overflow: hidden;
  pointer-events: none;
  touch-action: pan-y;
}

.home-pixels__canvas {
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
