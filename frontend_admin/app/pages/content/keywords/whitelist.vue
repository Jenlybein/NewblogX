<script setup lang="ts">
/**
 * 关键词白名单（`/content/keywords/whitelist`）：`content_filter` 段的白名单独立页。
 *
 * 2026-11-27 从词表页拆出（同一轮按设计稿重排）：白名单是"例外清单"，与词表
 * 共写同一段配置但**各管各的字段**，拆页后两页的保存按钮语义才不会互相清空字段。
 * 词表页在 `pages/content/keywords/index.vue`。
 */
import SettingsContentFilterWhitelist from "~/components/settings/SettingsContentFilterWhitelist.vue";
</script>

<template>
  <SettingsContentFilterWhitelist />
</template>
