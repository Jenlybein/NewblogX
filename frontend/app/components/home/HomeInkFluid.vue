<script setup lang="ts">
import { computed, ref } from "vue";

import { useInkFluid, type InkFluidOptions } from "~/composables/useInkFluid";
import { useMotionPreference } from "~/composables/useMotionPreference";

/**
 * 首屏背景板上的「指针墨流」。
 *
 * 画布只是装饰层：`aria-hidden`、`pointer-events: none`，不抢轮播/链接的点击；
 * 指针采样挂在父级 `.home-cover` 上，靠事件冒泡拿到移动量。
 *
 * 颜色不在组件里写死：墨流读首屏的 `--color-on-inverse`（主墨）与
 * `--color-on-inverse-muted`（次级反白），所以亮/暗主题切换、token 调整都不需要改这里。
 */
const props = withDefaults(
  defineProps<{
    /** 墨的最终不透明度，0–1。 */
    intensity?: number;
    /** 次级反白占比，0–1；其余是主墨。 */
    tint?: number;
    /** 快速划动推流体的力度。 */
    force?: number;
    /** 指针静止时保留的缓慢墨羽。 */
    ambient?: boolean;
  }>(),
  { intensity: 0.95, tint: 0.5, force: 0.7, ambient: false },
);

const root = ref<HTMLElement | null>(null);
const canvas = ref<HTMLCanvasElement | null>(null);
const { prefersReducedMotion } = useMotionPreference();

const options = computed<InkFluidOptions>(() => ({
  intensity: props.intensity,
  tint: props.tint,
  force: props.force,
  ambient: props.ambient,
  interactive: true,
  quality: "medium",
}));

useInkFluid(canvas, root, () => options.value, prefersReducedMotion);
</script>

<template>
  <div ref="root" class="home-ink" aria-hidden="true" data-home-ink>
    <canvas ref="canvas" class="home-ink__canvas" aria-hidden="true" />
  </div>
</template>

<style scoped>
.home-ink {
  position: absolute;
  inset: 0;
  z-index: 1;
  overflow: hidden;
  pointer-events: none;
  touch-action: pan-y;
}

/* 没有 WebGL2 / 浮点 color buffer 时，画布不参与合成；首屏回到纯深墨底。 */
.home-ink[data-ink-fallback] {
  display: none;
}

.home-ink__canvas {
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
