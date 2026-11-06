<script setup lang="ts">
import { motion } from "motion-v";
import { computed } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import type { MessageCenterNavItem, MessageCenterTab } from "~/utils/messageCenter";
import { shouldShowUnreadCount } from "~/utils/messageCenter";
import {
  NECK_NOMINAL_SPAN,
  NECK_SEPARATION_PX,
  isNeckSeamOpen,
  neckGradientStops,
  neckMotionKind,
  neckPath,
  neckSegmentMotion,
  neckTransition,
} from "~/utils/subnavNeck";

/**
 * 消息中心顶部子项条（票 239 建立，票 297 改成**一体化分段条**）。
 *
 * 形态（口径见 docs/notes/2026-11-07-reading-ui-grill.md §三 3.7）：
 *   - 五段**相邻贴合**成一个分段控件，整条是内容宽度（不再 `self-stretch` 拉满）；
 *   - 激活块 = 深墨面（语义层 inverse）铺底 + 反白文字，未激活段仍是次级墨色；
 *   - 只有激活段两侧的接缝张开，张开的接缝处画一段**颈部**：两条凹曲线夹出的腰形面，
 *     用线性渐变填充，两端分别取左右相邻段的填充色；
 *   - 段位 `margin-left` 与圆角由弹簧驱动（**全站唯一允许布局属性动画的地方**，
 *     仅限这条 ≤5 个节点，ADR-0012 后续修订 2026-11-07 已登记），颜色走 CSS 过渡；
 *   - 「减少动态」下弹簧落位（时长 0），不是放慢。
 *
 * 几何与弹簧接线全部在 `~/utils/subnavNeck`：本组件不写几何数字、也不碰
 * stiffness / damping（`test/design/motion-spring-gates.test.ts` 会扫）。
 *
 * 五个子项与未读数来自同一份表（`utils/messageCenter` 的 `messageCenterNavItems`），
 * 顶栏 hover 面板消费同一份数据，所以页面内与顶栏的角标不可能对不上。
 * 路由、子项顺序、标签文案、`aria-current` 与数据源在本票里都没变。
 */
const props = defineProps<{
  items: MessageCenterNavItem[];
  activeTab: MessageCenterTab;
}>();

const { prefersReducedMotion } = useMotionPreference();

const activeIndex = computed(() => props.items.findIndex((item) => item.key === props.activeTab));

/** 颈部路径与 viewBox 是常量：形状只描述比例，真实宽高由 CSS 拉伸。 */
const neckD = neckPath(NECK_SEPARATION_PX, NECK_NOMINAL_SPAN);
const neckViewBox = `0 0 ${NECK_SEPARATION_PX} ${NECK_NOMINAL_SPAN}`;

/**
 * 逐段的到位状态 + 颈部装饰。
 *
 * gradientId 用序号而不是随机 id：SSR 与客户端必须渲染出同一份标记，
 * 否则 hydration 会把整条 SVG 重建一遍。
 */
const segments = computed(() =>
  props.items.map((item, index) => ({
    item,
    active: index === activeIndex.value,
    seamOpen: isNeckSeamOpen(index, activeIndex.value),
    animate: neckSegmentMotion(index, activeIndex.value),
    stops: neckGradientStops(index, activeIndex.value),
    gradientId: `inbox-subnav-neck-${index}`,
  })),
);

const transition = computed(() => neckTransition(prefersReducedMotion.value));
const motionKind = computed(() => neckMotionKind(prefersReducedMotion.value));
</script>

<template>
  <nav class="message-center__subnav" aria-label="消息中心子项" :data-neck-motion="motionKind">
    <motion.div
      v-for="segment in segments"
      :key="segment.item.key"
      class="message-center__subnav-segment"
      :data-active="segment.active ? 'true' : 'false'"
      :data-seam="segment.seamOpen ? 'open' : 'closed'"
      :initial="false"
      :animate="segment.animate"
      :transition="transition"
    >
      <!-- 颈部：只有张开的接缝才画。路径与渐变端点都来自纯几何助手。 -->
      <svg
        v-if="segment.seamOpen"
        class="message-center-tab__neck"
        :viewBox="neckViewBox"
        preserveAspectRatio="none"
        aria-hidden="true"
        focusable="false"
      >
        <defs>
          <linearGradient :id="segment.gradientId" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" :stop-color="segment.stops.from" />
            <stop offset="1" :stop-color="segment.stops.to" />
          </linearGradient>
        </defs>
        <path :d="neckD" :fill="`url(#${segment.gradientId})`" />
      </svg>

      <NuxtLink
        :to="segment.item.to"
        class="message-center-tab"
        :class="{ 'is-active': segment.active }"
        :aria-current="segment.active ? 'page' : undefined"
      >
        <span>{{ segment.item.label }}</span>
        <span v-if="shouldShowUnreadCount(segment.item.count)" class="message-center-tab__count">
          {{ segment.item.count }}
        </span>
      </NuxtLink>
    </motion.div>
  </nav>
</template>
