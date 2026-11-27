<script setup lang="ts">
/**
 * 文章发布（`/content/publish`）：只改 site 段的 `article` 块。
 *
 * 页面从 `/site/publish` 搬到这里（ADR-0029 改写版）：「发布线多高、免审核要不要开」
 * 是内容生产链的中段，与「关键词过滤」「文章审核」是一件事的三步。放在站点配置里时，
 * 它和十几个站点级开关挤在一起，而它其实决定每一篇稿子的命运。
 *
 * 旧地址 `/site/publish` 留了跳转页。
 */
import SettingsSitePublish from '~/components/settings/SettingsSitePublish.vue'
</script>

<template>
  <SettingsSitePublish />
</template>
