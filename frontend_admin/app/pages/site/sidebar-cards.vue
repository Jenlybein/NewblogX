<script setup lang="ts">
/**
 * 右栏卡片管理页。
 *
 * 页面只负责壳与标题；编辑逻辑在 `SidebarCardsEditor`，纯函数在
 * `~/utils/sidebarCardsAdmin`。入口挂在「站点配置」下，与其它站点运营项同层。
 */
import { NCard } from 'naive-ui'
import SidebarCardsEditor from '~/components/settings/SidebarCardsEditor.vue'
</script>

<template>
  <NCard class="admin-card" title="右栏卡片">
    <SidebarCardsEditor />
  </NCard>
</template>
