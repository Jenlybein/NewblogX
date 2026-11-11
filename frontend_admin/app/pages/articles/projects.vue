<script setup lang="ts">
/**
 * 项目管理页（票 348）。
 *
 * 页面本身只剩"哪一类实体"这一个参数：分组网格、拖排、分组 CRUD、编辑弹窗
 * 都在 EntityAdminPage 里，与合集共用同一份实现（ADR-0041 的分组语义
 * 对两个实体完全一致）。
 */
import EntityAdminPage from '~/components/articles/EntityAdminPage.vue'
</script>

<template>
  <EntityAdminPage kind="project" />
</template>