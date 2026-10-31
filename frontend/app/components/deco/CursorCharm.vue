<script setup lang="ts">
import { computed } from "vue";
import { useRuntimeConfig } from "#imports";

import { useCursorCharm } from "~/composables/useCursorCharm";
import { brandAssetUrl } from "~/utils/brand";
import {
  CHARM_TETHER_LENGTH_PX,
  charmLogoCenter,
  charmTetherPath,
  clampCharmLogoSize,
} from "~/utils/decoCharm";

/**
 * 光标挂件：一条挂绳把站标系在指针上（装饰层，不是功能入口）。
 *
 * 为什么是 SVG 覆盖层而不是 canvas：描边与图像都能直接吃语义色 token，
 * 暗色/亮色、高 DPI 全部免费；canvas 得每帧或切主题时手读计算样式，
 * 既撞设计 token 门禁，测试环境里也没有 canvas 实现。
 *
 * 为什么一整层 `pointer-events: none` + `aria-hidden="true"`：它浮在一切之上
 * （z-index 9999，压过 Naive 的弹窗与提示条），但"在上面"不等于"挡住"——
 * 点击穿透、不进 Tab 序列、读屏读不到。
 *
 * 为什么总开关关着时整个模板都不渲染：`v-if` 连 `<image>` 一起摘掉，
 * 于是关掉之后既没有站标素材请求，也没有任何逐帧计算。
 */
const siteStore = useSiteStore();

// 本组件挂在应用根层（覆盖默认 / 创作 / 写作三个布局），而写作页布局
// 从不拉公开配置 —— 这里自己确保那一次引导已经发生。store 内部幂等，
// 默认/创作布局上不会多发一轮请求。
void siteStore.ensurePublicBootstrap();

const runtimeConfig = useRuntimeConfig();
const logoSrc = computed(() => brandAssetUrl(String(runtimeConfig.app.baseURL || "/"), "logo"));

const charmConfig = computed(() => siteStore.deco?.charm);
const logoSize = computed(() => clampCharmLogoSize(Number(charmConfig.value?.logo_size)));

const { enabled, visible, pointer } = useCursorCharm({
  configEnabled: () => Boolean(charmConfig.value?.enable),
});

// 本票不做物理：站标固定在指针下方一个绳长处，挂绳是一条直线。
const logoCenter = computed(() => charmLogoCenter(pointer.value, CHARM_TETHER_LENGTH_PX));
const tetherPath = computed(() => charmTetherPath(pointer.value, logoCenter.value));
const logoBox = computed(() => ({
  x: logoCenter.value.x - logoSize.value / 2,
  y: logoCenter.value.y - logoSize.value / 2,
}));
</script>

<template>
  <div
    v-if="enabled"
    class="charm-layer"
    :class="{ 'charm-layer--visible': visible }"
    aria-hidden="true"
  >
    <svg class="charm-layer__canvas" aria-hidden="true" focusable="false">
      <path class="charm-layer__tether" :d="tetherPath" />
      <!-- 与 favicon 同一张画的品牌资产：不新造素材，也不按主题换变体。 -->
      <image
        class="charm-layer__logo"
        :href="logoSrc"
        :x="logoBox.x"
        :y="logoBox.y"
        :width="logoSize"
        :height="logoSize"
      />
    </svg>
  </div>
</template>

<style scoped>
/*
 * 固定铺满视口的一层，只画不接：整层 pointer-events: none，
 * 子元素也一律 none（不需要任何"可交互的子元素"，这与捏捏正好相反）。
 * 层级 9999 是票面定的：压过 Naive 的弹窗/消息（2000+），低于浏览器扩展层。
 */
.charm-layer {
  position: fixed;
  inset: 0;
  z-index: 9999;
  pointer-events: none;
  opacity: 0;
  transition: opacity var(--motion-base) var(--motion-ease);
}

/* 首次指针移动后淡入（200ms，base 档 + 全站唯一缓动）；不做位移或缩放。 */
.charm-layer--visible {
  opacity: 1;
}

.charm-layer__canvas {
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

/* 挂绳：极淡语义 token，它的既有定义就是"仅大字 / 装饰 / 控件边界"。 */
.charm-layer__tether {
  fill: none;
  stroke: var(--color-ink-faint);
  stroke-width: 1.5;
  stroke-linecap: round;
}

.charm-layer__logo {
  pointer-events: none;
}
</style>
