<script setup lang="ts">
import { motion } from "motion-v";
import { computed } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import type { MessageCenterNavItem, MessageCenterTab } from "~/utils/messageCenter";
import { shouldShowUnreadCount } from "~/utils/messageCenter";
import {
  isSeamOpen,
  seamMotionKind,
  seamSegmentMotion,
  seamTransition,
} from "~/utils/subnavNeck";

/**
 * 消息中心顶部子项条（票 239 建立，票 297 改成**一体化分段条**，本次去掉颈部连接）。
 *
 * 形态（口径见阅读页 UI 记录 §三 3.7）：
 *   - 五段**相邻贴合**成一个分段控件，整条是内容宽度（不再 `self-stretch` 拉满）；
 *   - 未激活段是轨道面（`--color-surface-raised`），激活块是深墨面
 *     （`--color-surface-inverse`）铺底 + 反白文字，敞口侧保留 `--radius-md` 圆角；
 *   - 激活段两侧的接缝张开 10px：**只是间隙，不画任何连接形状** —— 激活块读成一块
 *     独立的深色圆角块，与左右邻居之间是干净的页底色；
 *   - 段位 `margin-left` 与四角圆角由同一组弹簧驱动（**全站唯一允许布局属性动画的
 *     地方**，仅限这条 ≤5 个节点，ADR-0011 后续修订 2026-11-07 已登记），
 *     颜色走 CSS 过渡；
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
const seamCount = computed(() => props.items.length);

/**
 * 逐段的到位状态。
 *
 * 没有颈部了，所以这里只剩「这一段该让开多少 + 哪两个角是圆的」：两者来自同一份
 * 接缝判定，过渡时不会各走各的。
 */
const segments = computed(() =>
  props.items.map((item, index) => ({
    item,
    active: index === activeIndex.value,
    /** 第 0 段左边没有接缝；其余每段的左接缝是否张开，只作观察标记（样式全在行内）。 */
    seamOpen: isSeamOpen(index, activeIndex.value),
    animate: seamSegmentMotion(index, activeIndex.value, seamCount.value),
  })),
);

const transition = computed(() => seamTransition(prefersReducedMotion.value));
const motionKind = computed(() => seamMotionKind(prefersReducedMotion.value));
</script>

<template>
  <nav class="message-center__subnav" aria-label="消息中心子项" :data-seam-motion="motionKind">
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
