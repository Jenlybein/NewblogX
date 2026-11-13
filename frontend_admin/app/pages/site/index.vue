<script setup lang="ts">
/**
 * 站点基础（`/site`）：备案号 + SEO。
 *
 * 「站点」拆成四页后（ADR-0029），本页只拥有 `site_info` 与 `seo` 两个顶层块；
 * 保存走读-改-写，不覆盖登录策略 / 发布治理 / 私信，也不动没有编辑入口的块。
 * 路由沿用旧地址 `/site`，旧书签不失效。
 */
import SettingsSiteBasics from '~/components/settings/SettingsSiteBasics.vue'
</script>

<template>
  <SettingsSiteBasics />
</template>
