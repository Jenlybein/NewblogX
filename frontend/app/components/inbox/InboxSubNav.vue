<script setup lang="ts">
import type { MessageCenterNavItem, MessageCenterTab } from "~/utils/messageCenter";
import { shouldShowUnreadCount } from "~/utils/messageCenter";

/**
 * 消息中心顶部导航条（2026-11-05 重构，替代原来的 220px 左侧竖导航）。
 *
 * 五个子项与未读数来自同一份表（`utils/messageCenter` 的 `messageCenterNavItems`），
 * 顶栏 hover 面板消费同一份数据，所以页面内与顶栏的角标不可能对不上。
 *
 * 选中态 = 抬升面底 + 底部 2px 强调条 + 字重；未读数 0 不渲染数字。
 * 窄屏横向可滚，不换行、不截断文字。
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
