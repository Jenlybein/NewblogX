<script setup lang="ts">
/**
 * 旧地址 `/site/publish` 的跳转页。
 *
 * 这一页搬到了「内容修改与发布 → 文章发布」（`/content/publish`，ADR-0029 改写版）。
 * 留跳转页而不是直接删文件：这个地址在文档、书签与运营记录里存在过。
 */
onMounted(() => {
  void navigateTo('/content/publish', { replace: true })
})
</script>

<template>
  <div class="admin-empty">正在跳转到文章发布…</div>
</template>
