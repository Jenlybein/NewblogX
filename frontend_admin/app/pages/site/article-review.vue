<script setup lang="ts">
/**
 * 文章审核设置（`/site/article-review`）：审核规则的唯一配置入口（2026-11-27）。
 *
 * 2026-11-28 起它是「文章审核设置」分组下的「审核规则」子页（另一页是「关键词命中处理」）。
 * 内容审核（免审核开关）、文章质量评分、质量闸门、AI 合规判定、投稿配额 ——
 * 它们是"提交之后要不要审、审多严、一天能交几篇"的规则，和「发布与展示」
 * （谁能发、发出来对谁可见）是两件事。
 *
 * 两页写同一个 site 段的 article 块，但保存走**字段级补丁**（见
 * `utils/siteSectionSave.applySiteBlockFieldPatch`），互不覆盖对方的字段。
 */
import SettingsArticleReview from '~/components/settings/SettingsArticleReview.vue'
</script>

<template>
  <SettingsArticleReview />
</template>
