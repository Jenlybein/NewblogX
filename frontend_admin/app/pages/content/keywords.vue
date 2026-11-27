<script setup lang="ts">
/**
 * 关键词过滤（`/content/keywords`）：第 11 段 `content_filter` 的独立页面（ADR-0032）。
 *
 * 页面从 `/site/content-filter` 搬到这里（ADR-0029 改写版）：它属于内容生产链
 * （改词表 → 调发布线 → 审文章），不是"站点配置"—— 站点配置里那些是站点级开关，
 * 而词表直接决定一篇稿子能不能过。旧地址留了跳转页。
 *
 * 名字从「内容过滤」改成「关键词过滤」：后台另有一个「审核 → 关键词风险」
 * （命中记录与处置），两个叫"内容过滤"的名字并存时，第一次就会走错页。
 * 现在：过滤是**配置**（改词表），风险是**结果**（看命中）。
 */
import SettingsContentFilter from '~/components/settings/SettingsContentFilter.vue'
</script>

<template>
  <SettingsContentFilter />
</template>
