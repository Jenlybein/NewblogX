<script setup lang="ts">
import { computed } from "vue";
import { useRuntimeConfig } from "#imports";

import { useCursorCharm } from "~/composables/useCursorCharm";
import { brandAssetUrl } from "~/utils/brand";
import {
  CHARM_CONFIG_FIELDS,
  clampCharmLogoSize,
  type CharmRopeConfigInput,
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
 *
 * 渲染成本：物理状态在组合式里是非响应式普通对象；每帧只有两处可见输出 ——
 * 一条路径的 `d`（带形多边形的填充）与站标图像的一个 `transform`。不碰任何
 * 布局属性；每秒画几次由后台 `deco.charm.fps_cap`（默认 60）门控，与指针是否
 * 在移动无关 —— 上限只影响"每秒画几次"，不影响绳子运动速度。
 *
 * 指针端偏移（`pointer_offset_x` / `pointer_offset_y`）**不在这里加**：它由组合式
 * 换算进指针端节点，本组件的路径与 `transform` 直接读那份结果。渲染层再偏一次，
 * 弧垂与张力就会算在错的原点上（ADR-0018 约束 7）。
 *
 * 进出场：指针移出窗口 → 立即离开可见态（快档渐隐），指针端在原处冻结、链自然
 * 收敛、绝不瞬移；移回满 0.1 秒才渐显，0.1 秒内再次移出则取消这次显示（防抖，
 * 沿窗口边缘反复划过因此不闪）。0.1 秒窗口是组合式里可见性状态机的事，本组件
 * 只消费 `visible`（= 这一刻该不该在屏幕上），不自己计时。
 */
const siteStore = useSiteStore();

// 本组件挂在应用根层（覆盖默认 / 创作 / 写作三个布局），而写作页布局
// 从不拉公开配置 —— 这里自己确保那一次引导已经发生。store 内部幂等，
// 默认/创作布局上不会多发一轮请求。
void siteStore.ensurePublicBootstrap();

const runtimeConfig = useRuntimeConfig();
const logoSrc = computed(() => brandAssetUrl(String(runtimeConfig.app.baseURL || "/"), "logo"));

const charmConfig = computed(() => siteStore.deco?.charm);
// 站标尺寸不是挂绳物理参数（不进 normalizeCharmRopeConfig），由它自己的归一化
// 函数兜底；`Number(null) === 0`、而 0 对站标尺寸就是"没填"，钳位会回落 48。
const logoSize = computed(() => clampCharmLogoSize(Number(charmConfig.value?.logo_size)));

const { enabled, visible, tetherPath, logoCenter } = useCursorCharm({
  configEnabled: () => Boolean(charmConfig.value?.enable),
  // 十个数值参数**原样**交出去（可能是 null / undefined）：唯一的兜底点是
  // `normalizeCharmRopeConfig` —— 组合式每次建链 / 读参数时都会跑它。
  // 组件层不再把 null 折成 NaN 或 0：那等于把兜底点复制到第二处，两处一旦
  // 分叉（漏一个字段、或有人改了一条规则）就又会退回"null 被读成 0"。
  // 接线本身也由 `CHARM_CONFIG_FIELDS` 驱动（票 #78）：配置键 ↔ 后端键 ↔ 钳位
  // 规则只写一遍，加参数不再改这里。
  config: () =>
    CHARM_CONFIG_FIELDS.reduce<CharmRopeConfigInput>((params, { configKey, storeKey }) => {
      params[configKey] = charmConfig.value?.[storeKey];
      return params;
    }, {}),
});

/** 站标图像画在原点、只用一个 translate 定位（每帧只改这一个 transform）。 */
const logoTransform = computed(() => {
  const center = logoCenter.value;
  const half = logoSize.value / 2;
  return `translate(${(center.x - half).toFixed(1)} ${(center.y - half).toFixed(1)})`;
});
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
        :transform="logoTransform"
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
 *
 * 透明度有两个方向、两个时长：移出走快档（越快越像"跟着指针一起离开窗口"），
 * 渐显走 base 档。CSS 的过渡取"变化之后"那条声明，所以下面必须两处都写
 * `transition` —— 只改基础声明会让 200ms 的渐显也变成快档。
 */
.charm-layer {
  position: fixed;
  inset: 0;
  z-index: 9999;
  pointer-events: none;
  opacity: 0;
  transition: opacity var(--motion-fast) var(--motion-ease);
}

/* 渐显（首次登场 / 移回满 0.1 秒后）：200ms，base 档 + 全站唯一缓动；
 * 不做位移或缩放。移回的 0.1 秒防抖在组合式里，不在 CSS 里 —— 它要能被
 * 单测断言（"0.1 秒内再次移出就取消显示"）。 */
.charm-layer--visible {
  opacity: 1;
  transition: opacity var(--motion-base) var(--motion-ease);
}

.charm-layer__canvas {
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

/* 挂绳：极淡语义 token，它的既有定义就是"仅大字 / 装饰 / 控件边界"。
 *
 * 这里用 `fill` 而不是 `stroke`：一根等宽 stroke 变不了宽度，而挂绳要求
 * 起始 → 0.6 倍的收尾。几何层每帧生成一条沿中心线两侧偏移半宽的带形
 * 多边形（`charmTetherGeometry`），所以这里只负责填色。每帧 DOM 写入
 * 仍然只有路径的一个 `d` 与站标的一个 `transform`，与票 #41 一致。 */
.charm-layer__tether {
  fill: var(--color-ink-faint);
  stroke: none;
  pointer-events: none;
}

.charm-layer__logo {
  pointer-events: none;
}
</style>
