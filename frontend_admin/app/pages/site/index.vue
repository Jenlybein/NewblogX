<script setup lang="ts">
/**
 * 站点域（`/site`）：站点基础信息 + 登录策略。
 *
 * 站点配置的八个域拆成独立路由后，每个域一个页面文件、由 URL 决定当前域：
 * 刷新停留在本域，链接可以直接分享。这里只负责挂载配置组件，
 * 组件本身不改名、不移动（表单与保存逻辑仍在自己那一段里）。
 *
 * 「登录策略」属于 site 段（`site.login`），所以和站点基础信息放在同一页、
 * 共用一次保存，否则两个组件各自提交整段 site 会互相覆盖。
 */
import SettingsSite from '~/components/settings/SettingsSite.vue'
</script>

<template>
  <SettingsSite />
</template>
