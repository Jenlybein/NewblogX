<script setup lang="ts">
import type { MessageCenterNavItem, MessageCenterTab } from "~/utils/messageCenter";
import { shouldShowUnreadCount } from "~/utils/messageCenter";

/**
 * 消息中心左侧子项导航（第 239 票）。
 *
 * 五个子项与未读数来自同一份表（`utils/messageCenter` 的 `messageCenterNavItems`），
 * 顶栏 hover 面板消费同一份数据，所以"页面内未读数"与"顶栏未读数"不可能对不上。
 *
 * 未读数 0 时**不渲染数字**：0 只会制造噪声，未读态由列表行的强调条 / 圆点表达。
 * 选中态（抬升面底 + 左侧 2px 强调条 + 字重）在 tailwind.css 的
 * `.message-center-tab.is-active`，<1024px 整条导航转成顶部横向 tab（同一份 DOM）。
 */
defineProps<{
  items: MessageCenterNavItem[];
  activeTab: MessageCenterTab;
}>();
</script>

<template>
  <nav class="message-center__subnav" aria-label="消息中心子项">
    <NuxtLink
      v-for="item in items"
      :key="item.key"
      :to="item.to"
      class="message-center-tab"
      :class="{ 'is-active': activeTab === item.key }"
      :aria-current="activeTab === item.key ? 'page' : undefined"
    >
      <span>{{ item.label }}</span>
      <span v-if="shouldShowUnreadCount(item.count)" class="message-center-tab__count">{{ item.count }}</span>
    </NuxtLink>
  </nav>
</template>
